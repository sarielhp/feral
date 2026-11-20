package cfg_g

import "testing"

// The TUI watches for new mail unless config.json says "tui_watch": false.
func TestTuiWatchDefaultsOnAndFollowsTheFile(t *testing.T) {
	off, on := false, true
	cases := []struct {
		name string
		file *bool
		want bool
	}{
		{"unset", nil, true},
		{"false", &off, false},
		{"true", &on, true},
	}
	for _, tc := range cases {
		config := newDefaultConfig(t.TempDir())
		applyFileFilterAndToolConfig(config, &FileConfig{TuiWatch: tc.file})
		if config.TuiWatch != tc.want {
			t.Errorf("%s: TuiWatch = %v, want %v", tc.name, config.TuiWatch, tc.want)
		}
	}
}
