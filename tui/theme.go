package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// Everything on screen is coloured with the roles of package ui (ui.Success, ui.Warning, ui.Muted and so on), never
// with colours of its own, so that how it looks is decided in one place and PACKWIZ_COLOR, NO_COLOR and the config
// file's color apply to the TUI as they do to every command. Colour only adds to what is drawn: with it off the screen
// says the same, which is why what is selected or marked is shown with a symbol as well as a style.

// statusKind is what a status message says, which decides how it is styled.
type statusKind int

const (
	statusNone statusKind = iota
	statusInfo
	statusSuccess
	statusWarning
	statusError
)

// status is a message shown below a screen's content, about what was last done.
type status struct {
	text string
	kind statusKind
}

func (st status) render() string {
	switch st.kind {
	case statusInfo:
		return ui.Info.Sprint(st.text)
	case statusSuccess:
		return ui.Success.Sprint(st.text)
	case statusWarning:
		return ui.Warning.Sprint(st.text)
	case statusError:
		return ui.Error.Sprint(st.text)
	}
	return ""
}

func infoStatus(text string) status    { return status{text, statusInfo} }
func successStatus(text string) status { return status{text, statusSuccess} }
func warningStatus(text string) status { return status{text, statusWarning} }
func errorStatus(err error) status     { return status{err.Error(), statusError} }

// clip cuts s to width columns, ending it with an ellipsis if anything was cut off. It measures what is shown, so s can
// be styled: what is left of it keeps its style.
func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// padRight adds spaces to s until it is width columns wide, leaving it as it is if it is already.
func padRight(s string, width int) string {
	if w := ansi.StringWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// spread puts left at the start of a line width columns wide and right at its end, if there is room for both with a
// gap between: otherwise it is just left, cut off if that doesn't fit.
func spread(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 2 {
		return clip(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// hints shows key bindings as "key description" pairs, as many as fit in width columns with more still to be shown
// once there is no room for the rest, which the caller ends with.
func hints(bindings []key.Binding, width int) string {
	var b strings.Builder
	used := 0
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		help := binding.Help()
		hint := ui.Bold.Sprint(help.Key) + " " + ui.Muted.Sprint(help.Desc)
		w := ansi.StringWidth(help.Key) + 1 + ansi.StringWidth(help.Desc)
		if used > 0 {
			w += 2
		}
		if used+w > width {
			break
		}
		if used > 0 {
			b.WriteString("  ")
		}
		b.WriteString(hint)
		used += w
	}
	return b.String()
}

// frame draws a rounded border around lines, each of which is cut off or padded to width columns first, so the box is
// width+4 columns wide: the border, a space of padding, and what is inside.
func frame(lines []string, width int) []string {
	width = max(width, 1)
	out := make([]string, 0, len(lines)+2)
	out = append(out, ui.Muted.Sprint("╭"+strings.Repeat("─", width+2)+"╮"))
	side := ui.Muted.Sprint("│")
	for _, line := range lines {
		out = append(out, side+" "+padRight(clip(line, width), width)+" "+side)
	}
	out = append(out, ui.Muted.Sprint("╰"+strings.Repeat("─", width+2)+"╯"))
	return out
}

// centre puts box, which is no wider than width, in the middle of an area width columns wide and height lines high,
// and returns its lines: exactly height of them, if the box is no higher.
func centre(box []string, width, height int) []string {
	top := max((height-len(box))/2, 0)
	out := make([]string, 0, height)
	for range top {
		out = append(out, "")
	}
	for _, line := range box {
		out = append(out, strings.Repeat(" ", max((width-ansi.StringWidth(line))/2, 0))+line)
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out[:height]
}
