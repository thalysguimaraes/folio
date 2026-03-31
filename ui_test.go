package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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

func TestRenderTodayViewOverdueTasksStayInPrioritySections(t *testing.T) {
	today := localToday()
	yesterday := today.AddDate(0, 0, -1)

	m := Model{
		cfg: DefaultConfig(),
		allTasks: []Task{
			{Description: "Overdue urgent", Priority: PriorityHighest, DueDate: yesterday},
			{Description: "Today urgent", Priority: PriorityHighest, DueDate: today},
			{Description: "Today important", Priority: PriorityHigh, DueDate: today},
			{Description: "Low", Priority: PriorityLow, DueDate: today},
		},
		showPrioritySeparators: true,
	}

	m.buildViews()

	plain := ansiRE.ReplaceAllString(m.renderTodayView(100, 40), "")

	if strings.Count(plain, "P1 > Urgent Tasks") != 1 {
		t.Fatalf("expected one urgent priority header, got %d", strings.Count(plain, "P1 > Urgent Tasks"))
	}
	if strings.Count(plain, "P2 > Important Tasks") != 1 {
		t.Fatalf("expected one important priority header, got %d", strings.Count(plain, "P2 > Important Tasks"))
	}
	if strings.Count(plain, "P3+ > Other Tasks") != 1 {
		t.Fatalf("expected one other priority header, got %d", strings.Count(plain, "P3+ > Other Tasks"))
	}

	overduePos := strings.Index(plain, "Overdue urgent")
	todayPos := strings.Index(plain, "Today urgent")
	if overduePos == -1 || todayPos == -1 {
		t.Fatalf("expected both overdue and today tasks in output; got:\n%s", plain)
	}
	if overduePos > todayPos {
		t.Fatalf("expected overdue task to be shown before same-priority today task")
	}

	if strings.Contains(plain, "  ── Overdue") {
		t.Fatalf("did not expect extra overdue section header in today view")
	}
}

func TestRenderTodayViewClipsToViewportHeight(t *testing.T) {
	today := localToday()
	var tasks []Task
	for i := 0; i < 25; i++ {
		tasks = append(tasks, Task{
			Description: fmt.Sprintf("Task %02d", i),
			DueDate:     today,
			Priority:    PriorityMedium,
		})
	}

	m := Model{
		cfg:                    DefaultConfig(),
		allTasks:               tasks,
		focus:                  focusContent,
		showPrioritySeparators: true,
	}
	m.buildViews()
	m.contentCursor = len(m.todayTasks) - 1

	plain := ansiRE.ReplaceAllString(m.renderTodayView(80, 8), "")
	lines := strings.Split(plain, "\n")
	if len(lines) > 8 {
		t.Fatalf("expected view to be clipped to 8 rows, got %d", len(lines))
	}
	if !strings.Contains(plain, "Task 24") {
		t.Fatalf("expected selected task to be visible after scroll: %s", plain)
	}
}

func TestRenderTodayViewShowsAllClearEmptyState(t *testing.T) {
	m := Model{
		cfg:                    DefaultConfig(),
		showPrioritySeparators: true,
	}

	m.buildViews()

	rows, _ := m.renderTodayRows(80)
	plain := ansiRE.ReplaceAllString(strings.Join(rows, "\n"), "")

	if !strings.Contains(plain, "All clear") {
		t.Fatalf("expected all clear title in empty state, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Day complete") {
		t.Fatalf("expected day complete kicker in empty state, got:\n%s", plain)
	}
	if !strings.Contains(plain, "^v^") {
		t.Fatalf("expected ascii illustration in empty state, got:\n%s", plain)
	}
}

func TestRenderTodayViewEmptyStateClipsToViewportHeight(t *testing.T) {
	m := Model{
		cfg:                    DefaultConfig(),
		showPrioritySeparators: true,
	}

	m.buildViews()

	plain := ansiRE.ReplaceAllString(m.renderTodayView(80, 8), "")
	lines := strings.Split(plain, "\n")
	if len(lines) > 8 {
		t.Fatalf("expected empty state to be clipped to 8 rows, got %d", len(lines))
	}
}

func TestRenderTodayViewOverdueRowStyling(t *testing.T) {
	today := localToday()
	yesterday := today.AddDate(0, 0, -1)
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(termenv.Ascii)
		lipgloss.SetHasDarkBackground(false)
	})

	m := Model{
		cfg: DefaultConfig(),
		allTasks: []Task{
			{Description: "Overdue urgent", Priority: PriorityHighest, DueDate: yesterday},
			{Description: "Today urgent", Priority: PriorityHighest, DueDate: today},
		},
		showPrioritySeparators: false,
	}

	m.buildViews()
	rows, _ := m.renderTodayRows(120)

	overdueMarker := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.Overdue)).
		Render("Overdue urgent")
	todayMarker := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.Overdue)).
		Render("Today urgent")

	overdueRow := ""
	todayRow := ""
	for _, row := range rows {
		if strings.Contains(row, "Overdue urgent") {
			overdueRow = row
		}
		if strings.Contains(row, "Today urgent") {
			todayRow = row
		}
	}

	if overdueRow == "" {
		t.Fatalf("expected overdue task row in rendered output")
	}
	if todayRow == "" {
		t.Fatalf("expected today task row in rendered output")
	}

	if !strings.Contains(overdueRow, overdueMarker) {
		t.Fatalf("expected overdue row to include overdue color styling")
	}
	if strings.Contains(todayRow, todayMarker) {
		t.Fatalf("did not expect overdue styling on non-overdue task")
	}
}

