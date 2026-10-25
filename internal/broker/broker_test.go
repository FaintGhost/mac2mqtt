package broker

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func validConfig() Config {
	return Config{
		Host:           "broker.example.com",
		Port:           1883,
		ClientID:       "test-client",
		KeepAlive:      30 * time.Second,
		ConnectTimeout: 5 * time.Second,
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing host", func(c *Config) { c.Host = "  " }},
		{"zero port", func(c *Config) { c.Port = 0 }},
		{"port too high", func(c *Config) { c.Port = 65536 }},
		{"missing client ID", func(c *Config) { c.ClientID = "" }},
		{"negative keep alive", func(c *Config) { c.KeepAlive = -time.Second }},
		{"negative connect timeout", func(c *Config) { c.ConnectTimeout = -time.Second }},
		{"URL in host", func(c *Config) { c.Host = "tcp://broker.example.com" }},
	}

	if err := validConfig().validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestEffectiveDefaults(t *testing.T) {
	if got := effectiveKeepAlive(0); got != defaultKeepAlive {
		t.Fatalf("keep alive default = %v, want %v", got, defaultKeepAlive)
	}
	if got := effectiveKeepAlive(45 * time.Second); got != 45*time.Second {
		t.Fatalf("keep alive = %v, want 45s", got)
	}
	c := &pahoClient{cfg: Config{}}
	if got := c.connectionTimeout(); got != defaultConnectTimeout {
		t.Fatalf("connect timeout default = %v, want %v", got, defaultConnectTimeout)
	}
	c.cfg.ConnectTimeout = 3 * time.Second
	if got := c.connectionTimeout(); got != 3*time.Second {
		t.Fatalf("connect timeout = %v, want 3s", got)
	}
}

func TestTLSConfig(t *testing.T) {
	tlsCfg := newTLSConfig(Config{Host: "broker.example.com"})
	if tlsCfg.ServerName != "broker.example.com" {
		t.Fatalf("ServerName = %q", tlsCfg.ServerName)
	}
	if tlsCfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify unexpectedly enabled")
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %d, want TLS 1.2", tlsCfg.MinVersion)
	}
}

func TestTLSServerName(t *testing.T) {
	for _, host := range []string{"192.0.2.10", "2001:db8::1", "[2001:db8::1]"} {
		if got := newTLSConfig(Config{Host: host}).ServerName; got != "" {
			t.Errorf("IP host %q set ServerName %q", host, got)
		}
	}
	insecure := newTLSConfig(Config{Host: "broker.example.com", InsecureSkipVerify: true})
	if insecure.InsecureSkipVerify == false {
		t.Fatal("insecure flag not honored")
	}
}

func TestBrokerURL(t *testing.T) {
	if got := brokerURL(Config{Host: "broker.local", Port: 1883}); got != "tcp://broker.local:1883" {
		t.Errorf("plain URL = %q", got)
	}
	if got := brokerURL(Config{Host: "broker.local", Port: 8883, TLS: true}); got != "ssl://broker.local:8883" {
		t.Errorf("TLS URL = %q", got)
	}
	if got := brokerURL(Config{Host: "2001:db8::1", Port: 1883}); got != "tcp://[2001:db8::1]:1883" {
		t.Errorf("IPv6 URL = %q", got)
	}
}

func TestValidateTopicAndQoS(t *testing.T) {
	if err := validateTopic(""); err == nil {
		t.Error("empty topic accepted")
	}
	if err := validateTopic("mac2mqtt/test/state"); err != nil {
		t.Errorf("valid topic rejected: %v", err)
	}
	for _, qos := range []byte{0, 1, 2} {
		if err := validateQoS(qos); err != nil {
			t.Errorf("QoS %d rejected: %v", qos, err)
		}
	}
	if err := validateQoS(3); err == nil {
		t.Error("QoS 3 accepted")
	}
}

type testMessage struct {
	topic   string
	payload []byte
}

func (m testMessage) Duplicate() bool   { return false }
func (m testMessage) Qos() byte         { return 0 }
func (m testMessage) Retained() bool    { return false }
func (m testMessage) Topic() string     { return m.topic }
func (m testMessage) MessageID() uint16 { return 0 }
func (m testMessage) Payload() []byte   { return m.payload }
func (m testMessage) Ack()              {}

var _ mqtt.Message = testMessage{}

func TestCopyMessageDetachesPayload(t *testing.T) {
	src := []byte("payload")
	got := copyMessage(testMessage{topic: "topic", payload: src})
	src[0] = 'X'
	if got.Topic != "topic" || string(got.Payload) != "payload" {
		t.Errorf("copyMessage = %+v", got)
	}
}

func TestNewClientIDIsUnique(t *testing.T) {
	a, b := newClientID(), newClientID()
	if a == b {
		t.Fatalf("duplicate client IDs: %q", a)
	}
	if strings.HasPrefix(a, "mac2mqtt-") == false {
		t.Errorf("client ID = %q", a)
	}
}

func TestWaitTokenReturnsContextError(t *testing.T) {
	token := &fakeToken{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitToken(ctx, token); errors.Is(err, context.Canceled) == false {
		t.Errorf("waitToken error = %v, want context.Canceled", err)
	}
}

type fakeToken struct {
	done chan struct{}
	err  error
}

func (t *fakeToken) Wait() bool                     { <-t.done; return true }
func (t *fakeToken) WaitTimeout(time.Duration) bool { return t.Wait() }
func (t *fakeToken) Done() <-chan struct{}          { return t.done }
func (t *fakeToken) Error() error                   { return t.err }
