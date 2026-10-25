package system

import "testing"

func TestParsePMSetFormats(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantSource   string
		wantPercent  *float64
		wantCharging *bool
	}{
		{
			name:         "ac charging",
			output:       "Now drawing from 'AC Power'\n -InternalBattery-0 (id=1234)\t87%; charging; 1:23 remaining present: true\n",
			wantSource:   "AC",
			wantPercent:  floatPtr(87),
			wantCharging: boolPtr(true),
		},
		{
			name:         "battery discharging",
			output:       "Now drawing from 'Battery Power'\n -InternalBattery-0 (id=1234)\t45%; discharging; 2:10 remaining present: true\n",
			wantSource:   "Battery",
			wantPercent:  floatPtr(45),
			wantCharging: boolPtr(false),
		},
		{
			name:         "charged",
			output:       "Now drawing from 'AC Power'\n -InternalBattery-0 (id=1234)\t100%; charged; 0:00 remaining present: true\n",
			wantSource:   "AC",
			wantPercent:  floatPtr(100),
			wantCharging: boolPtr(false),
		},
		{
			name:         "ac attached",
			output:       "Now drawing from 'AC Power'\n -InternalBattery-0 (id=1234)\t76%; AC attached; not charging present: true\n",
			wantSource:   "AC",
			wantPercent:  floatPtr(76),
			wantCharging: boolPtr(false),
		},
		{
			name:       "absent battery",
			output:     "Now drawing from 'AC Power'\n",
			wantSource: "AC",
		},
		{
			name:       "malformed",
			output:     "unexpected output\n",
			wantSource: "",
		},
		{
			name:       "malformed battery line",
			output:     "Now drawing from 'Battery Power'\n -InternalBattery-0 (id=1) nope%; charging;\n",
			wantSource: "Battery",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parsePMSet(test.output)
			requireEqual(t, "source", got.source, test.wantSource)
			requireEqual(t, "percent", sameFloatPtr(got.batteryPercent, test.wantPercent), true)
			requireEqual(t, "charging", sameBoolPtr(got.charging, test.wantCharging), true)
		})
	}
}
