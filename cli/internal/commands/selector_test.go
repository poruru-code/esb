package commands

import (
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
)

func TestSelectOption(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opt := interaction.SelectOption{Label: "test label", Value: "test-value"}
	if opt.Label != "test label" {
		t.Errorf("Label = %q, want %q", opt.Label, "test label")
	}
	if opt.Value != "test-value" {
		t.Errorf("Value = %q, want %q", opt.Value, "test-value")
	}
}
