package system

import (
	"context"
	"encoding/json"
	"strings"
)

// Hardware names this Mac's model, as About This Mac does.
type Hardware struct {
	Name string // "Mac mini"
	ID   string // "Mac16,10"
}

// readHardware asks system_profiler for the model's name and identifier,
// falling back to sysctl's identifier alone.
func readHardware(ctx context.Context, runner commandRunner) Hardware {
	var hw Hardware
	if output, err := runCommand(ctx, runner, "/usr/sbin/system_profiler", "SPHardwareDataType", "-json"); err == nil {
		hw = parseHardware(output)
	}
	if hw.ID == "" {
		if output, err := runCommand(ctx, runner, "/usr/sbin/sysctl", "-n", "hw.model"); err == nil {
			hw.ID = strings.TrimSpace(string(output))
		}
	}
	return hw
}

func parseHardware(output []byte) Hardware {
	var report struct {
		Hardware []struct {
			Name  string `json:"machine_name"`
			Model string `json:"machine_model"`
		} `json:"SPHardwareDataType"`
	}
	if json.Unmarshal(output, &report) != nil || len(report.Hardware) == 0 {
		return Hardware{}
	}
	return Hardware{Name: report.Hardware[0].Name, ID: report.Hardware[0].Model}
}
