package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

// Cobra writes help and usage itself, and which stream it does that to depends on why: the help that was asked for
// (--help, or the help command) goes to stdout, and the usage that comes after an error goes to stderr. Whether to
// colour is a matter for each stream on its own, which cobra's templates can't be told, so both are written here:
// the text is what cobra's own template makes (see usageText), and it is coloured for the stream it is going to.

// usageFuncs are the functions that cobra's templates can call. Cobra keeps them to itself.
var usageFuncs = template.FuncMap{
	"trim":                    strings.TrimSpace,
	"trimRightSpace":          trimRightSpace,
	"trimTrailingWhitespaces": trimRightSpace,
	"rpad":                    rpad,
	"gt":                      cobra.Gt,
	"eq":                      cobra.Eq,
}

func trimRightSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

func rpad(s string, padding int) string {
	return fmt.Sprintf("%-*s", padding, s)
}

// usageText is how c is used, as cobra writes it: its usage template (by default, one that says what its usage, aliases,
// examples, subcommands and flags are) filled in for c.
func usageText(c *cobra.Command) (string, error) {
	// Cobra merges the flags that a command gets from its parents into its own before it writes usage, which the line
	// that says how to use it depends on. Asking for them is how that is done without it.
	_ = c.LocalFlags()

	t, err := template.New("usage").Funcs(usageFuncs).Parse(c.UsageTemplate())
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, c); err != nil {
		return "", err
	}
	return b.String(), nil
}

// helpText is the help for c as cobra writes it: what c is (its long description, or its short one if it has no long
// one), then how it is used. It doesn't use a help template that has been set for c.
func helpText(c *cobra.Command) (string, error) {
	var b strings.Builder
	description := c.Long
	if description == "" {
		description = c.Short
	}
	if description = trimRightSpace(description); description != "" {
		b.WriteString(description + "\n\n")
	}
	if c.Runnable() || c.HasSubCommands() {
		usage, err := usageText(c)
		if err != nil {
			return "", err
		}
		b.WriteString(usage)
	}
	return b.String(), nil
}

// destination is the file that what is written to w goes to, if it is one. Otherwise it is fallback: usage is written to
// a buffer when cobra wants it as a string, as it does to print it after an error, and that is printed to stderr.
func destination(w io.Writer, fallback *os.File) *os.File {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return fallback
}

// usageFunc writes how a command is used, coloured if where it goes is a terminal.
func usageFunc(c *cobra.Command) error {
	text, err := usageText(c)
	if err != nil {
		c.PrintErrln(err)
		return err
	}
	w := c.OutOrStderr()
	_, err = io.WriteString(w, styleUsage(text, destination(w, os.Stderr), groupTitles(c)))
	return err
}

// helpFunc writes the help for a command, to stdout, coloured if that is a terminal.
func helpFunc(c *cobra.Command, _ []string) {
	// Help is printed before a command's initializers run, which is where colour is applied
	applyColorAfterFlags()

	text, err := helpText(c)
	if err != nil {
		c.PrintErrln(err)
		return
	}
	w := c.OutOrStdout()
	_, _ = io.WriteString(w, styleUsage(text, destination(w, os.Stdout), groupTitles(c)))
}

// groupTitles are the titles of the groups that the subcommands of c are put in, which are headings of usage too.
func groupTitles(c *cobra.Command) []string {
	var titles []string
	for _, group := range c.Groups() {
		titles = append(titles, group.Title)
	}
	return titles
}

// errPrefix is what cobra begins an error message with, which it writes to stderr.
func errPrefix() string {
	return ui.Error.For(os.Stderr).Sprint("Error:")
}

// usageHeadings are the headings of the parts of usage that cobra writes.
var usageHeadings = map[string]bool{
	"Usage:":                  true,
	"Aliases:":                true,
	"Examples:":               true,
	"Available Commands:":     true,
	"Additional Commands:":    true,
	"Flags:":                  true,
	"Global Flags:":           true,
	"Additional help topics:": true,
}

var (
	commandLine = regexp.MustCompile(`^(  )(\S+)(.*)$`)
	flagLine    = regexp.MustCompile(`^(\s+)((?:-\w, )?--[\w.-]+)(.*)$`)
	// flagDefault is the default of a flag, which is at the end of what is said about it
	flagDefault = regexp.MustCompile(`\(default .*\)$`)
)

// styleUsage colours the parts of usage that make it easy to scan, when f is where it is going and that is a terminal:
// the headings, the names of commands and of flags, and what is only there to be looked up. Text is only coloured, never
// changed. What comes before the usage of a command, its description, is left as it is. The titles of groups of
// commands are headings too, and commands are listed under them; they are only known to the command (see groupTitles).
func styleUsage(text string, f *os.File, groups []string) string {
	if !ui.EnabledFor(f) {
		return text
	}
	heading, name, muted := ui.Bold.For(f), ui.Info.For(f), ui.Muted.For(f)
	fade := func(s string) string { return muted.Sprint(s) }

	lines := strings.Split(text, "\n")
	usage, section := false, ""
	for i, line := range lines {
		switch {
		case (usageHeadings[line] || slices.Contains(groups, line)) && (usage || line == "Usage:"):
			usage, section = true, line
			lines[i] = heading.Sprint(line)
		case !usage:
		case line == "":
			section = ""
		case section == "Available Commands:" || section == "Additional Commands:" || section == "Additional help topics:" ||
			(section != "" && slices.Contains(groups, section)):
			if m := commandLine.FindStringSubmatch(line); m != nil {
				lines[i] = m[1] + name.Sprint(m[2]) + m[3]
			}
		case section == "Flags:" || section == "Global Flags:":
			if m := flagLine.FindStringSubmatch(line); m != nil {
				lines[i] = m[1] + name.Sprint(m[2]) + flagDefault.ReplaceAllStringFunc(m[3], fade)
			}
		case strings.HasPrefix(line, `Use "`) && strings.HasSuffix(line, " for more information about a command."):
			lines[i] = muted.Sprint(line)
		}
	}
	return strings.Join(lines, "\n")
}
