package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildTaskLinePreservesMetadata(t *testing.T) {
	dueDate := time.Date(2026, time.March, 11, 0, 0, 0, 0, time.Local)
	doneDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.Local)

	line := buildTaskLine("Ship release", []string{"#work", "#ship"}, PriorityHigh, dueDate, true, false, false, doneDate, time.Time{})

	expected := "- [x] Ship release #work #ship ⏫ 📅 2026-03-11 ✅ 2026-03-10"
	if line != expected {
		t.Fatalf("unexpected task line\nexpected: %s\nactual:   %s", expected, line)
	}
}

func TestBuildTaskLinePreservesCancelledMetadata(t *testing.T) {
	dueDate := time.Date(2026, time.March, 11, 0, 0, 0, 0, time.Local)
	cancelledDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.Local)

	line := buildTaskLine("Drop release", []string{"#work", "#ship"}, PriorityLow, dueDate, false, true, false, time.Time{}, cancelledDate)

	expected := "- [-] Drop release #work #ship 🔽 📅 2026-03-11 ❌ 2026-03-10"
	if line != expected {
		t.Fatalf("unexpected cancelled task line\nexpected: %s\nactual:   %s", expected, line)
	}
}

func TestBuildTaskLinePreservesBlockedStatus(t *testing.T) {
	dueDate := time.Date(2026, time.March, 11, 0, 0, 0, 0, time.Local)

	line := buildTaskLine("Waiting on review", []string{"#work", "#review"}, PriorityMedium, dueDate, false, false, true, time.Time{}, time.Time{})

	expected := "- [b] Waiting on review #work #review 🔼 📅 2026-03-11"
	if line != expected {
		t.Fatalf("unexpected blocked task line\nexpected: %s\nactual:   %s", expected, line)
	}
}

func TestCreateFollowUpTaskCreatesTomorrowNote(t *testing.T) {
	vaultDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Vault.Path = vaultDir
	cfg.Vault.DailyNotesDir = "Daily"
	cfg.Tasks.SectionHeadings = []string{"## Tasks"}
	cfg.Tasks.SectionHeading = ""

	task := Task{
		Description: "Send proposal",
		Tags:        []string{"#work/client"},
		Priority:    PriorityMedium,
	}

	followUpDate, err := CreateFollowUpTask(cfg, task)
	if err != nil {
		t.Fatalf("CreateFollowUpTask returned error: %v", err)
	}

	expectedDate := localToday().AddDate(0, 0, 1)
	if !followUpDate.Equal(expectedDate) {
		t.Fatalf("unexpected follow-up date\nexpected: %s\nactual:   %s", expectedDate.Format("2006-01-02"), followUpDate.Format("2006-01-02"))
	}

	notePath := filepath.Join(vaultDir, "Daily", followUpDate.Format(cfg.Vault.DailyNoteFormat)+".md")
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("failed to read follow-up note: %v", err)
	}

	body := string(content)
	if !strings.Contains(body, "## Tasks") {
		t.Fatalf("expected section heading in note, got:\n%s", body)
	}

	expectedTask := "- [ ] Follow up: Send proposal #work/client 🔼 📅 " + followUpDate.Format("2006-01-02")
	if !strings.Contains(body, expectedTask) {
		t.Fatalf("expected follow-up task in note, got:\n%s", body)
	}
}

func TestParseTaskRecognizesCancelledStatus(t *testing.T) {
	noteDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.Local)
	task, ok := ParseTask("- [-] Archive draft #work 🔽 📅 2026-03-09 ❌ 2026-03-10", "note.md", 12, noteDate)
	if !ok {
		t.Fatal("expected line to be parsed as task")
	}

	if task.Done {
		t.Fatal("cancelled task should not be marked done")
	}
	if !task.Cancelled {
		t.Fatal("expected task to be marked cancelled")
	}
	if task.CancelledDate.Format("2006-01-02") != "2026-03-10" {
		t.Fatalf("unexpected cancelled date: %s", task.CancelledDate.Format("2006-01-02"))
	}
}

func TestParseTaskRecognizesBlockedStatus(t *testing.T) {
	noteDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.Local)
	task, ok := ParseTask("- [b] Waiting on review #work 🔼 📅 2026-03-11", "note.md", 12, noteDate)
	if !ok {
		t.Fatal("expected line to be parsed as task")
	}

	if task.Done {
		t.Fatal("blocked task should not be marked done")
	}
	if task.Cancelled {
		t.Fatal("blocked task should not be marked cancelled")
	}
	if !task.Blocked {
		t.Fatal("expected task to be marked blocked")
	}
}

