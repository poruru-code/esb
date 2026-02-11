// Where: cli/internal/command/deploy_entry_emoji_test.go
// What: Unit tests for deploy emoji resolution.
// Why: Lock down flag/env precedence and conflict handling.
package command

import (
	"bytes"
	"testing"
)

func TestResolveDeployEmojiEnabled(t *testing.T) {
	tests := []struct {
		name      string
		flags     DeployCmd
		noEmoji   string
		term      string
		want      bool
		expectErr bool
	}{
		{
			name:      "conflicting flags",
			flags:     DeployCmd{Emoji: true, NoEmoji: true},
			expectErr: true,
		},
		{
			name:  "emoji flag has priority",
			flags: DeployCmd{Emoji: true},
			want:  true,
		},
		{
			name:  "no emoji flag has priority",
			flags: DeployCmd{NoEmoji: true},
			want:  false,
		},
		{
			name:    "no emoji env disables emoji",
			noEmoji: "1",
			want:    false,
		},
		{
			name: "dumb terminal disables emoji",
			term: "dumb",
			want: false,
		},
		{
			name: "non file writer falls back to disabled",
			term: "xterm-256color",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_EMOJI", tt.noEmoji)
			if tt.term == "" {
				t.Setenv("TERM", "xterm-256color")
			} else {
				t.Setenv("TERM", tt.term)
			}
			got, err := resolveDeployEmojiEnabled(&bytes.Buffer{}, tt.flags)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve emoji: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}
