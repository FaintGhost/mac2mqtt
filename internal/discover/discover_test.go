package discover

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func name(t *testing.T, s string) dnsmessage.Name {
	t.Helper()
	n, err := dnsmessage.NewName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// response builds what a Home Assistant answers, as captured on a home
// network: the instance, then where it is and what it says of itself
// among the additional records, an AAAA record among them.
func response(t *testing.T) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	b.EnableCompression()
	head := func(n string, typ dnsmessage.Type) dnsmessage.ResourceHeader {
		return dnsmessage.ResourceHeader{Name: name(t, n), Type: typ, Class: dnsmessage.ClassINET, TTL: 120}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	instance := `My\032Home._home-assistant._tcp.local.`
	must(b.StartAnswers())
	must(b.PTRResource(head(haService, dnsmessage.TypePTR), dnsmessage.PTRResource{PTR: name(t, instance)}))
	must(b.StartAdditionals())
	must(b.SRVResource(head(instance, dnsmessage.TypeSRV), dnsmessage.SRVResource{Target: name(t, "93e2.local."), Port: 8123}))
	must(b.TXTResource(head(instance, dnsmessage.TypeTXT), dnsmessage.TXTResource{TXT: []string{"location_name=Home", "internal_url=http://192.168.50.86:8123"}}))
	must(b.AAAAResource(head("93e2.local.", dnsmessage.TypeAAAA), dnsmessage.AAAAResource{}))
	must(b.AResource(head("93e2.local.", dnsmessage.TypeA), dnsmessage.AResource{A: [4]byte{192, 168, 50, 86}}))
	packet, err := b.Finish()
	must(err)
	return packet
}

func TestHomeAssistantsHostIsWhereMosquittoMayListen(t *testing.T) {
	a := newAnswers()
	a.read(response(t))
	got := a.candidates()
	want := []Broker{
		{Name: "Home", Host: "192.168.50.86", Port: 1883},
		{Name: "Home", Host: "192.168.50.86", Port: 8883, TLS: true},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("candidates = %+v, want %+v", got, want)
	}
}

func TestMalformedResponsesAddNothing(t *testing.T) {
	packet := response(t)
	for _, p := range [][]byte{nil, {1, 2, 3}, packet[:20]} {
		a := newAnswers()
		a.read(p)
		if c := a.candidates(); len(c) != 0 {
			t.Errorf("candidates from %d bytes = %+v", len(p), c)
		}
	}
}
