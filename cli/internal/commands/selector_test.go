package commands

import (
	"testing"
)

func TestSelectOption(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opt := selectOption{Label: "test label", Value: "test-value"}
	if opt.Label != "test label" {
		t.Errorf("Label = %q, want %q", opt.Label, "test label")
	}
	if opt.Value != "test-value" {
		t.Errorf("Value = %q, want %q", opt.Value, "test-value")
	}
}

func TestParseIndex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		input   string
		max     int
		wantIdx int
		wantErr bool
	}{
		{"1", 3, 0, false},
		{"2", 3, 1, false},
		{"3", 3, 2, false},
		{"0", 3, 0, true},
		{"4", 3, 0, true},
		{"-1", 3, 0, true},
		{"abc", 3, 0, true},
		{"", 3, 0, true},
	}

	for _, tt := range tests {
		idx, err := parseIndex(tt.input, tt.max)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseIndex(%q, %d) error = %v, wantErr %v", tt.input, tt.max, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && idx != tt.wantIdx {
			t.Errorf("parseIndex(%q, %d) = %d, want %d", tt.input, tt.max, idx, tt.wantIdx)
		}
	}
}
