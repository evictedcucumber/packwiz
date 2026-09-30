package tui

import (
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

// Work that takes more than a moment (reading the network, writing a pack) runs off the update loop, in a tea.Cmd, and
// comes back as a message, so that the screen keeps being drawn and answering keys while it runs. A worker is what a
// screen keeps of the work it has started: which it is, and what to say of it while it runs.
//
// Every message a worker sends carries the number of the job it is for, so a screen that hears one for a job that isn't
// its own (the app gives every message to every screen, as a screen that isn't shown still has to hear that its work is
// done) leaves it alone.

// backendLock lets only one piece of work on the pack run at a time, whichever screen it is for. The pack is read and written
// through package core, which keeps its settings in one place that nothing guards (viper), and notices are collected in
// one place too (package notice), so work that overlapped could corrupt both. It waits its turn instead: what is slow is the
// network, and a screen that is waiting for its turn says so as it does for any work.
var backendLock sync.Mutex

// exclusive is a command that runs fn with the pack to itself, for what is read when a screen is shown rather than done on
// request.
func exclusive(fn func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		backendLock.Lock()
		defer backendLock.Unlock()
		return fn()
	}
}

// jobs numbers the work that is started, so that no two jobs have the same number, whichever screen starts them.
var jobs atomic.Int64

// jobResult is a job having finished, with the message it ended in.
type jobResult struct {
	job int64
	msg tea.Msg
}

// jobProgress is a job saying how far it has got. next is what to run to hear what it says after that.
type jobProgress struct {
	job  int64
	text string
	next tea.Cmd
}

// worker keeps track of the one job that a screen has running.
type worker struct {
	// job is the number of the job that is running, or 0 if there is none
	job int64
	// text says what it is doing, for the status line
	text string
}

// running is whether a job is in progress.
func (w *worker) running() bool { return w.job != 0 }

// start runs work off the update loop, and returns the command that delivers what it says as it goes and, in the end,
// what it returns. text is what to show while it runs; work can change it by calling progress, which never blocks it.
func (w *worker) start(text string, work func(progress func(string)) tea.Msg) tea.Cmd {
	job := jobs.Add(1)
	w.job, w.text = job, text

	// Progress is only what to say while waiting, so one that nobody is ready to hear yet is dropped: the next one says
	// more, and the result is never dropped
	events := make(chan tea.Msg, 16)
	go func() {
		result := func() tea.Msg {
			backendLock.Lock()
			defer backendLock.Unlock()
			return work(func(text string) {
				select {
				case events <- jobProgress{job: job, text: text, next: listen(events)}:
				default:
				}
			})
		}()
		events <- jobResult{job, result}
	}()
	return listen(events)
}

// listen waits for the next thing a job has to say.
func listen(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

// receive takes the messages that are for the worker's job. It returns the message the job ended in when that is what
// it was given (mine is true for that, and for progress, which has no result); a screen handles the result as it handles
// any other message, and for progress it runs next.
func (w *worker) receive(msg tea.Msg) (result tea.Msg, next tea.Cmd, mine bool) {
	switch msg := msg.(type) {
	case jobProgress:
		if msg.job != w.job {
			return nil, nil, false
		}
		w.text = msg.text
		return nil, msg.next, true
	case jobResult:
		if msg.job != w.job {
			return nil, nil, false
		}
		w.job, w.text = 0, ""
		return msg.msg, nil, true
	}
	return nil, nil, false
}
