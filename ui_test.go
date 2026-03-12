package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestBuildViewsShowsEmptyTodayLogbookGroupBeforeYesterday(t *testing.T) {
	today := localToday()
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	m := Model{
		cfg: DefaultConfig(),
		allTasks: []Task{
			{
				Description:    "Wrapped up yesterday",
				Done:           true,
				DueDate:        yesterday,
				CompletionDate: yesterday,
			},
			{
				Description:   "Cancelled earlier",
				Cancelled:     true,
				DueDate:       twoDaysAgo,
				CancelledDate: twoDaysAgo,
			},
		},
	}

	m.buildViews()

	if len(m.logbookGroups) != 3 {
		t.Fatalf("expected 3 logbook groups, got %d", len(m.logbookGroups))
	}
	if !sameDay(m.logbookGroups[0].Date, today) {
		t.Fatalf("expected first logbook group to be today, got %s", m.logbookGroups[0].Date.Format("2006-01-02"))
	}
	if len(m.logbookGroups[0].Tasks) != 0 {
		t.Fatalf("expected empty today logbook group, got %d tasks", len(m.logbookGroups[0].Tasks))
	}
	if !sameDay(m.logbookGroups[1].Date, yesterday) {
		t.Fatalf("expected second logbook group to be yesterday, got %s", m.logbookGroups[1].Date.Format("2006-01-02"))
	}
	if count := m.viewTaskCount(viewLogbook); count != 0 {
		t.Fatalf("expected logbook sidebar count to show 0 for today, got %d", count)
	}
}

func TestBuildViewsDoesNotDuplicateTodayLogbookGroup(t *testing.T) {
	today := localToday()

	m := Model{
		cfg: DefaultConfig(),
		allTasks: []Task{
			{
				Description:    "Closed today",
				Done:           true,
				DueDate:        today,
				CompletionDate: today,
			},
		},
	}

	m.buildViews()

	if len(m.logbookGroups) != 1 {
		t.Fatalf("expected 1 logbook group, got %d", len(m.logbookGroups))
	}
	if !sameDay(m.logbookGroups[0].Date, today) {
		t.Fatalf("expected today logbook group, got %s", m.logbookGroups[0].Date.Format("2006-01-02"))
	}
	if len(m.logbookGroups[0].Tasks) != 1 {
		t.Fatalf("expected 1 task in today's logbook group, got %d", len(m.logbookGroups[0].Tasks))
	}
}

func TestWatchEventReloadsEvenDuringInternalWriteGracePeriod(t *testing.T) {
	cfg := testConfigWithTempVault(t)
	today := localToday()
	notePath := writeDailyNote(t, cfg, today, []string{
		"- [ ] First task 📅 " + today.Format("2006-01-02"),
	})

	tasks, err := ScanDailyNotes(cfg)
	if err != nil {
		t.Fatalf("scan daily notes: %v", err)
	}

	m := Model{
		cfg:      cfg,
		allTasks: tasks,
		selected: make(map[int]bool),
	}
	m.buildViews()
	m.markInternalWrite("Task created")

	writeDailyNote(t, cfg, today, []string{
		"- [ ] First task 📅 " + today.Format("2006-01-02"),
		"- [ ] Second task 📅 " + today.Format("2006-01-02"),
	})

	updated, _ := m.Update(fileWatchMsg{at: time.Now()})
	got := updated.(Model)

	if len(got.allTasks) != 2 {
		t.Fatalf("expected reload to pick up external change, got %d tasks", len(got.allTasks))
	}
	if got.statusMsg != "Task created" {
		t.Fatalf("expected recent internal status to be preserved, got %q", got.statusMsg)
	}
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("expected daily note to exist after reload, got %v", err)
	}
}

func TestRenderTodayViewPrioritySeparatorsDefaultOn(t *testing.T) {
	today := localToday()
	m := Model{
		cfg: DefaultConfig(),
		allTasks: []Task{
			{Description: "Urgent", Priority: PriorityHighest, DueDate: today},
			{Description: "Important", Priority: PriorityHigh, DueDate: today},
			{Description: "Other", Priority: PriorityMedium, DueDate: today},
			{Description: "Low", Priority: PriorityLow, DueDate: today},
		},
		showPrioritySeparators: true,
	}

	m.buildViews()

	plain := ansiRE.ReplaceAllString(m.renderTodayView(80, 20), "")

	if !strings.Contains(plain, "P1 > Urgent Tasks") {
		t.Fatalf("expected urgent separator label")
	}
	if !strings.Contains(plain, "P2 > Important Tasks") {
		t.Fatalf("expected important separator label")
	}
	if !strings.Contains(plain, "P3+ > Other Tasks") {
		t.Fatalf("expected other separator label")
	}

	if !sectionHasPadding(plain, "P2 > Important Tasks") {
		t.Fatalf("expected spacing around important task section")
	}
	if !sectionHasPadding(plain, "P3+ > Other Tasks") {
		t.Fatalf("expected spacing around other task section")
	}

	m.showPrioritySeparators = false
	disabled := ansiRE.ReplaceAllString(m.renderTodayView(80, 20), "")
	if strings.Contains(disabled, "P1 >") || strings.Contains(disabled, "P2 >") || strings.Contains(disabled, "P3+ >") {
		t.Fatalf("expected priority separator labels to be hidden when disabled")
	}
}

func sectionHasPadding(text, sectionLabel string) bool {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(line, sectionLabel) {
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) == "" {
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				return true
			}
		}
	}
	return false
}

func testConfigWithTempVault(t *testing.T) Config {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Vault.Path = t.TempDir()
	cfg.Vault.DailyNotesDir = "daily"
	cfg.Tasks.SectionHeading = "## Open Space"
	cfg.Tasks.ExcludeTags = nil

	dailyDir := filepath.Join(cfg.Vault.Path, cfg.Vault.DailyNotesDir)
	if err := os.MkdirAll(dailyDir, 0o755); err != nil {
		t.Fatalf("mkdir daily notes dir: %v", err)
	}

	return cfg
}

func writeDailyNote(t *testing.T, cfg Config, day time.Time, taskLines []string) string {
	t.Helper()

	notePath := filepath.Join(
		cfg.Vault.Path,
		cfg.Vault.DailyNotesDir,
		day.Format(cfg.Vault.DailyNoteFormat)+".md",
	)

	lines := []string{cfg.Tasks.SectionHeading, ""}
	lines = append(lines, taskLines...)
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write daily note: %v", err)
	}

	return notePath
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
