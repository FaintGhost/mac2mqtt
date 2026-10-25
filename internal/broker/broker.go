// Package broker provides the small MQTT transport surface used by Mac2MQTT.
//
// The abstraction intentionally keeps reconnection policy outside of the
// transport. The service layer owns the reconnect loop and can create a fresh
// paho client for every connection attempt.
package broker

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// The defaults tolerate a slow, lossy link, such as a VPN's relay with
// hundreds of milliseconds of round trip: a connection is given up only
// when the broker has not answered a ping for PingTimeout after a quiet
// KeepAlive, not when one packet is late.
const (
	defaultKeepAlive      = 60 * time.Second
	defaultConnectTimeout = 30 * time.Second
	pingTimeout           = 20 * time.Second
	writeTimeout          = 30 * time.Second
	defaultWillQoS        = byte(1)
)

// Config describes one MQTT broker connection.
type Config struct {
	Host               string
	Port               int
	TLS                bool
	InsecureSkipVerify bool
	Username           string
	Password           string
	ClientID           string
	KeepAlive          time.Duration
	ConnectTimeout     time.Duration
	WillTopic          string
	WillPayload        string
	WillRetain         bool
}

// Message is an inbound MQTT message. Payload is owned by the receiver.
type Message struct {
	Topic   string
	Payload []byte
}

// Client is the MQTT transport used by the status service. Implementations
// must be safe for the calls made by the service: Connect once per instance,
// then Publish/Subscribe/Disconnect, with Lost observed to drive reconnects.
type Client interface {
	Connect(ctx context.Context) error
	Publish(ctx context.Context, topic string, payload []byte, retain bool, qos byte) error
	// PublishAll publishes the messages in order without waiting for each
	// to be acknowledged before sending the next, then waits for them all:
	// over a slow link, a batch takes about one round trip, not one each.
	PublishAll(ctx context.Context, messages []Publication) error
	Subscribe(ctx context.Context, filter string, qos byte, handler func(Message)) error
	// SubscribeAll subscribes to the filters in one request.
	SubscribeAll(ctx context.Context, filters []string, qos byte, handler func(Message)) error
	Disconnect(ctx context.Context) error
	Lost() <-chan struct{} // closed when the connection is lost
}

// Publication is one message of PublishAll.
type Publication struct {
	Topic   string
	Payload []byte
	Retain  bool
	QoS     byte
}

type pahoClient struct {
	cfg            Config
	client         mqtt.Client
	lost           chan struct{}
	closeOnce      sync.Once
	disconnectOnce sync.Once
	disconnectDone chan struct{}
}

// Dial constructs an MQTT client. Configuration errors are reported by
// Connect, keeping the requested public constructor signature.
func Dial(cfg Config) Client {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.ClientID == "" {
		cfg.ClientID = newClientID()
	}

	c := &pahoClient{
		cfg:            cfg,
		lost:           make(chan struct{}),
		disconnectDone: make(chan struct{}),
	}
	c.client = mqtt.NewClient(c.options())
	return c
}

func (c *pahoClient) Connect(ctx context.Context) error {
	if err := c.cfg.validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Put a hard upper bound on the wait for the CONNACK. paho also receives
	// the same timeout so the underlying dial cannot keep running indefinitely.
	timeout := c.connectionTimeout()
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return waitToken(waitCtx, c.client.Connect())
}

func (c *pahoClient) Publish(ctx context.Context, topic string, payload []byte, retain bool, qos byte) error {
	if err := validateTopic(topic); err != nil {
		return err
	}
	if err := validateQoS(qos); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return waitToken(ctx, c.client.Publish(topic, qos, retain, payload))
}

func (c *pahoClient) PublishAll(ctx context.Context, messages []Publication) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, m := range messages {
		if err := validateTopic(m.Topic); err != nil {
			return err
		}
		if err := validateQoS(m.QoS); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// paho sends them in order, from one goroutine.
	tokens := make([]mqtt.Token, len(messages))
	for i, m := range messages {
		tokens[i] = c.client.Publish(m.Topic, m.QoS, m.Retain, m.Payload)
	}
	for i, token := range tokens {
		if err := waitToken(ctx, token); err != nil {
			return fmt.Errorf("broker: publish %s: %w", messages[i].Topic, err)
		}
	}
	return nil
}

