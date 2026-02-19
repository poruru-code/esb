package interaction

import (
	"errors"
	"testing"

	"github.com/charmbracelet/huh"
)

func TestHuhPrompterInputUsesRunner(t *testing.T) {
	orig := runInputPrompt
	t.Cleanup(func() { runInputPrompt = orig })

	var gotTitle string
	var gotSuggestions []string
	runInputPrompt = func(title string, suggestions []string, input *string) error {
		gotTitle = title
		gotSuggestions = append([]string(nil), suggestions...)
		*input = "dev"
		return nil
	}

	got, err := (HuhPrompter{}).Input("Select env", []string{"dev", "staging"})
	if err != nil {
		t.Fatalf("Input() error = %v", err)
	}
	if got != "dev" {
		t.Fatalf("Input() = %q, want %q", got, "dev")
	}
	if gotTitle != "Select env" {
		t.Fatalf("title = %q", gotTitle)
	}
	if len(gotSuggestions) != 2 || gotSuggestions[0] != "dev" || gotSuggestions[1] != "staging" {
		t.Fatalf("suggestions = %#v", gotSuggestions)
	}
}

func TestHuhPrompterInputWrapsError(t *testing.T) {
	orig := runInputPrompt
	t.Cleanup(func() { runInputPrompt = orig })
	runInputPrompt = func(string, []string, *string) error {
		return errors.New("tty unavailable")
	}

	_, err := (HuhPrompter{}).Input("Select env", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "prompt input: tty unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHuhPrompterSelectUsesRunner(t *testing.T) {
	orig := runSelectPrompt
	t.Cleanup(func() { runSelectPrompt = orig })

	var gotTitle string
	var gotOptions int
	runSelectPrompt = func(title string, options []huh.Option[string], selected *string) error {
		gotTitle = title
		gotOptions = len(options)
		*selected = "containerd"
		return nil
	}

	got, err := (HuhPrompter{}).Select("Mode", []string{"docker", "containerd"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got != "containerd" {
		t.Fatalf("Select() = %q, want %q", got, "containerd")
	}
	if gotTitle != "Mode" {
		t.Fatalf("title = %q", gotTitle)
	}
	if gotOptions != 2 {
		t.Fatalf("options len = %d, want 2", gotOptions)
	}
}

func TestHuhPrompterSelectWrapsError(t *testing.T) {
	orig := runSelectPrompt
	t.Cleanup(func() { runSelectPrompt = orig })
	runSelectPrompt = func(string, []huh.Option[string], *string) error {
		return errors.New("select failed")
	}

	_, err := (HuhPrompter{}).Select("Mode", []string{"docker"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "prompt select: select failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHuhPrompterSelectValueEmptyOptionsReturnsEmpty(t *testing.T) {
	orig := runSelectPrompt
	t.Cleanup(func() { runSelectPrompt = orig })
	called := false
	runSelectPrompt = func(string, []huh.Option[string], *string) error {
		called = true
		return nil
	}

	got, err := (HuhPrompter{}).SelectValue("Runtime", nil)
	if err != nil {
		t.Fatalf("SelectValue() error = %v", err)
	}
	if got != "" {
		t.Fatalf("SelectValue() = %q, want empty", got)
	}
	if called {
		t.Fatal("runner must not be called for empty options")
	}
}

func TestHuhPrompterSelectValueWrapsError(t *testing.T) {
	orig := runSelectPrompt
	t.Cleanup(func() { runSelectPrompt = orig })
	runSelectPrompt = func(string, []huh.Option[string], *string) error {
		return errors.New("select value failed")
	}

	_, err := (HuhPrompter{}).SelectValue("Runtime", []SelectOption{{Label: "python", Value: "python3.12"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "prompt select value: select value failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}
