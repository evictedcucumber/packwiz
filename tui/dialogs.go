package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The boxes that screens ask things in, other than the ones of the config screen, which are its own: a question that is
// answered yes or no, with what it is about listed under it, and a line of text to type.

var (
	keyOK     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "ok"))
	keyCancel = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	keyClose  = key.NewBinding(key.WithKeys("esc", "enter", "q"), key.WithHelp("esc", "close"))
)

// dialogWidth is how wide the text in a box is at most, however wide the terminal is: lines any longer are hard to read.
const dialogWidth = 72

// confirmBox asks whether to go ahead with something, and lists what it is. The list scrolls if it is longer than there
// is room for. A box that only says something (see newInfoBox) is the same, without the question.
type confirmBox struct {
	title string
	lines []string
	// info is whether it asks nothing: it says its lines, and any key that closes a box closes it
	info bool

	scroll        scroller
	width, height int
}

func newConfirmBox(title string, lines ...string) *confirmBox {
	return &confirmBox{title: title, lines: lines}
}

// newInfoBox is a box that lists lines, and is closed when it has been read.
func newInfoBox(title string, lines ...string) *confirmBox {
	return &confirmBox{title: title, lines: lines, info: true}
}

func (c *confirmBox) setSize(width, height int) { c.width, c.height = width, height }

// inner is how wide the text in the box is.
func (c *confirmBox) inner() int { return max(min(c.width-4, dialogWidth), 1) }

// shown is the lines of the box, wrapped to fit it.
func (c *confirmBox) shown() []string {
	var out []string
	for _, line := range c.lines {
		out = append(out, wrap(line, c.inner(), "  ")...)
	}
	return out
}

// room is how many of the lines are shown at once: the box less its border, its title and the gap after that.
func (c *confirmBox) room() int { return max(c.height-4, 1) }

// scrolls is whether there are more lines than there is room for, as the box then says where in them it is.
func (c *confirmBox) scrolls() bool { return len(c.shown()) > c.room() }

func (c *confirmBox) update(msg tea.Msg) (overlay, overlayResult) {
	msgKey, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, overlayOpen
	}
	switch {
	case c.info && key.Matches(msgKey, keyClose):
		return c, overlayCancelled
	case !c.info && key.Matches(msgKey, keyConfirm):
		return c, overlayConfirmed
	case !c.info && key.Matches(msgKey, keyDecline):
		return c, overlayCancelled
	case key.Matches(msgKey, keyMoveUp):
		c.scroll.move(-1, len(c.shown()), c.roomForList())
	case key.Matches(msgKey, keyMoveDown):
		c.scroll.move(1, len(c.shown()), c.roomForList())
	case key.Matches(msgKey, keyPageUp):
		c.scroll.move(-c.roomForList(), len(c.shown()), c.roomForList())
	case key.Matches(msgKey, keyPageDown):
		c.scroll.move(c.roomForList(), len(c.shown()), c.roomForList())
	}
	return c, overlayOpen
}

// roomForList is how many lines are shown, which is one fewer when the box has a line of its own to say where they are.
func (c *confirmBox) roomForList() int {
	if c.scrolls() {
		return max(c.room()-1, 1)
	}
	return c.room()
}

func (c *confirmBox) keys() []key.Binding {
	bindings := []key.Binding{keyConfirm, keyDecline}
	if c.info {
		bindings = []key.Binding{keyClose}
	}
	if c.scrolls() {
		bindings = append(bindings, keyPickerMove)
	}
	return bindings
}

func (c *confirmBox) view() []string {
	lines := []string{ui.Bold.Sprint(c.title), ""}
	shown := c.shown()
	from, to := c.scroll.visible(len(shown), c.roomForList())
	lines = append(lines, shown[from:to]...)
	if c.scrolls() {
		lines = append(lines, ui.Muted.Sprintf("%s  ↑/↓ to see the rest", c.scroll.position(len(shown))))
	}
	return frame(lines, c.inner())
}

// promptBox asks for a line of text.
type promptBox struct {
	title string
	// lines say what it is for, above the text
	lines []string
	input input
	// check says what is wrong with what was typed, or "" if it will do; enter doesn't accept it while there is
	check func(string) string
	// problem is what check said of the text as it is now
	problem string

	width, height int
}

func newPromptBox(title, initial string, check func(string) string, lines ...string) *promptBox {
	p := &promptBox{title: title, lines: lines, check: check}
	p.input.insert(initial)
	p.recheck()
	return p
}

// text is what was typed.
func (p *promptBox) text() string { return p.input.String() }

func (p *promptBox) recheck() {
	p.problem = ""
	if p.check != nil {
		p.problem = p.check(p.input.String())
	}
}

func (p *promptBox) setSize(width, height int) { p.width, p.height = width, height }

func (p *promptBox) update(msg tea.Msg) (overlay, overlayResult) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		p.input.handle(msg)
		p.recheck()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyOK):
			if p.problem == "" {
				return p, overlayConfirmed
			}
		case key.Matches(msg, keyCancel):
			return p, overlayCancelled
		default:
			if p.input.handle(msg) {
				p.recheck()
			}
		}
	}
	return p, overlayOpen
}

func (p *promptBox) keys() []key.Binding { return []key.Binding{keyOK, keyCancel} }

func (p *promptBox) view() []string {
	lines := []string{ui.Bold.Sprint(p.title)}
	lines = append(lines, p.lines...)
	lines = append(lines, "", p.input.String()+ui.Muted.Sprint("█"))
	if p.problem != "" {
		lines = append(lines, ui.Warning.Sprint(p.problem))
	}
	return frame(lines, min(p.width-4, dialogWidth))
}
