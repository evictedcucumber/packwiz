package ui

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// setMode sets the mode for the duration of the test.
func setMode(t *testing.T, m Mode) {
	t.Helper()
	old := SetMode(m)
	t.Cleanup(func() { SetMode(old) })
}

func TestSetModeReturnsThePreviousMode(t *testing.T) {
	setMode(t, Never)
	if got := SetMode(Always); got != Never {
		t.Errorf("SetMode() = %v, want the previous mode, Never", got)
	}
	if got := SetMode(Auto); got != Always {
		t.Errorf("SetMode() = %v, want the previous mode, Always", got)
	}
}

// captureStdout replaces os.Stdout with a pipe for the duration of fn, and returns what was written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-out
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", Auto, false},
		{"auto", Auto, false},
		{"always", Always, false},
		{"never", Never, false},
		{" Always ", Always, false},
		{"NEVER", Never, false},
		{"sometimes", Auto, true},
		{"true", Auto, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNeverLeavesTextAlone(t *testing.T) {
	setMode(t, Never)
	for name, got := range map[string]string{
		"Sprint":  Error.Sprint("a ", 1, " b"),
		"Sprintf": Success.Sprintf("%s: %d", "a", 1),
		"nested":  Warning.Sprintf("x %s y", Bold.Sprint("z")),
		"lines":   Info.Sprint("one\ntwo\n"),
		"prompt":  Prompt("Version [1.0.0]: "),
	} {
		if Strip(got) != got {
			t.Errorf("%s: got escape sequences with colour off: %q", name, got)
		}
	}
	if got := Error.Sprintf("%s: %d", "a", 1); got != "a: 1" {
		t.Errorf("Sprintf with colour off = %q, want %q", got, "a: 1")
	}
}

func TestStyleSequences(t *testing.T) {
	setMode(t, Always)
	tests := []struct {
		name  string
		style Style
		want  string
	}{
		{"bold", Bold, "\x1b[1mx\x1b[0m"},
		{"muted", Muted, "\x1b[2mx\x1b[0m"},
		{"error", Error, "\x1b[31mx\x1b[0m"},
		{"success", Success, "\x1b[32mx\x1b[0m"},
		{"warning", Warning, "\x1b[33mx\x1b[0m"},
		{"info", Info, "\x1b[36mx\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.style.Sprint("x"); got != tt.want {
				t.Errorf("Sprint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmptyTextIsNotStyled(t *testing.T) {
	setMode(t, Always)
	if got := Error.Sprint(""); got != "" {
		t.Errorf("Sprint(\"\") = %q, want nothing, as there's nothing to show", got)
	}
}

func TestPaintsEachLineOnItsOwn(t *testing.T) {
	setMode(t, Always)
	got := Error.Sprint("one\n\ntwo\n")
	want := "\x1b[31mone\x1b[0m\n\n\x1b[31mtwo\x1b[0m\n"
	if got != want {
		t.Errorf("Sprint() = %q, want %q: a style must end before each line break, and a blank line has nothing to style", got, want)
	}
}

func TestNestedStyleKeepsOuterStyle(t *testing.T) {
	setMode(t, Always)
	got := Success.Sprintf("added %s now", Bold.Sprint("Sodium"))
	// Bold ends with a reset, which would end the green too: it has to be started again for " now"
	want := "\x1b[32madded \x1b[1mSodium\x1b[0m\x1b[32m now\x1b[0m"
	if got != want {
		t.Errorf("Sprintf() = %q, want %q", got, want)
	}
}

func TestNestedStyleAtEndOfTextAddsNothing(t *testing.T) {
	setMode(t, Always)
	got := Success.Sprintf("added %s", Bold.Sprint("Sodium"))
	want := "\x1b[32madded \x1b[1mSodium\x1b[0m"
	if got != want {
		t.Errorf("Sprintf() = %q, want %q: nothing follows the nested style, so nothing needs styling again", got, want)
	}
}

func TestNestedStyleInsideMultipleLines(t *testing.T) {
	setMode(t, Always)
	got := Warning.Sprintf("a %s\nb %s c", Bold.Sprint("1"), Bold.Sprint("2"))
	// The first line ends in the nested style, and the second has text after it
	want := "\x1b[33ma \x1b[1m1\x1b[0m\n\x1b[33mb \x1b[1m2\x1b[0m\x1b[33m c\x1b[0m"
	if got != want {
		t.Errorf("Sprintf() = %q, want %q", got, want)
	}
}

// Colour is only ever added to text, so taking it out has to give back the text as it is without it.
func TestStripGivesBackTheText(t *testing.T) {
	texts := []string{
		"plain",
		"",
		"two\nlines\n",
		"\nleading and trailing\n\n",
		"100% [Y/n] (a, b): ",
		"has a literal \\x1b in it",
		"tabs\tand\r\nwindows endings\r\n",
	}
	for _, text := range texts {
		for _, m := range []Mode{Never, Always} {
			t.Run(strings.NewReplacer("\n", "⏎", "\t", "→").Replace(text), func(t *testing.T) {
				setMode(t, m)
				styled := Success.Sprintf("%s|%s", Bold.Sprint(text), Muted.Sprint(Error.Sprint(text)))
				want := text + "|" + text
				if got := Strip(styled); got != want {
					t.Errorf("Strip(%q) = %q, want %q", styled, got, want)
				}
				if got := Strip(Prompt(text)); got != text {
					t.Errorf("Strip(Prompt(%q)) = %q, want the prompt as it was", text, got)
				}
			})
		}
	}
}

func TestTransition(t *testing.T) {
	setMode(t, Always)
	got := Transition("a-1.jar", "a-2.jar")
	want := "\x1b[31ma-1.jar\x1b[0m -> \x1b[32ma-2.jar\x1b[0m"
	if got != want {
		t.Errorf("Transition() = %q, want %q", got, want)
	}

	setMode(t, Never)
	if got, want := Transition("a-1.jar", "a-2.jar"), "a-1.jar -> a-2.jar"; got != want {
		t.Errorf("Transition() with colour off = %q, want %q", got, want)
	}
}

func TestPrintfAndPrintlnWriteToStdout(t *testing.T) {
	setMode(t, Always)
	got := captureStdout(t, func() {
		Success.Printf("%d done\n", 3)
		Error.Println("failed:", errors.New("boom"))
		Warning.Println("no", "space", 1, 2)
	})
	want := "\x1b[32m3 done\x1b[0m\n" +
		"\x1b[31mfailed: boom\x1b[0m\n" +
		// Println puts a space between operands, unlike Sprint, which only does between two that aren't strings
		"\x1b[33mno space 1 2\x1b[0m\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintWithColourOffIsWhatFmtWrites(t *testing.T) {
	setMode(t, Never)
	got := captureStdout(t, func() {
		Success.Printf("%d done\n", 3)
		Error.Println("failed:", errors.New("boom"))
	})
	if want := "3 done\nfailed: boom\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAutoDoesNotColourAPipe(t *testing.T) {
	setMode(t, Auto)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	got := captureStdout(t, func() {
		if Enabled() {
			t.Error("Enabled() = true for a pipe, want false")
		}
		Error.Println("x")
	})
	if got != "x\n" {
		t.Errorf("got %q, want the text without colour", got)
	}
}

func TestAlwaysColoursAPipe(t *testing.T) {
	setMode(t, Always)
	t.Setenv("NO_COLOR", "1")
	got := captureStdout(t, func() { Error.Println("x") })
	if want := "\x1b[31mx\x1b[0m\n"; got != want {
		t.Errorf("got %q, want %q: asking for colour outranks NO_COLOR", got, want)
	}
}

func TestPrompt(t *testing.T) {
	setMode(t, Always)
	tests := []struct {
		name, prompt, want string
	}{
		{
			"yes/no",
			"Would you like to update it? [Y/n]: ",
			"\x1b[1mWould you like to update it? \x1b[0m\x1b[36m[Y/n]\x1b[0m: ",
		},
		{
			"default",
			"Modpack name [My Pack]: ",
			"\x1b[1mModpack name \x1b[0m\x1b[36m[My Pack]\x1b[0m: ",
		},
		{
			"default and choices",
			"Mod loader [neoforge] (none, fabric): ",
			"\x1b[1mMod loader \x1b[0m\x1b[36m[neoforge]\x1b[0m\x1b[1m \x1b[0m\x1b[2m(none, fabric)\x1b[0m: ",
		},
		{
			"no default",
			"Author: ",
			"\x1b[1mAuthor\x1b[0m: ",
		},
		{
			"no colon",
			"Would you like to fix this automatically? [Y/n] ",
			"\x1b[1mWould you like to fix this automatically? \x1b[0m\x1b[36m[Y/n]\x1b[0m ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Prompt(tt.prompt)
			if got != tt.want {
				t.Errorf("Prompt(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
			if Strip(got) != tt.prompt {
				t.Errorf("Strip(Prompt(%q)) = %q, want the prompt as it was", tt.prompt, Strip(got))
			}
		})
	}
}

func TestForWritesToItsStreamAndNotToStdout(t *testing.T) {
	setMode(t, Always)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	elsewhere := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		elsewhere <- string(b)
	}()

	stdout := captureStdout(t, func() {
		Error.For(w).Printf("failed: %s\n", "boom")
		Warning.For(w).Println("careful")
	})
	_ = w.Close()

	if stdout != "" {
		t.Errorf("stdout = %q, want nothing: the style was for another stream", stdout)
	}
	if got, want := <-elsewhere, "\x1b[31mfailed: boom\x1b[0m\n\x1b[33mcareful\x1b[0m\n"; got != want {
		t.Errorf("the stream got %q, want %q", got, want)
	}
}

func TestForDoesNotChangeTheStyleItIsCalledOn(t *testing.T) {
	setMode(t, Always)
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	_ = Error.For(w)

	out := captureStdout(t, func() { Error.Println("x") })
	if want := "\x1b[31mx\x1b[0m\n"; out != want {
		t.Errorf("Error still printed %q to stdout, want %q: For makes a new style", out, want)
	}
}

// Real-world names: what Git for Windows' mintty and Cygwin make for a program that is started from them.
func TestIsCygwinPipeName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{`\msys-1888ae32e00d56aa-pty0-to-master`, true},
		{`\msys-1888ae32e00d56aa-pty0-from-master`, true},
		{`\cygwin-e022582115c10879-pty4-to-master`, true},
		{`\cygwin-e022582115c10879-pty12-from-master`, true},
		// As some versions of Windows name them, with the device
		{`\Device\NamedPipe\msys-1888ae32e00d56aa-pty0-to-master`, true},
		{`\Device\NamedPipe\cygwin-e022582115c10879-pty1-from-master`, true},
		// A pipe that Cygwin makes, but that isn't a terminal
		{`\msys-1888ae32e00d56aa-1234-pipe-0x1`, false},
		{`\cygwin-e022582115c10879-lpc-0x1`, false},
		// Not the pipes of Cygwin at all
		{`\pipe-to-master`, false},
		{`\mypipe-1888ae32e00d56aa-pty0-to-master`, false},
		{`\msys--pty0-to-master`, false},
		{`\msys-1888ae32e00d56aa-tty0-to-master`, false},
		{`\msys-1888ae32e00d56aa-pty0-through-master`, false},
		{`\msys-1888ae32e00d56aa-pty0-to-slave`, false},
		{`\msys-1888ae32e00d56aa-pty0-to`, false},
		{``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCygwinPipeName(tt.name); got != tt.want {
				t.Errorf("isCygwinPipeName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
