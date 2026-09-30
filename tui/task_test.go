package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type resultMsg string

// run runs a job to its end as a program would, giving each message it has for the worker back to it, and returns what
// it said along the way and what it ended in.
func runJob(t *testing.T, w *worker, cmd tea.Cmd) (progress []string, result tea.Msg) {
	t.Helper()
	for range 100 {
		msg := cmd()
		res, next, mine := w.receive(msg)
		if !mine {
			t.Fatalf("the worker didn't take %#v, which is for its job", msg)
		}
		if next == nil {
			return progress, res
		}
		progress = append(progress, w.text)
		cmd = next
	}
	t.Fatal("the job never ended")
	return nil, nil
}

func TestWorkerDeliversWhatAJobSaysAndWhatItEndsIn(t *testing.T) {
	var w worker
	cmd := w.start("Starting…", func(progress func(string)) tea.Msg {
		progress("half way")
		return resultMsg("done")
	})
	if !w.running() || w.text != "Starting…" {
		t.Fatalf("the worker is running %v saying %q, want it to be busy saying what it was told", w.running(), w.text)
	}

	progress, result := runJob(t, &w, cmd)
	if result != resultMsg("done") {
		t.Errorf("the job ended in %#v, want what it returned", result)
	}
	if w.running() || w.text != "" {
		t.Errorf("the worker is running %v saying %q after the job was done, want it idle", w.running(), w.text)
	}
	// Progress that nobody was listening for yet may be dropped, but what is heard is in order and is the job's own
	for _, p := range progress {
		if p != "half way" {
			t.Errorf("the worker heard %q, want only what the job said", p)
		}
	}
}

func TestWorkerLeavesMessagesThatAreForAnotherJob(t *testing.T) {
	var a, b worker
	cmdA := a.start("a", func(func(string)) tea.Msg { return resultMsg("a") })
	cmdB := b.start("b", func(func(string)) tea.Msg { return resultMsg("b") })

	msgA, msgB := cmdA(), cmdB()
	if _, _, mine := a.receive(msgB); mine {
		t.Error("a took the result of b's job")
	}
	if _, _, mine := b.receive(msgA); mine {
		t.Error("b took the result of a's job")
	}
	if !a.running() || !b.running() {
		t.Error("a job was ended by a message that wasn't its own")
	}
	if res, _, mine := a.receive(msgA); !mine || res != resultMsg("a") {
		t.Errorf("a didn't take its own result: %#v %v", res, mine)
	}
	if res, _, mine := b.receive(msgB); !mine || res != resultMsg("b") {
		t.Errorf("b didn't take its own result: %#v %v", res, mine)
	}
}

func TestWorkerDoesNotTakeAnUnrelatedMessage(t *testing.T) {
	var w worker
	w.start("x", func(func(string)) tea.Msg { return nil })
	if _, _, mine := w.receive(resultMsg("?")); mine {
		t.Error("the worker took a message that isn't a job's")
	}
}

func TestJobsNeverShareANumber(t *testing.T) {
	seen := map[int64]bool{}
	for range 50 {
		var w worker
		w.start("x", func(func(string)) tea.Msg { return nil })
		if seen[w.job] {
			t.Fatalf("job number %d was given twice", w.job)
		}
		seen[w.job] = true
	}
}

func TestScrollerKeepsTheCursorInView(t *testing.T) {
	var s scroller
	s.move(7, 20, 5)
	if s.cursor != 7 || s.offset != 3 {
		t.Errorf("after moving 7 down in 5 lines of 20 the cursor is %d and the offset %d, want 7 and 3", s.cursor, s.offset)
	}
	s.move(100, 20, 5)
	if s.cursor != 19 || s.offset != 15 {
		t.Errorf("at the end the cursor is %d and the offset %d, want 19 and 15", s.cursor, s.offset)
	}
	s.move(-100, 20, 5)
	if s.cursor != 0 || s.offset != 0 {
		t.Errorf("at the start the cursor is %d and the offset %d, want both 0", s.cursor, s.offset)
	}
	if from, to := s.visible(20, 5); from != 0 || to != 5 {
		t.Errorf("the lines shown are %d to %d, want 0 to 5", from, to)
	}
}

func TestScrollerCopesWithAListThatShrinks(t *testing.T) {
	s := scroller{cursor: 18, offset: 15}
	s.clamp(3, 5)
	if s.cursor != 2 || s.offset != 0 {
		t.Errorf("after the list shrank to 3 the cursor is %d and the offset %d, want 2 and 0", s.cursor, s.offset)
	}
	s.clamp(0, 5)
	if s.cursor != 0 || s.offset != 0 {
		t.Errorf("for an empty list the cursor is %d and the offset %d, want both 0", s.cursor, s.offset)
	}
	if got := s.position(0); got != "" {
		t.Errorf("the position in an empty list is %q, want none", got)
	}
}

// Work on the pack is one piece at a time, whichever screens it is for: core keeps its settings in one place that nothing
// guards, and notices are collected in one place too
func TestJobsOnThePackRunOneAtATime(t *testing.T) {
	var a, b worker
	started := make(chan string, 2)
	release := make(chan struct{})
	blocked := func(name string) func(func(string)) tea.Msg {
		return func(func(string)) tea.Msg {
			started <- name
			if name == "first" {
				<-release
			}
			return resultMsg(name)
		}
	}
	// Whichever gets the pack first is the one that waits to be let go
	cmdA := a.start("a", func(p func(string)) tea.Msg { return blocked(firstCome(started))(p) })
	cmdB := b.start("b", func(p func(string)) tea.Msg { return blocked(firstCome(started))(p) })

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("neither job started")
	}
	select {
	case name := <-started:
		t.Fatalf("job %s started while the other had the pack", name)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the second job never started once the first was done")
	}
	_, _ = cmdA(), cmdB()
}

// firstCome says whether the job that is asking is the first to, by what is in started: nothing yet for the first.
func firstCome(started chan string) string {
	if len(started) == 0 {
		return "first"
	}
	return "second"
}

func TestWhatIsReadWhenAScreenIsShownWaitsForWorkOnThePack(t *testing.T) {
	var w worker
	release := make(chan struct{})
	running := make(chan struct{})
	job := w.start("x", func(func(string)) tea.Msg {
		close(running)
		<-release
		return nil
	})
	<-running

	done := make(chan struct{})
	go func() {
		exclusive(func() tea.Msg { return nil })()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("what is read when a screen is shown ran while a job had the pack")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("what is read when a screen is shown never ran once the job was done")
	}
	_ = job()
}
