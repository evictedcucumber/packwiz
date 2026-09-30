package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// page is what the screens have in common: how much room they have, the line they say what happened on, the job they have
// running and the box that is open over them. A screen has one, and lays itself out with render: a line that sums up what
// is shown, the lines of what is shown, and the status line.
type page struct {
	worker

	width, height int

	status  status
	overlay overlay
}

func (p *page) setSize(width, height int) {
	p.width, p.height = width, height
	if p.overlay != nil {
		p.overlay.setSize(width, p.bodyHeight())
	}
}

// bodyHeight is how many lines a screen has between its summary and its status: what is left of the room.
func (p *page) bodyHeight() int {
	return max(p.height-2, 1)
}

// open shows a box over the body of the screen.
func (p *page) open(o overlay) {
	o.setSize(p.width, p.bodyHeight())
	p.overlay = o
}

// modal is whether a box is open that takes the keys.
func (p *page) modal() bool { return p.overlay != nil }

// working is whether a job is running, which the app waits for before it quits.
func (p *page) working() bool { return p.running() }

// routeOverlay gives a message to the box that is open, and closes it if it is done. It says whether there was a box,
// and if it is done how it ended: a screen then does what the box was for.
func (p *page) routeOverlay(msg tea.Msg) (box overlay, result overlayResult, routed bool) {
	if p.overlay == nil {
		return nil, overlayOpen, false
	}
	box, result = p.overlay.update(msg)
	p.overlay = box
	if result != overlayOpen {
		p.overlay = nil
	}
	return box, result, true
}

// statusLine is what the line below the body says: what is being waited for, or else what was last done, or else hint,
// which a screen has for when there is nothing else to say.
func (p *page) statusLine(hint string) string {
	switch {
	case p.running():
		return ui.Muted.Sprint(p.text)
	case p.status.text != "":
		return p.status.render()
	}
	return hint
}

// render lays the screen out in the room it has: exactly that many lines, none wider. body is cut off or padded to the
// room between the summary and the status, and is replaced by the box that is open, if there is one.
func (p *page) render(summary string, body []string, hint string) string {
	if p.width <= 0 {
		return ""
	}
	h := p.bodyHeight()
	if p.overlay != nil {
		body = centre(p.overlay.view(), p.width, h)
	}
	lines := make([]string, 0, p.height)
	lines = append(lines, clip(summary, p.width))
	for i := range h {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		lines = append(lines, clip(line, p.width))
	}
	lines = append(lines, clip(p.statusLine(hint), p.width))
	return strings.Join(lines, "\n")
}

// messageBody is what a body says when there is nothing to list, as a few lines of muted text.
func messageBody(lines ...string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = ui.Muted.Sprint(line)
	}
	return out
}
