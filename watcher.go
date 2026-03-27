package main

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

type fileWatchMsg struct {
	at    time.Time
	paths []string
	err   error
}

type dailyNotesWatcher struct {
	events chan fileWatchMsg
}

func newDailyNotesWatcher(cfg Config) (*dailyNotesWatcher, error) {
	dir := filepath.Join(cfg.Vault.Path, cfg.Vault.DailyNotesDir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, err
	}

	w := &dailyNotesWatcher{
		events: make(chan fileWatchMsg, 1),
	}

	go func() {
		defer watcher.Close()
		defer close(w.events)

		var debounce *time.Timer
		var debounceC <-chan time.Time
		pendingPaths := make(map[string]struct{})

		resetDebounce := func() {
			if debounce == nil {
				debounce = time.NewTimer(250 * time.Millisecond)
			} else {
				if !debounce.Stop() {
					select {
					case <-debounce.C:
					default:
					}
				}
				debounce.Reset(250 * time.Millisecond)
			}
			debounceC = debounce.C
		}

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !isRelevantDailyNoteEvent(event) {
					continue
				}
				pendingPaths[event.Name] = struct{}{}
				resetDebounce()
			case <-debounceC:
				debounceC = nil
				paths := make([]string, 0, len(pendingPaths))
				for path := range pendingPaths {
					paths = append(paths, path)
				}
				sort.Strings(paths)
				pendingPaths = make(map[string]struct{})
				w.enqueue(fileWatchMsg{at: time.Now(), paths: paths})
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				w.enqueue(fileWatchMsg{at: time.Now(), err: err})
			}
		}
	}()

	return w, nil
}

func isRelevantDailyNoteEvent(event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	name := strings.ToLower(event.Name)
	return strings.HasSuffix(name, ".md")
}

func (w *dailyNotesWatcher) enqueue(msg fileWatchMsg) {
	select {
	case w.events <- msg:
	default:
	}
}

func (w *dailyNotesWatcher) nextCmd() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-w.events
		if !ok {
			return nil
		}
		return msg
	}
}
