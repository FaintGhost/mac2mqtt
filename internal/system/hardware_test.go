package system

import (
	"context"
	"errors"
	"testing"
)

func TestReadHardware(t *testing.T) {
	runner := &fakeRunner{runFn: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		return []byte(`{"SPHardwareDataType":[{"machine_name":"Mac mini","machine_model":"Mac16,10","chip_type":"Apple M4"}]}`), nil
	}}
	if hw := readHardware(context.Background(), runner); hw != (Hardware{Name: "Mac mini", ID: "Mac16,10"}) {
		t.Fatalf("hardware = %+v", hw)
	}
}

func TestReadHardwareFallsBackToSysctl(t *testing.T) {
	runner := &fakeRunner{runFn: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "/usr/sbin/sysctl" {
			return []byte("Mac16,10\n"), nil
		}
		return nil, errors.New("no system_profiler")
	}}
	if hw := readHardware(context.Background(), runner); hw != (Hardware{ID: "Mac16,10"}) {
		t.Fatalf("hardware = %+v", hw)
	}
}
