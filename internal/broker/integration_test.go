package broker

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// mqttTestServer is a deliberately tiny MQTT 3.1.1 server for transport
// tests. It accepts one client and supports CONNECT, SUBSCRIBE, PUBLISH (QoS
// 0/1), PINGREQ and DISCONNECT. It is not a general broker.
type serverMessage struct {
	topic string
	body  []byte
}

type mqttTestServer struct {
	listener net.Listener
	conn     net.Conn
	port     int

	writeMu sync.Mutex
	pubCh   chan serverMessage
	errCh   chan error
	gotSub  chan string
}

func newMQTTTestServer(t *testing.T) *mqttTestServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &mqttTestServer{
		listener: ln,
		port:     ln.Addr().(*net.TCPAddr).Port,
		pubCh:    make(chan serverMessage, 16),
		errCh:    make(chan error, 1),
		gotSub:   make(chan string, 16),
	}
	t.Cleanup(func() {
		ln.Close()
		if s.conn != nil {
			s.conn.Close()
		}
	})
	go s.serve()
	return s
}

func (s *mqttTestServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		s.errCh <- err
		return
	}
	s.conn = conn
	for {
		header, body, err := readMQTTPacket(conn)
		if err != nil {
			s.errCh <- err
			return
		}
		switch header >> 4 {
		case 1: // CONNECT
			if err := s.write([]byte{0x20, 0x02, 0x00, 0x00}); err != nil {
				s.errCh <- err
				return
			}
		case 3: // PUBLISH from client
			topic, payload, qos, packetID, err := parseMQTTPublish(header, body)
			if err != nil {
				s.errCh <- err
				return
			}
			if qos == 1 {
				if err := s.write([]byte{0x40, 0x02, byte(packetID >> 8), byte(packetID)}); err != nil {
					s.errCh <- err
					return
				}
			}
			s.pubCh <- serverMessage{topic: topic, body: payload}
		case 8: // SUBSCRIBE
			if len(body) < 4 {
				s.errCh <- fmt.Errorf("short SUBSCRIBE")
				return
			}
			id := binary.BigEndian.Uint16(body[:2])
			n := int(binary.BigEndian.Uint16(body[2:4]))
			if len(body) < 4+n {
				s.errCh <- fmt.Errorf("short SUBSCRIBE topic")
				return
			}
			topic := string(body[4 : 4+n])
			s.gotSub <- topic
			if err := s.write([]byte{0x90, 0x03, byte(id >> 8), byte(id), 0x01}); err != nil {
				s.errCh <- err
				return
			}
		case 12: // PINGREQ
			if err := s.write([]byte{0xD0, 0x00}); err != nil {
				s.errCh <- err
				return
			}
		case 14: // DISCONNECT
			return
		default:
			// Ignore packet types not used by this test.
		}
	}
}

func (s *mqttTestServer) write(packet []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.conn.Write(packet)
	return err
}

func (s *mqttTestServer) publish(topic string, payload []byte) error {
	packet := []byte{0x30}
	body := make([]byte, 0, 2+len(topic)+len(payload))
	body = append(body, byte(len(topic)>>8), byte(len(topic)))
	body = append(body, topic...)
	body = append(body, payload...)
	packet = append(packet, encodeRemainingLength(len(body))...)
	packet = append(packet, body...)
	return s.write(packet)
}

func readMQTTPacket(r io.Reader) (byte, []byte, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, nil, err
	}
	remaining, err := readRemainingLength(r)
	if err != nil {
		return 0, nil, err
	}
	body := make([]byte, remaining)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return first[0], body, nil
}

func readRemainingLength(r io.Reader) (int, error) {
	value, multiplier := 0, 1
	for i := 0; i < 4; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		value += int(b[0]&0x7f) * multiplier
		if b[0]&0x80 == 0 {
			return value, nil
		}
		multiplier *= 128
	}
	return 0, fmt.Errorf("malformed remaining length")
}

func encodeRemainingLength(n int) []byte {
	var out []byte
	for {
		digit := byte(n % 128)
		n /= 128
		if n > 0 {
			digit |= 0x80
		}
		out = append(out, digit)
		if n == 0 {
			return out
		}
	}
}

func parseMQTTPublish(header byte, body []byte) (string, []byte, int, uint16, error) {
	if len(body) < 2 {
		return "", nil, 0, 0, fmt.Errorf("short PUBLISH")
	}
	qos := int((header >> 1) & 3)
	n := int(binary.BigEndian.Uint16(body[:2]))
	if len(body) < 2+n {
		return "", nil, 0, 0, fmt.Errorf("short PUBLISH topic")
	}
	topic := string(body[2 : 2+n])
	rest := body[2+n:]
	var id uint16
	if qos > 0 {
		if len(rest) < 2 {
			return "", nil, 0, 0, fmt.Errorf("short PUBLISH packet ID")
		}
		id = binary.BigEndian.Uint16(rest[:2])
		rest = rest[2:]
	}
	return topic, append([]byte(nil), rest...), qos, id, nil
}

func TestPahoTransportRoundTrip(t *testing.T) {
	srv := newMQTTTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := Dial(Config{
		Host:           "127.0.0.1",
		Port:           srv.port,
		ClientID:       "roundtrip-test",
		KeepAlive:      10 * time.Second,
		ConnectTimeout: 2 * time.Second,
		WillTopic:      "mac2mqtt/test/availability",
		WillPayload:    "offline",
		WillRetain:     true,
	})
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Disconnect(context.Background())

	received := make(chan string, 1)
	if err := client.Subscribe(ctx, "mac2mqtt/test/+/set", 1, func(msg Message) {
		if msg.Topic == "mac2mqtt/test/volume/set" {
			received <- string(msg.Payload)
		}
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	select {
	case topic := <-srv.gotSub:
		if topic != "mac2mqtt/test/+/set" {
			t.Errorf("subscribed to %q", topic)
		}
	case <-ctx.Done():
		t.Fatal("server did not receive SUBSCRIBE")
	}

	if err := srv.publish("mac2mqtt/test/volume/set", []byte("42")); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-received:
		if payload != "42" {
			t.Errorf("received %q", payload)
		}
	case <-ctx.Done():
		t.Fatal("message callback did not run")
	}

	if err := client.Publish(ctx, "mac2mqtt/test/state", []byte("{\"ok\":true}"), true, 1); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case msg := <-srv.pubCh:
		if msg.topic != "mac2mqtt/test/state" || string(msg.body) != "{\"ok\":true}" {
			t.Errorf("server got %+v", msg)
		}
	case <-ctx.Done():
		t.Fatal("server did not receive PUBLISH")
	}
}