func TestRenderTodayViewBlockedRowStyling(t *testing.T) {
	today := localToday()

	m := Model{
		cfg: DefaultConfig(),
		allTasks: []Task{
			{Description: "Waiting on vendor", Blocked: true, DueDate: today, Priority: PriorityMedium},
			{Description: "Ready to ship", DueDate: today, Priority: PriorityMedium},
		},
		showPrioritySeparators: false,
	}

	m.buildViews()
	rows, _ := m.renderTodayRows(120)

	blockedRow := ""
	normalRow := ""
	for _, row := range rows {
		if strings.Contains(row, "Waiting on vendor") {
			blockedRow = ansiRE.ReplaceAllString(row, "")
		}
		if strings.Contains(row, "Ready to ship") {
			normalRow = ansiRE.ReplaceAllString(row, "")
		}
	}

	if blockedRow == "" {
		t.Fatalf("expected blocked task row in rendered output")
	}
	if !strings.Contains(blockedRow, "◆") {
		t.Fatalf("expected blocked task row to use blocked bullet, got %q", blockedRow)
	}
	if !strings.Contains(blockedRow, "blocked") {
		t.Fatalf("expected blocked task row to include blocked label, got %q", blockedRow)
	}
	if strings.Contains(normalRow, "blocked") {
		t.Fatalf("did not expect normal task row to include blocked label, got %q", normalRow)
	}
}

func TestRenderTodayViewOverdueUsesLocalDateNotTimezone(t *testing.T) {
	today := localToday()
	dueDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	task := Task{
		Description: "UTC due today",
		DueDate:     dueDate,
		Priority:    PriorityMedium,
	}

	if isTaskOverdue(task, today) {
		t.Fatalf("expected UTC task dated today to be non-overdue in local-date comparison")
	}
}

func TestHandleNormalModeNewTaskInheritsSelectedPriority(t *testing.T) {
	today := localToday()
	m := NewModel(DefaultConfig(), nil, []Task{{Description: "Priority task", Priority: PriorityMedium, DueDate: today}})
	m.focus = focusContent

	m.buildViews()

	updated, _ := m.handleNormalMode(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := updated.(Model)

	if got.mode != modeNewTask {
		t.Fatalf("expected new task mode, got %d", got.mode)
	}
	if got.newTaskDefaultPriority != PriorityMedium {
		t.Fatalf("expected inherited priority %d, got %d", PriorityMedium, got.newTaskDefaultPriority)
	}
}

func TestHandleNormalModeDuplicatePrefillsTaskContent(t *testing.T) {
	today := localToday()
	m := NewModel(DefaultConfig(), nil, []Task{{
		Description: "Ship release",
		Tags:        []string{"#work", "#release"},
		Priority:    PriorityHigh,
		DueDate:     today,
	}})
	m.focus = focusContent

	m.buildViews()

	updated, _ := m.handleNormalMode(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	got := updated.(Model)

	if got.mode != modeNewTask {
		t.Fatalf("expected duplicate shortcut to open new task mode, got %d", got.mode)
	}
	if got.newTaskTitle != "Duplicate Task" {
		t.Fatalf("expected duplicate task title, got %q", got.newTaskTitle)
	}
	if got.input.Value() != "Ship release #work #release" {
		t.Fatalf("unexpected duplicate prefill: %q", got.input.Value())
	}
}

func TestRenderTaskRowExpandsSelectedTaskInsteadOfTruncating(t *testing.T) {
	task := Task{
		Description: "alpha beta gamma delta epsilon zeta eta theta iota kappa omega",
		Tags:        []string{"#project/personal/obsidian-tasks-tui", "#work"},
		Priority:    PriorityMedium,
		DueDate:     localToday(),
	}
	m := Model{cfg: DefaultConfig()}

	collapsed := m.renderTaskRow(task, false, 32, false, false)
	plainCollapsed := ansiRE.ReplaceAllString(collapsed, "")
	plainExpanded := ansiRE.ReplaceAllString(m.renderTaskRow(task, true, 32, false, false), "")

	if !strings.Contains(plainCollapsed, "...") {
		t.Fatalf("expected collapsed row to be truncated, got %q", plainCollapsed)
	}
	if strings.Contains(collapsed, "\n") {
		t.Fatalf("did not expect collapsed row to wrap, got %q", plainCollapsed)
	}
	if ansi.StringWidth(collapsed) > 32 {
		t.Fatalf("expected collapsed row width <= 32, got %d: %q", ansi.StringWidth(collapsed), plainCollapsed)
	}
	if strings.Contains(plainExpanded, "...") {
		t.Fatalf("did not expect selected row to be truncated, got %q", plainExpanded)
	}
	if !strings.Contains(plainExpanded, "omega") {
		t.Fatalf("expected selected row to include the end of the description, got %q", plainExpanded)
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
	cfg.Tasks.SectionHeadings = []string{"## Open Space"}
	cfg.Tasks.SectionHeading = ""
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

	lines := []string{cfg.Tasks.EffectiveSectionHeadings()[0], ""}
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
