package cmdshared

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/viper"
	"io"
	"os"
	"strings"
)

var (
	stdinReader *bufio.Reader
	// stdinFile is what stdinReader reads from, so that a replaced os.Stdin (as tests do) gets a reader of its own
	stdinFile *os.File
)

// stdin returns the reader that every prompt reads its answer from. It is shared because a reader per prompt would
// read ahead into its own buffer and discard whatever it didn't use as a line, silently dropping piped-in answers to
// later prompts (and failing them with EOF).
func stdin() *bufio.Reader {
	if stdinReader == nil || stdinFile != os.Stdin {
		stdinFile = os.Stdin
		stdinReader = bufio.NewReader(os.Stdin)
	}
	return stdinReader
}

func PromptYesNo(prompt string) bool {
	fmt.Print(ui.Prompt(prompt))
	if viper.GetBool("non-interactive") {
		ui.Info.Println("Y (non-interactive mode)")
		return true
	}
	answer, err := stdin().ReadString('\n')
	// The last answer of piped input needn't end in a newline (printf 'y'); only having no answer at all is a failure
	if err != nil && !(errors.Is(err, io.EOF) && answer != "") {
		ui.Error.Printf("Failed to prompt user: %v\n", err)
		os.Exit(1)
	}

	ansNormal := strings.ToLower(strings.TrimSpace(answer))
	if len(ansNormal) > 0 && ansNormal[0] == 'n' {
		return false
	}
	return true
}
