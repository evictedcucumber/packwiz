package cmdshared

import (
	"testing"

	"github.com/spf13/viper"
)

func TestPromptYesNoNonInteractiveAlwaysYes(t *testing.T) {
	// Only the non-interactive branch is safe to exercise here: the
	// interactive branch reads from os.Stdin and calls os.Exit(1) on a read
	// error, neither of which is safe/meaningful in a test process.
	old := viper.GetBool("non-interactive")
	viper.Set("non-interactive", true)
	t.Cleanup(func() { viper.Set("non-interactive", old) })

	if !PromptYesNo("Continue? ") {
		t.Error("PromptYesNo() = false, want true in non-interactive mode")
	}
}
