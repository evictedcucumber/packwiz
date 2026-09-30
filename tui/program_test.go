package tui

import (
	"bytes"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// syncBuffer is what a program draws to, which the test reads from another goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// waitFor waits until condition is true, failing the test if it isn't within a few seconds.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Runs the app as a program, with the bytes a terminal sends for keys, so that the parts that the other tests call
// directly are run as they are for real: reading keys, running commands and their results coming back, and quitting.
// It can't see the screen (what is drawn is cell by cell, with the cursor moved about, not text to search in), so it goes
// by what the program did to the pack.
func TestProgramRelatesAFileFromKeyPresses(t *testing.T) {
	setUpPack(t)
	backend := packBackend{}
	data, err := backend.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}

	// The pack is read again once a change has been made, all of it, so when that is read it is safe to look at it
	reloaded := make(chan struct{}, 1)
	watch := func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(loadedMsg); ok {
			select {
			case reloaded <- struct{}{}:
			default:
			}
		}
		return msg
	}

	in, send := io.Pipe()
	var out syncBuffer
	program := tea.NewProgram(newApp(data.pack, newConfigScreen(backend, data)),
		tea.WithInput(in), tea.WithOutput(&out), tea.WithWindowSize(100, 30), tea.WithoutSignalHandler(), tea.WithFilter(watch))

	type result struct {
		model tea.Model
		err   error
	}
	done := make(chan result, 1)
	go func() {
		model, err := program.Run()
		done <- result{model, err}
	}()
	t.Cleanup(func() { _ = send.Close() })

	waitFor(t, "the first frame", func() bool { return out.Len() > 0 })

	key := func(keys string) {
		t.Helper()
		if _, err := send.Write([]byte(keys)); err != nil {
			t.Fatalf("failed to send keys: %v", err)
		}
	}
	// Down to config/orphan.json, relate it, pick Beta Mod (down past the pack and Alpha Mod, and space), and confirm with enter
	key("jjjjjj")
	key("r")
	key("jj") // past the pack and Alpha Mod
	key(" ")
	key("\r")

	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		t.Fatal("the pack wasn't read again after relating the file")
	}
	key("q")

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("the program failed: %v", r.err)
		}
		final, ok := r.model.(*app)
		if !ok {
			t.Fatalf("the program ended with a %T, want the app", r.model)
		}
		if final.screen.modal() {
			t.Error("a box was still open when it quit")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the program didn't quit on q")
	}

	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want the file", got)
	}
	assertIndexIsConsistent(t)
}

func TestProgramQuitsOnCtrlCFromInsideABox(t *testing.T) {
	setUpPack(t)
	backend := packBackend{}
	data, _ := backend.load()

	in, send := io.Pipe()
	var out syncBuffer
	program := tea.NewProgram(newApp(data.pack, newConfigScreen(backend, data)),
		tea.WithInput(in), tea.WithOutput(&out), tea.WithWindowSize(100, 30), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()
	t.Cleanup(func() { _ = send.Close() })

	waitFor(t, "the first frame", func() bool { return out.Len() > 0 })
	// Into the picker, and typing into its filter, where q is text
	if _, err := send.Write([]byte("jjjjjjr/q")); err != nil {
		t.Fatalf("failed to send keys: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("the program ended (with %v) when q was typed into a filter", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := send.Write([]byte{0x03}); err != nil { // ctrl+c
		t.Fatalf("failed to send ctrl+c: %v", err)
	}
	select {
	case err := <-done:
		// Quitting with ctrl+c is what was asked for, so it isn't an error
		if err != nil {
			t.Errorf("the program ended with %v, want a clean quit", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the program didn't quit on ctrl+c")
	}
}
