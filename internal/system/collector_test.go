package system

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCollectorSnapshot(t *testing.T) {
	runner := &fakeRunner{runFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "/usr/bin/sw_vers":
			return []byte("14.5\n"), nil
		case "/usr/bin/pmset":
			return []byte("Now drawing from 'AC Power'\n -InternalBattery-0 (id=1)\t87%; charging; 1:23 remaining present: true\n"), nil
		case "/usr/sbin/system_profiler":
			return []byte(`{"SPHardwareDataType":[{"machine_name":"Mac mini","machine_model":"Mac16,10"}]}`), nil
		case "/usr/sbin/ioreg":
			return []byte(`"IOConsoleUsers" = ({"CGSSessionScreenIsLocked"=Yes,"kCGSSessionOnConsoleKey"=Yes,"kCGSSessionUserIDKey"=` + strconv.Itoa(os.Getuid()) + `})`), nil
		case "/usr/bin/osascript":
			if args[0] == "-l" {
				return []byte(`{"title":"Song","artist":"Artist","playing":true}`), nil
			}
			return []byte("output volume:33, input volume:100, alert volume:100, output muted:true\n"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}}
	collector := newCollector("1.2.3", "darwin", runner)
	collector.SetLocked(true)

	before := time.Now()
	snapshot := collector.Snapshot(context.Background(), Runtime{
		NetworkOnline:  true,
		NetworkMetered: true,
		OnBattery:      true,
		Idle:           90 * time.Second,
	})
	after := time.Now()

	if snapshot.LastSeen.Before(before) || snapshot.LastSeen.After(after) {
		t.Fatalf("LastSeen = %v, want between %v and %v", snapshot.LastSeen, before, after)
	}
	hostname, _ := os.Hostname()
	requireEqual(t, "hostname", snapshot.Device.Hostname, hostname)
	requireEqual(t, "app version", snapshot.App.Version, "1.2.3")
	requireEqual(t, "OS version", snapshot.Device.OSVersion, "14.5")
	requireEqual(t, "architecture", snapshot.Device.Arch, runtime.GOARCH)
	requireEqual(t, "model", snapshot.Device.Model, "Mac mini")
	requireEqual(t, "model ID", snapshot.Device.ModelID, "Mac16,10")
	requireEqual(t, "network online", snapshot.Network.Online, true)
	requireEqual(t, "network metered", snapshot.Network.Metered, true)
	requireEqual(t, "on battery", snapshot.Power.OnBattery, true)
	requireEqual(t, "power source", snapshot.Power.Source, "AC")
	requireEqual(t, "battery percent", sameFloatPtr(snapshot.Power.BatteryPercent, floatPtr(87)), true)
	requireEqual(t, "charging", sameBoolPtr(snapshot.Power.Charging, boolPtr(true)), true)
	requireEqual(t, "locked", snapshot.Session.Locked, true)
	requireEqual(t, "idle seconds", snapshot.Session.IdleSeconds, int64(90))
	if snapshot.App.UptimeSeconds < 0 {
		t.Fatalf("uptime = %d, want non-negative", snapshot.App.UptimeSeconds)
	}
	if snapshot.Audio == nil {
		t.Fatal("audio = nil, want parsed audio state")
	}
	requireEqual(t, "audio volume", snapshot.Audio.Volume, 33)
	requireEqual(t, "audio muted", snapshot.Audio.Muted, true)
	if snapshot.Media == nil || snapshot.Media.Title != "Song" || snapshot.Media.State != "playing" {
		t.Fatalf("media = %+v, want playing Song", snapshot.Media)
	}

	calls := runner.Calls()
	requireEqual(t, "first snapshot command count", len(calls), 6)
	wantCommands := map[string][]string{
		"/usr/bin/sw_vers":   {"-productVersion"},
		"/usr/bin/pmset":     {"-g", "batt"},
		"/usr/bin/osascript": {"-e", "get volume settings"},
	}
	for _, call := range calls {
		if call.name == "/usr/bin/osascript" && call.args[0] == "-l" || call.name == "/usr/sbin/ioreg" || call.name == "/usr/sbin/system_profiler" {
			continue // tested in media_test.go and session_test.go
		}
		wantArgs, found := wantCommands[call.name]
		requireEqual(t, "known command", found, true)
		requireEqual(t, "command args for "+call.name, reflect.DeepEqual(call.args, wantArgs), true)
	}

	second := collector.Snapshot(context.Background(), Runtime{})
	if second.Device.OSVersion == "14.5" {
	} else {
		t.Fatalf("cached OS version = %q, want 14.5", second.Device.OSVersion)
	}
	swVersCalls := 0
	for _, call := range runner.Calls() {
		if call.name == "/usr/bin/sw_vers" {
			swVersCalls++
		}
	}
	requireEqual(t, "sw_vers call count after cache", swVersCalls, 1)
}