func (c *pahoClient) SubscribeAll(ctx context.Context, filters []string, qos byte, handler func(Message)) error {
	if len(filters) == 0 {
		return nil
	}
	if err := validateQoS(qos); err != nil {
		return err
	}
	if handler == nil {
		return fmt.Errorf("broker: subscribe handler must not be nil")
	}
	byFilter := make(map[string]byte, len(filters))
	for _, filter := range filters {
		if err := validateTopic(filter); err != nil {
			return err
		}
		byFilter[filter] = qos
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	wrapped := func(_ mqtt.Client, msg mqtt.Message) {
		handler(copyMessage(msg))
	}
	return waitToken(ctx, c.client.SubscribeMultiple(byFilter, wrapped))
}

func (c *pahoClient) Subscribe(ctx context.Context, filter string, qos byte, handler func(Message)) error {
	if err := validateTopic(filter); err != nil {
		return err
	}
	if err := validateQoS(qos); err != nil {
		return err
	}
	if handler == nil {
		return fmt.Errorf("broker: subscribe handler must not be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	wrapped := func(_ mqtt.Client, msg mqtt.Message) {
		handler(copyMessage(msg))
	}
	return waitToken(ctx, c.client.Subscribe(filter, qos, wrapped))
}

func (c *pahoClient) Disconnect(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// paho's Disconnect has no context-aware form. Run it once and bound the
	// caller's wait by the supplied context; repeated calls are harmless.
	c.disconnectOnce.Do(func() {
		go func() {
			defer close(c.disconnectDone)
			c.client.Disconnect(0)
		}()
	})

	select {
	case <-c.disconnectDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Lost returns a channel that is closed when paho reports an unexpected
// connection loss. Closing is guarded so duplicate callbacks are safe.
func (c *pahoClient) Lost() <-chan struct{} {
	return c.lost
}

func (c *pahoClient) markLost() {
	c.closeOnce.Do(func() {
		close(c.lost)
	})
}

func (c *pahoClient) connectionTimeout() time.Duration {
	if c.cfg.ConnectTimeout > 0 {
		return c.cfg.ConnectTimeout
	}
	return defaultConnectTimeout
}

func (c *pahoClient) options() *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL(c.cfg)).
		SetClientID(c.cfg.ClientID).
		SetCleanSession(true).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetResumeSubs(false).
		SetOrderMatters(false).
		SetConnectTimeout(c.connectionTimeout()).
		SetKeepAlive(effectiveKeepAlive(c.cfg.KeepAlive)).
		SetPingTimeout(pingTimeout).
		SetWriteTimeout(writeTimeout).
		SetConnectionLostHandler(func(_ mqtt.Client, _ error) {
			c.markLost()
		})

	if c.cfg.Username != "" {
		opts.SetUsername(c.cfg.Username)
	}
	if c.cfg.Password != "" {
		opts.SetPassword(c.cfg.Password)
	}
	if c.cfg.TLS {
		opts.SetTLSConfig(newTLSConfig(c.cfg))
	}
	if c.cfg.WillTopic != "" {
		opts.SetWill(c.cfg.WillTopic, c.cfg.WillPayload, defaultWillQoS, c.cfg.WillRetain)
	}
	return opts
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("broker: host must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("broker: port must be between 1 and 65535")
	}
	if c.ClientID == "" {
		return fmt.Errorf("broker: client ID must not be empty")
	}
	if c.KeepAlive < 0 {
		return fmt.Errorf("broker: keep alive must not be negative")
	}
	if c.ConnectTimeout < 0 {
		return fmt.Errorf("broker: connect timeout must not be negative")
	}
	if strings.ContainsAny(c.Host, "/?#") {
		return fmt.Errorf("broker: host must not contain a URL scheme or path")
	}
	return nil
}

func effectiveKeepAlive(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultKeepAlive
	}
	return d
}

func newTLSConfig(cfg Config) *tls.Config {
	// A nil RootCAs means crypto/tls uses the platform trust store. Prefer an
	// explicit system pool when available, but never turn a platform lookup
	// issue into a failed connection attempt.
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = nil
	}

	tlsCfg := &tls.Config{
		RootCAs:            roots,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // explicitly configured by the user
		MinVersion:         tls.VersionTLS12,
	}
	if host := tlsServerName(cfg.Host); host != "" {
		tlsCfg.ServerName = host
	}
	return tlsCfg
}

func tlsServerName(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	return host
}

func brokerURL(cfg Config) string {
	scheme := "tcp"
	if cfg.TLS {
		scheme = "ssl"
	}
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(cfg.Host), "["), "]")
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(cfg.Port)))
}

// copyMessage detaches inbound payload bytes from paho's reusable packet
// buffer. Handlers that retain messages therefore remain safe.
func copyMessage(msg mqtt.Message) Message {
	return Message{
		Topic:   msg.Topic(),
		Payload: append([]byte(nil), msg.Payload()...),
	}
}

func newClientID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// This is only an identifier, not a security boundary. Uniqueness per
		// process is sufficient if the system random source is unavailable.
		return fmt.Sprintf("mac2mqtt-%d", time.Now().UnixNano())
	}
	return "mac2mqtt-" + hex.EncodeToString(b[:])
}

func waitToken(ctx context.Context, token mqtt.Token) error {
	if token == nil {
		return fmt.Errorf("broker: nil MQTT token")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		return token.Error()
	}
}

func validateTopic(topic string) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("broker: topic must not be empty")
	}
	return nil
}

func validateQoS(qos byte) error {
	if qos > 2 {
		return fmt.Errorf("broker: QoS must be 0, 1, or 2")
	}
	return nil
}