func TestToggleBlockedRewritesTaskStatus(t *testing.T) {
	vaultDir := t.TempDir()
	filePath := filepath.Join(vaultDir, "task.md")
	rawLine := "- [ ] Waiting on review #work 📅 2026-03-11"
	if err := os.WriteFile(filePath, []byte(rawLine+"\n"), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	task := Task{
		Description: "Waiting on review",
		FilePath:    filePath,
		LineNumber:  1,
		RawLine:     rawLine,
		DueDate:     time.Date(2026, time.March, 11, 0, 0, 0, 0, time.Local),
		Tags:        []string{"#work"},
	}

	if err := ToggleBlocked(&task); err != nil {
		t.Fatalf("ToggleBlocked returned error: %v", err)
	}
	if !task.Blocked {
		t.Fatal("expected task to be blocked after first toggle")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read task file: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "- [b] Waiting on review #work 📅 2026-03-11" {
		t.Fatalf("unexpected blocked task line: %s", got)
	}

	if err := ToggleBlocked(&task); err != nil {
		t.Fatalf("ToggleBlocked second call returned error: %v", err)
	}
	if task.Blocked {
		t.Fatal("expected task to be unblocked after second toggle")
	}

	content, err = os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read task file after unblock: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != rawLine {
		t.Fatalf("unexpected unblocked task line: %s", got)
	}
}

func benchmarkConfigWithSyntheticVault(b *testing.B) (Config, string) {
	b.Helper()

	vaultDir := b.TempDir()
	cfg := DefaultConfig()
	cfg.Vault.Path = vaultDir
	cfg.Vault.DailyNotesDir = "Daily"
	cfg.Tasks.SectionHeadings = []string{"## Tasks"}
	cfg.Tasks.SectionHeading = ""
	cfg.Tasks.LogbookDays = 30
	cfg.Tasks.LookaheadDays = 14

	base := localToday().AddDate(0, 0, -cfg.Tasks.LogbookDays)
	dailyDir := filepath.Join(vaultDir, cfg.Vault.DailyNotesDir)
	if err := os.MkdirAll(dailyDir, 0755); err != nil {
		b.Fatalf("mkdir daily dir: %v", err)
	}

	targetPath := ""
	for day := 0; day <= cfg.Tasks.LogbookDays+cfg.Tasks.LookaheadDays; day++ {
		date := base.AddDate(0, 0, day)
		heading := cfg.Tasks.EffectiveSectionHeadings()[0]
		lines := []string{
			"---",
			"created: " + date.Format("2006-01-02"),
			"---",
			"",
			heading,
			"",
		}
		for task := 0; task < 20; task++ {
			lines = append(lines, fmt.Sprintf("- [ ] Task %02d-%02d #bench ⏫ 📅 %s", day, task, date.Format("2006-01-02")))
		}
		lines = append(lines, "")
		path := dailyNotePath(cfg, date)
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			b.Fatalf("write note: %v", err)
		}
		if day == cfg.Tasks.LogbookDays {
			targetPath = path
		}
	}

	return cfg, targetPath
}

func BenchmarkScanDailyNotes(b *testing.B) {
	cfg, _ := benchmarkConfigWithSyntheticVault(b)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tasks, err := ScanDailyNotes(cfg)
		if err != nil {
			b.Fatalf("scan daily notes: %v", err)
		}
		if len(tasks) == 0 {
			b.Fatal("expected tasks from benchmark vault")
		}
	}
}

func BenchmarkDailyNotesCacheReloadPaths(b *testing.B) {
	cfg, targetPath := benchmarkConfigWithSyntheticVault(b)
	cache, tasks, err := NewDailyNotesCache(cfg)
	if err != nil {
		b.Fatalf("new daily notes cache: %v", err)
	}
	if len(tasks) == 0 {
		b.Fatal("expected tasks from benchmark vault")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tasks, err := cache.ReloadPaths([]string{targetPath})
		if err != nil {
			b.Fatalf("reload changed paths: %v", err)
		}
		if len(tasks) == 0 {
			b.Fatal("expected tasks after targeted reload")
		}
	}
}
