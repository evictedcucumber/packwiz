package cmdshared

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/evictedcucumber/packwiz/internal/ui"
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

// PromptYesNo asks a yes or no question, where no answer is yes.
func PromptYesNo(prompt string) bool {
	return PromptYesNoDefault(prompt, true)
}

// PromptYesNoDefault asks a yes or no question, where no answer is def, as is any answer that isn't the other one.
func PromptYesNoDefault(prompt string, def bool) bool {
	fmt.Print(ui.Prompt(prompt))
	answer, err := stdin().ReadString('\n')
	// The last answer of piped input needn't end in a newline (printf 'y'); only having no answer at all is a failure
	if err != nil && !(errors.Is(err, io.EOF) && answer != "") {
		ui.Error.Printf("Failed to prompt user: %v\n", err)
		os.Exit(1)
	}

	ansNormal := strings.ToLower(strings.TrimSpace(answer))
	if len(ansNormal) == 0 {
		return def
	}
	// Only the answer that isn't the default has to be given as such
	if def {
		return ansNormal[0] != 'n'
	}
	return ansNormal[0] == 'y'
}

// ReadLine reads a line of what is typed, without its line break, from the reader the prompts read from. At the end of
// what there is to read, it returns io.EOF with whatever was left.
func ReadLine() (string, error) {
	line, err := stdin().ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}
