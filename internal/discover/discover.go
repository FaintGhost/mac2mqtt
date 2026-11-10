// Package discover finds MQTT brokers on the local network, for the first
// setup: those announcing themselves over mDNS (_mqtt._tcp), and the
// hosts of Home Assistant (_home-assistant._tcp), whose Mosquitto add-on
// listens on the same host, which Home Assistant does not announce.
package discover

import (
	"context"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

// Broker is a broker found on the network.
type Broker struct {
	// Name is what announced it: Home Assistant's location, as "Home", or
	// the broker's own name.
	Name string
	Host string
	Port int
	TLS  bool
}

// Address is the broker's host and port, as "192.168.1.10:1883".
func (b Broker) Address() string { return net.JoinHostPort(b.Host, strconv.Itoa(b.Port)) }

const (
	mqttService = "_mqtt._tcp.local."
	haService   = "_home-assistant._tcp.local."
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// Find asks the network once, listens for answers for about wait, and
// returns the brokers that take a connection: those announced, and
// Mosquitto's ports, 1883 and 8883, on Home Assistant's hosts. It returns
// none, without an error, when the network does not answer, as when
// macOS's Local Network access is denied.
func Find(ctx context.Context, wait time.Duration) []Broker {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil
	}
	defer conn.Close()
	// Asked from a port other than 5353, responders answer to it alone
	// (RFC 6762's legacy unicast): nothing listens on mDNS's own port,
	// which mDNSResponder holds.
	query, err := buildQuery()
	if err != nil {
		return nil
	}
	if sendOnEachInterface(conn, query) == 0 {
		return nil
	}
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	answers := newAnswers()
	buf := make([]byte, 9000)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		answers.read(buf[:n])
	}
	return probe(ctx, answers.candidates())
}

// sendOnEachInterface asks on every local network the Mac is on: the
// route to the group alone may go through a VPN, where no one answers.
// It returns on how many it asked.
func sendOnEachInterface(conn *net.UDPConn, query []byte) int {
	pc := ipv4.NewPacketConn(conn)
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0
	}
	sent := 0
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 ||
			iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || !hasIPv4(iface) {
			continue
		}
		if pc.SetMulticastInterface(&iface) != nil {
			continue
		}
		if _, err := conn.WriteToUDP(query, mdnsGroup); err == nil {
			sent++
		}
	}
	return sent
}

func hasIPv4(iface net.Interface) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return true
		}
	}
	return false
}

func buildQuery() ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	for _, service := range []string{mqttService, haService} {
		name, err := dnsmessage.NewName(service)
		if err != nil {
			return nil, err
		}
		if err := b.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

// answers gathers the records of the responses: the instances of the
// services, where they are, what they say of themselves, and the hosts'
// addresses.
type answers struct {
	instances map[string]string                 // instance → service
	labels    map[string]string                 // instance → its name as written
	srv       map[string]dnsmessage.SRVResource // instance → target and port
	txt       map[string][]string               // instance → TXT strings
	addrs     map[string]string                 // host → IPv4 address
}

func newAnswers() *answers {
	return &answers{
		instances: map[string]string{},
		labels:    map[string]string{},
		srv:       map[string]dnsmessage.SRVResource{},
		txt:       map[string][]string{},
		addrs:     map[string]string{},
	}
}

// read takes the records of one response; a malformed one adds nothing.
func (a *answers) read(packet []byte) {
	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		return
	}
	if err := p.SkipAllQuestions(); err != nil {
		return
	}
	for {
		h, err := p.AnswerHeader()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil || a.record(&p, h) != nil {
			return
		}
	}
	// Responders put where an instance is among the additional records.
	if err := p.SkipAllAuthorities(); err != nil {
		return
	}
	for {
		h, err := p.AdditionalHeader()
		if err != nil || a.record(&p, h) != nil {
			return
		}
	}
}

// record reads the resource of a record whose header p just read, in any
// section; an error leaves p where it cannot go on.
func (a *answers) record(p *dnsmessage.Parser, h dnsmessage.ResourceHeader) error {
	name := strings.ToLower(h.Name.String())
	switch h.Type {
	case dnsmessage.TypePTR:
		r, err := p.PTRResource()
		if err != nil {
			return err
		}
		if name == mqttService || name == haService {
			instance := strings.ToLower(r.PTR.String())
			a.instances[instance] = name
			a.labels[instance] = instanceLabel(r.PTR.String(), len(name))
		}
	case dnsmessage.TypeSRV:
		r, err := p.SRVResource()
		if err != nil {
			return err
		}
		a.srv[name] = r
	case dnsmessage.TypeTXT:
		r, err := p.TXTResource()
		if err != nil {
			return err
		}
		a.txt[name] = r.TXT
	case dnsmessage.TypeA:
		r, err := p.AResource()
		if err != nil {
			return err
		}
		a.addrs[name] = net.IP(r.A[:]).String()
	default:
		if _, err := p.UnknownResource(); err != nil {
			return err
		}
	}
	return nil
}

// candidates are where a broker may listen: each MQTT instance's host and
// port, and Mosquitto's two ports on each Home Assistant host.
func (a *answers) candidates() []Broker {
	var out []Broker
	for instance, service := range a.instances {
		label := a.labels[instance]
		srv, hasSRV := a.srv[instance]
		host := ""
		if hasSRV {
			target := strings.ToLower(srv.Target.String())
			host = a.addrs[target]
			if host == "" {
				host = strings.TrimSuffix(target, ".")
			}
		}
		if service == haService {
			// Its internal URL names the host as the user knows it.
			if h := txtHost(a.txt[instance], "internal_url"); h != "" {
				host = h
			}
			if txt := txtValue(a.txt[instance], "location_name"); txt != "" {
				label = txt
			}
		}
		if host == "" {
			continue
		}
		if service == mqttService && hasSRV {
			out = append(out, Broker{Name: label, Host: host, Port: int(srv.Port)})
			continue
		}
		out = append(out, Broker{Name: label, Host: host, Port: 1883}, Broker{Name: label, Host: host, Port: 8883, TLS: true})
	}
	return out
}

// instanceLabel is an instance's own name, without its service's, whose
// length is serviceLen.
func instanceLabel(instance string, serviceLen int) string {
	label := instance[:max(len(instance)-serviceLen-1, 0)]
	// Names escape their dots and spaces in DNS's way.
	return strings.NewReplacer(`\032`, " ", `\.`, ".").Replace(label)
}

func txtValue(txt []string, key string) string {
	for _, kv := range txt {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func txtHost(txt []string, key string) string {
	u, err := url.Parse(txtValue(txt, key))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// probe keeps the candidates that take a TCP connection, once each, in a
// stable order.
func probe(ctx context.Context, candidates []Broker) []Broker {
	seen := map[string]bool{}
	var unique []Broker
	for _, c := range candidates {
		if !seen[c.Address()] {
			seen[c.Address()] = true
			unique = append(unique, c)
		}
	}
	var (
		mu    sync.Mutex
		found []Broker
		wg    sync.WaitGroup
	)
	dialer := net.Dialer{Timeout: 1500 * time.Millisecond}
	for _, c := range unique {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := dialer.DialContext(ctx, "tcp", c.Address())
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			found = append(found, c)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(found, func(i, j int) bool {
		if found[i].Host != found[j].Host {
			return found[i].Host < found[j].Host
		}
		return found[i].Port < found[j].Port
	})
	return found
}
