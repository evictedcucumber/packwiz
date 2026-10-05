package cmdshared

import (
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

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

func TestPromptYesNoDefaultNeedsTheOtherAnswerGivenAsSuch(t *testing.T) {
	for _, tc := range []struct {
		answer string
		def    bool
		want   bool
	}{
		{"\n", false, false}, {"y\n", false, true}, {"yes\n", false, true}, {"maybe\n", false, false},
		{"\n", true, true}, {"n\n", true, false}, {"maybe\n", true, true},
	} {
		cmdtest.SetStdin(t, tc.answer)
		var got bool
		cmdtest.CaptureStdout(t, func() { got = PromptYesNoDefault("Sure? [y/N]", tc.def) })
		if got != tc.want {
			t.Errorf("PromptYesNoDefault() with default %v answered %q = %v, want %v", tc.def, tc.answer, got, tc.want)
		}
	}
}

func TestReadLineReadsWhatPromptsLeave(t *testing.T) {
	cmdtest.SetStdin(t, "y\nstart\r\nlast")
	cmdtest.CaptureStdout(t, func() { PromptYesNo("Sure?") })
	for _, want := range []string{"start", "last"} {
		if got, _ := ReadLine(); got != want {
			t.Errorf("ReadLine() = %q, want %q", got, want)
		}
	}
	if _, err := ReadLine(); err == nil {
		t.Error("ReadLine() at the end returned no error")
	}
}
