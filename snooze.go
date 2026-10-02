package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Snoozes hide an open task from the Today and Upcoming views until a given
// time. They live outside the vault (in a small JSON file next to the config)
// so snoozing never rewrites daily notes.

type snoozeEntry struct {
	Until time.Time `json:"until"`
	Input string    `json:"input,omitempty"`
}

type SnoozeStore struct {
	mu      sync.Mutex
	path    string
	entries map[string]snoozeEntry
}

var (
	snoozeDurationRe = regexp.MustCompile(`^(?:in\s+)?(\d+)\s*(m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)$`)
	snoozeClockRe    = regexp.MustCompile(`^(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$`)
)

// Times used for day-part words such as "in the afternoon".
var snoozeDayParts = map[string]int{
	"morning":   9,
	"manha":     9,
	"noon":      12,
	"midday":    12,
	"afternoon": 14,
	"tarde":     14,
	"evening":   18,
	"tonight":   20,
	"night":     20,
	"noite":     20,
}

// Hour used when the snooze input is a date without a time ("tomorrow", "mon").
const snoozeDefaultHour = 9

func defaultSnoozePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "obsidian-tasks", "snoozes.json"), nil
}

// NewSnoozeStore loads the snooze file at path, or the default location when
// path is empty. A missing file is not an error.
func NewSnoozeStore(path string) (*SnoozeStore, error) {
	if path == "" {
		p, err := defaultSnoozePath()
		if err != nil {
			return nil, err
		}
		path = p
	}

	s := &SnoozeStore{path: path, entries: make(map[string]snoozeEntry)}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.entries); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return s, nil
}

// Set snoozes the task identified by key until the given time.
func (s *SnoozeStore) Set(key string, until time.Time, input string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = snoozeEntry{Until: until, Input: strings.TrimSpace(input)}
	return s.saveLocked()
}

// IsActive reports whether key is snoozed at now.
func (s *SnoozeStore) IsActive(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	return ok && e.Until.After(now)
}

// Prune drops expired snoozes and snoozes for tasks that are no longer open,
// saving the file only when something changed.
func (s *SnoozeStore) Prune(activeKeys map[string]struct{}, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for key, e := range s.entries {
		_, active := activeKeys[key]
		if !active || !e.Until.After(now) {
			delete(s.entries, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

// NextExpiration returns the earliest snooze end time, if any.
func (s *SnoozeStore) NextExpiration() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next time.Time
	for _, e := range s.entries {
		if next.IsZero() || e.Until.Before(next) {
			next = e.Until
		}
	}
	return next, !next.IsZero()
}

func (s *SnoozeStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// taskSnoozeKey identifies a task by its daily note and description, so a
// snooze survives the line moving within the file or the task being
// rescheduled. Editing the description releases the snooze.
func taskSnoozeKey(task Task) string {
	desc := strings.Join(strings.Fields(task.Description), " ")
	return filepath.Clean(task.FilePath) + "::" + desc
}

// parseSnoozeInput turns inputs such as "3h", "45m", "2pm", "14:30",
// "in the afternoon", "tomorrow" or "mon" into an absolute time after now.
func parseSnoozeInput(input string, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(input)
	normalized := normalizeNaturalDateInput(trimmed)
	if normalized == "" {
		return time.Time{}, fmt.Errorf("empty snooze input")
	}

	if m := snoozeDurationRe.FindStringSubmatch(normalized); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("invalid snooze duration: %s", trimmed)
		}
		switch m[2][0] {
		case 'm':
			return now.Add(time.Duration(n) * time.Minute), nil
		case 'h':
			return now.Add(time.Duration(n) * time.Hour), nil
		default:
			return now.AddDate(0, 0, n), nil
		}
	}

	if until, matched, err := parseSnoozeClock(normalized, now); matched {
		return until, err
	}

	part := strings.TrimPrefix(normalized, "in the ")
	part = strings.TrimPrefix(part, "this ")
	part = strings.TrimPrefix(part, "a ")
	if hour, ok := snoozeDayParts[part]; ok {
		return nextOccurrence(now, hour, 0), nil
	}

	if day, err := parseNaturalDate(trimmed, now); err == nil {
		until := time.Date(day.Year(), day.Month(), day.Day(), snoozeDefaultHour, 0, 0, 0, now.Location())
		if !until.After(now) {
			return time.Time{}, fmt.Errorf("snooze time is in the past: %s", trimmed)
		}
		return until, nil
	}

	return time.Time{}, fmt.Errorf("unrecognized snooze: %s", trimmed)
}

// parseSnoozeClock handles clock times. matched reports whether the input
// looked like a clock time, so an invalid one ("25:00") is rejected instead of
// falling through to the date parser.
func parseSnoozeClock(input string, now time.Time) (until time.Time, matched bool, err error) {
	m := snoozeClockRe.FindStringSubmatch(input)
	if m == nil {
		return time.Time{}, false, nil
	}
	// A bare number is ambiguous with a day offset; require a colon or suffix.
	if m[2] == "" && m[3] == "" {
		return time.Time{}, false, nil
	}
	invalid := fmt.Errorf("invalid time: %s", input)
	hour, _ := strconv.Atoi(m[1])
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	switch m[3] {
	case "am":
		if hour < 1 || hour > 12 {
			return time.Time{}, true, invalid
		}
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour < 1 || hour > 12 {
			return time.Time{}, true, invalid
		}
		if hour != 12 {
			hour += 12
		}
	}
	if hour > 23 || minute > 59 {
		return time.Time{}, true, invalid
	}
	return nextOccurrence(now, hour, minute), true, nil
}

// nextOccurrence returns today at hour:minute, or tomorrow if that has passed.
func nextOccurrence(now time.Time, hour, minute int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// formatSnoozeUntil renders a snooze end time relative to now.
func formatSnoozeUntil(until, now time.Time) string {
	today := normalizeDate(now, now.Location())
	day := normalizeDate(until, now.Location())
	clock := until.Format("15:04")
	switch days := int(day.Sub(today).Hours() / 24); {
	case days == 0:
		return clock
	case days == 1:
		return "tomorrow " + clock
	case days > 1 && days < 7:
		return until.Format("Mon") + " " + clock
	default:
		return until.Format("Jan 02") + " " + clock
	}
}
