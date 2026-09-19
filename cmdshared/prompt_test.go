package cmdshared

import (
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/spf13/viper"
)

func TestPromptYesNoNonInteractiveAlwaysYes(t *testing.T) {
	old := viper.GetBool("non-interactive")
	viper.Set("non-interactive", true)
	t.Cleanup(func() { viper.Set("non-interactive", old) })

	if !PromptYesNo("Continue? ") {
		t.Error("PromptYesNo() = false, want true in non-interactive mode")
	}
}

// Piped-in input answers every prompt of a command, so one prompt must not use up the answers to the ones after it
func TestPromptYesNoReadsOneAnswerPerLine(t *testing.T) {
	cmdtest.SetStdin(t, "y\nn\n\nno\nYes\n  N  \n")

	for i, want := range []bool{true, false, true, false, true, false} {
		if got := PromptYesNo("Continue? "); got != want {
			t.Errorf("answer %d: PromptYesNo() = %v, want %v", i+1, got, want)
		}
	}
}

func TestPromptYesNoAcceptsLastAnswerWithoutNewline(t *testing.T) {
	cmdtest.SetStdin(t, "y\nn") // as printf 'y\nn' pipes it

	if !PromptYesNo("First? ") {
		t.Error("PromptYesNo() = false, want true for the first answer")
	}
	if PromptYesNo("Second? ") {
		t.Error("PromptYesNo() = true, want false for the last answer, which has no newline")
	}
}

// A replaced os.Stdin must be read from, not the leftovers of the one before it
func TestPromptYesNoReadsFromReplacedStdin(t *testing.T) {
	cmdtest.SetStdin(t, "n\nn\n")
	if PromptYesNo("First? ") {
		t.Fatal("PromptYesNo() = true, want false for the first answer")
	}

	cmdtest.SetStdin(t, "y\n")
	if !PromptYesNo("Second? ") {
		t.Error("PromptYesNo() = false, want the answer of the new stdin rather than the old one's leftover")
	}
}
