package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseSnoozeInput(t *testing.T) {
	now := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)

	tests := []struct {
		input string
		want  time.Time
	}{
		{input: "3h", want: now.Add(3 * time.Hour)},
		{input: "45m", want: now.Add(45 * time.Minute)},
		{input: "in 2 hours", want: now.Add(2 * time.Hour)},
		{input: "2d", want: now.AddDate(0, 0, 2)},
		{input: "5pm", want: time.Date(2026, time.March, 27, 17, 0, 0, 0, time.Local)},
		{input: "2pm", want: time.Date(2026, time.March, 28, 14, 0, 0, 0, time.Local)},
		{input: "12am", want: time.Date(2026, time.March, 28, 0, 0, 0, 0, time.Local)},
		{input: "14:30", want: time.Date(2026, time.March, 28, 14, 30, 0, 0, time.Local)},
		{input: "at 16:45", want: time.Date(2026, time.March, 27, 16, 45, 0, 0, time.Local)},
		{input: "in the afternoon", want: time.Date(2026, time.March, 28, 14, 0, 0, 0, time.Local)},
		{input: "this evening", want: time.Date(2026, time.March, 27, 18, 0, 0, 0, time.Local)},
		{input: "à tarde", want: time.Date(2026, time.March, 28, 14, 0, 0, 0, time.Local)},
		{input: "tomorrow", want: time.Date(2026, time.March, 28, 9, 0, 0, 0, time.Local)},
		{input: "amanhã", want: time.Date(2026, time.March, 28, 9, 0, 0, 0, time.Local)},
		{input: "mon", want: time.Date(2026, time.March, 30, 9, 0, 0, 0, time.Local)},
	}

	for _, tt := range tests {
		got, err := parseSnoozeInput(tt.input, now)
		if err != nil {
			t.Errorf("parseSnoozeInput(%q) error: %v", tt.input, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("parseSnoozeInput(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseSnoozeInputRejects(t *testing.T) {
	now := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)
	for _, input := range []string{"", "soon", "25:00", "13pm", "0h", "today", "yesterday"} {
		if got, err := parseSnoozeInput(input, now); err == nil {
			t.Errorf("parseSnoozeInput(%q) = %v, want error", input, got)
		}
	}
}

func TestFormatSnoozeUntil(t *testing.T) {
	now := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)
	tests := []struct {
		until time.Time
		want  string
	}{
		{until: time.Date(2026, time.March, 27, 18, 0, 0, 0, time.Local), want: "18:00"},
		{until: time.Date(2026, time.March, 28, 9, 0, 0, 0, time.Local), want: "tomorrow 09:00"},
		{until: time.Date(2026, time.March, 30, 9, 0, 0, 0, time.Local), want: "Mon 09:00"},
		{until: time.Date(2026, time.April, 10, 9, 0, 0, 0, time.Local), want: "Apr 10 09:00"},
	}
	for _, tt := range tests {
		if got := formatSnoozeUntil(tt.until, now); got != tt.want {
			t.Errorf("formatSnoozeUntil(%v) = %q, want %q", tt.until, got, tt.want)
		}
	}
}

func TestTaskSnoozeKeyIgnoresLineAndDate(t *testing.T) {
	a := Task{Description: "Write  report", FilePath: "/vault/Daily/2026-03-27.md", LineNumber: 3, DueDate: time.Now()}
	b := Task{Description: "Write report", FilePath: "/vault/Daily/2026-03-27.md", LineNumber: 9}
	if taskSnoozeKey(a) != taskSnoozeKey(b) {
		t.Errorf("keys differ: %q vs %q", taskSnoozeKey(a), taskSnoozeKey(b))
	}
	c := Task{Description: "Write report", FilePath: "/vault/Daily/2026-03-28.md"}
	if taskSnoozeKey(a) == taskSnoozeKey(c) {
		t.Errorf("tasks in different notes share key %q", taskSnoozeKey(a))
	}
}

func TestSnoozeStorePersistsAndPrunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "snoozes.json")
	now := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)

	store, err := NewSnoozeStore(path)
	if err != nil {
		t.Fatalf("NewSnoozeStore: %v", err)
	}
	if _, ok := store.NextExpiration(); ok {
		t.Fatal("empty store reports an expiration")
	}

	if err := store.Set("a", now.Add(time.Hour), "1h"); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := store.Set("b", now.Add(3*time.Hour), "3h"); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if err := store.Set("gone", now.Add(2*time.Hour), "2h"); err != nil {
		t.Fatalf("Set gone: %v", err)
	}

	reloaded, err := NewSnoozeStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.IsActive("a", now) || !reloaded.IsActive("b", now) {
		t.Fatal("snoozes not persisted")
	}
	if next, ok := reloaded.NextExpiration(); !ok || !next.Equal(now.Add(time.Hour)) {
		t.Fatalf("NextExpiration = %v, %v", next, ok)
	}

	// "gone" is no longer an open task; "a" has expired by the later time.
	later := now.Add(90 * time.Minute)
	active := map[string]struct{}{"a": {}, "b": {}}
	if err := reloaded.Prune(active, later); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if reloaded.IsActive("a", later) || reloaded.IsActive("gone", later) {
		t.Fatal("expired or orphaned snooze survived prune")
	}
	if !reloaded.IsActive("b", later) {
		t.Fatal("live snooze was pruned")
	}

	final, err := NewSnoozeStore(path)
	if err != nil {
		t.Fatalf("final reload: %v", err)
	}
	if _, ok := final.entries["gone"]; ok {
		t.Fatal("prune not saved to disk")
	}
	if next, ok := final.NextExpiration(); !ok || !next.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("NextExpiration after prune = %v, %v", next, ok)
	}
}