func TestCollectorUptime(t *testing.T) {
	collector := newCollector("dev", "linux", &fakeRunner{})
	if got := collector.Snapshot(context.Background(), Runtime{}).Device.UptimeSeconds; got != 0 {
		t.Fatalf("uptime without a boot time = %d, want 0", got)
	}
	collector.bootTime = func() (time.Time, error) { return time.Now().Add(-3 * time.Hour), nil }
	if got := collector.Snapshot(context.Background(), Runtime{}).Device.UptimeSeconds; got < 3*3600-60 || got > 3*3600+60 {
		t.Fatalf("uptime = %d, want about %d", got, 3*3600)
	}
	collector.bootTime = func() (time.Time, error) { return time.Time{}, errors.New("no sysctl") }
	if got := collector.Snapshot(context.Background(), Runtime{}).Device.UptimeSeconds; got != 0 {
		t.Fatalf("uptime after an error = %d, want 0", got)
	}
}

func TestCollectorNonDarwinMakesNoCommands(t *testing.T) {
	runner := &fakeRunner{}
	collector := newCollector("dev", "linux", runner)

	snapshot := collector.Snapshot(context.Background(), Runtime{OnBattery: true})

	requireEqual(t, "command count", len(runner.Calls()), 0)
	requireEqual(t, "OS version", snapshot.Device.OSVersion, "linux")
	requireEqual(t, "power source", snapshot.Power.Source, "Battery")
	requireEqual(t, "on battery", snapshot.Power.OnBattery, true)
	requireEqual(t, "battery percent absent", sameFloatPtr(snapshot.Power.BatteryPercent, nil), true)
	requireEqual(t, "charging absent", sameBoolPtr(snapshot.Power.Charging, nil), true)
	if snapshot.Audio == nil {
		return
	}
	t.Fatal("audio = non-nil on non-macOS")
}

func TestCollectorCommandFailuresAreTolerated(t *testing.T) {
	runner := &fakeRunner{runFn: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}}
	collector := newCollector("dev", "darwin", runner)

	snapshot := collector.Snapshot(context.Background(), Runtime{OnBattery: false})

	requireEqual(t, "OS version fallback", snapshot.Device.OSVersion, "darwin")
	requireEqual(t, "power source fallback", snapshot.Power.Source, "AC")
	requireEqual(t, "battery percent absent", sameFloatPtr(snapshot.Power.BatteryPercent, nil), true)
	requireEqual(t, "charging absent", sameBoolPtr(snapshot.Power.Charging, nil), true)
	if snapshot.Audio == nil {
		return
	}
	t.Fatal("audio = non-nil after command failure")
}

func TestCollectorLockingIsConcurrentSafe(t *testing.T) {
	collector := newCollector("dev", "linux", &fakeRunner{})
	var group sync.WaitGroup
	for index := 0; index < 100; index++ {
		group.Add(1)
		go func(value bool) {
			defer group.Done()
			collector.SetLocked(value)
			_ = collector.Snapshot(context.Background(), Runtime{})
		}(index%2 == 0)
	}
	group.Wait()

	collector.SetLocked(true)
	requireEqual(t, "locked", collector.Snapshot(context.Background(), Runtime{}).Session.Locked, true)
}
