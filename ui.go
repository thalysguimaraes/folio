package main

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const hPad = 2

const (
	viewToday = iota
	viewUpcoming
	viewLogbook
)

const (
	focusSidebar = iota
	focusContent
)

const (
	modeNormal = iota
	modeNewTask
	modeEditTask
	modeFilter
	modeHelp
	modeConfirmDelete
	modeReschedule
	modePriority
)

type DateGroup struct {
	Date  time.Time
	Label string
	Tasks []int
}

type Model struct {
	cfg      Config
	allTasks []Task
	watcher  *dailyNotesWatcher
	cache    *DailyNotesCache

	activeView    int
	focus         int
	sidebarCursor int
	contentCursor int
	scrollOffset  int

	todayTasks      []int
	upcomingGroups  []DateGroup
	logbookGroups   []DateGroup
	logbookDayIndex int

	mode                int
	width               int
	height              int
	input               textinput.Model
	filter              string
	statusMsg           string
	statusTime          time.Time
	err                 error
	preserveStatusUntil time.Time

	selected map[int]bool

	showPrioritySeparators bool
}

func tagColor(tag string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(tag))
	colors := []string{
		"#E06C75", "#98C379", "#E5C07B", "#61AFEF",
		"#C678DD", "#56B6C2", "#D19A66", "#BE5046",
	}
	return lipgloss.Color(colors[h.Sum32()%uint32(len(colors))])
}

func NewModel(cfg Config, cache *DailyNotesCache, tasks []Task) Model {
	ti := textinput.New()
	ti.Placeholder = "Task description #tag p1-p5"
	ti.CharLimit = 256
	ti.Width = 50

	m := Model{
		cfg:                    cfg,
		cache:                  cache,
		allTasks:               tasks,
		mode:                   modeNormal,
		input:                  ti,
		activeView:             viewToday,
		focus:                  focusSidebar,
		selected:               make(map[int]bool),
		showPrioritySeparators: true,
	}
	watcher, err := newDailyNotesWatcher(cfg)
	if err != nil {
		m.statusMsg = "Auto-sync disabled: " + err.Error()
		m.statusTime = time.Now()
	} else {
		m.watcher = watcher
	}
	m.buildViews()
	return m
}

func (m *Model) matchesFilter(t Task) bool {
	if m.filter == "" {
		return true
	}
	low := strings.ToLower(m.filter)
	if strings.Contains(strings.ToLower(t.Description), low) {
		return true
	}
	for _, tag := range t.Tags {
		if strings.Contains(strings.ToLower(tag), low) {
			return true
		}
	}
	return false
}

func localToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func dueDateAtLocation(dueDate time.Time, location *time.Location) time.Time {
	return time.Date(dueDate.Year(), dueDate.Month(), dueDate.Day(), 0, 0, 0, 0, location)
}

func isTaskOverdue(task Task, today time.Time) bool {
	taskDate := dueDateAtLocation(task.DueDate, today.Location())
	return taskDate.Before(today)
}

func (m *Model) buildViews() {
	today := localToday()
	selectedLogbookDate := today
	if len(m.logbookGroups) > 0 && m.logbookDayIndex < len(m.logbookGroups) {
		g := m.logbookGroups[m.logbookDayIndex].Date
		selectedLogbookDate = time.Date(g.Year(), g.Month(), g.Day(), 0, 0, 0, 0, today.Location())
	}

	m.todayTasks = nil
	m.upcomingGroups = nil
	m.logbookGroups = nil

	var todayUndone []int
	var overdueUndone []int
	upcomingMap := make(map[string][]int)
	upcomingDates := make(map[string]time.Time)
	logbookMap := make(map[string][]int)
	logbookDates := make(map[string]time.Time)

	for i, t := range m.allTasks {
		if !m.matchesFilter(t) {
			continue
		}
		due := time.Date(t.DueDate.Year(), t.DueDate.Month(), t.DueDate.Day(), 0, 0, 0, 0, today.Location())

		if t.IsCompleted() {
			closedDate := t.ClosedDate()
			compDate := time.Time{}
			if !closedDate.IsZero() {
				compDate = time.Date(closedDate.Year(), closedDate.Month(), closedDate.Day(), 0, 0, 0, 0, today.Location())
			}
			if compDate.IsZero() {
				compDate = due
			}
			key := compDate.Format("2006-01-02")
			logbookMap[key] = append(logbookMap[key], i)
			logbookDates[key] = compDate
			continue
		}

		if due.After(today) {
			key := due.Format("2006-01-02")
			upcomingMap[key] = append(upcomingMap[key], i)
			upcomingDates[key] = due
		} else if due.Equal(today) {
			todayUndone = append(todayUndone, i)
		} else {
			overdueUndone = append(overdueUndone, i)
		}
	}

	sortByTodayPriority := func(indices []int) {
		sort.SliceStable(indices, func(i, j int) bool {
			ti := m.allTasks[indices[i]]
			tj := m.allTasks[indices[j]]
			tiDue := dueDateAtLocation(ti.DueDate, today.Location())
			tjDue := dueDateAtLocation(tj.DueDate, today.Location())

			if ti.Priority != tj.Priority {
				return ti.Priority < tj.Priority
			}

			iOverdue := isTaskOverdue(ti, today)
			jOverdue := isTaskOverdue(tj, today)
			if iOverdue != jOverdue {
				return iOverdue
			}

			if !tiDue.Equal(tjDue) {
				return tiDue.Before(tjDue)
			}
			return ti.Description < tj.Description
		})
	}
	sortByPriority := func(indices []int) {
		sort.SliceStable(indices, func(i, j int) bool {
			ti := m.allTasks[indices[i]]
			tj := m.allTasks[indices[j]]
			if !ti.DueDate.Equal(tj.DueDate) {
				return ti.DueDate.Before(tj.DueDate)
			}
			if ti.Priority != tj.Priority {
				return ti.Priority < tj.Priority
			}
			return ti.Description < tj.Description
		})
	}
	m.todayTasks = append(m.todayTasks, todayUndone...)
	m.todayTasks = append(m.todayTasks, overdueUndone...)
	sortByTodayPriority(m.todayTasks)

	var upcomingSorted []string
	for key := range upcomingMap {
		upcomingSorted = append(upcomingSorted, key)
	}
	sort.Strings(upcomingSorted)
	for _, key := range upcomingSorted {
		tasks := upcomingMap[key]
		sortByPriority(tasks)
		m.upcomingGroups = append(m.upcomingGroups, DateGroup{
			Date:  upcomingDates[key],
			Label: upcomingDates[key].Format("Mon, Jan 02"),
			Tasks: tasks,
		})
	}

	var logbookSorted []string
	for key := range logbookMap {
		logbookSorted = append(logbookSorted, key)
	}
	sort.Slice(logbookSorted, func(i, j int) bool {
		return logbookSorted[i] > logbookSorted[j]
	})
	for _, key := range logbookSorted {
		m.logbookGroups = append(m.logbookGroups, DateGroup{
			Date:  logbookDates[key],
			Label: logbookDates[key].Format("Jan 02"),
			Tasks: logbookMap[key],
		})
	}

	if len(m.logbookGroups) > 0 {
		todayKey := today.Format("2006-01-02")
		if _, ok := logbookMap[todayKey]; !ok {
			m.logbookGroups = append([]DateGroup{{
				Date:  today,
				Label: today.Format("Jan 02"),
				Tasks: nil,
			}}, m.logbookGroups...)
		}
	}

	m.logbookDayIndex = 0
	for i, g := range m.logbookGroups {
		gDate := time.Date(g.Date.Year(), g.Date.Month(), g.Date.Day(), 0, 0, 0, 0, today.Location())
		if gDate.Equal(selectedLogbookDate) {
			m.logbookDayIndex = i
			break
		}
	}

	if m.logbookDayIndex >= len(m.logbookGroups) {
		m.logbookDayIndex = max(0, len(m.logbookGroups)-1)
	}
	m.clampCursor()
}

func (m *Model) currentViewTasks() []int {
	switch m.activeView {
	case viewToday:
		return m.todayTasks
	case viewUpcoming:
		var flat []int
		for _, g := range m.upcomingGroups {
			flat = append(flat, g.Tasks...)
		}
		return flat
	case viewLogbook:
		if len(m.logbookGroups) > 0 && m.logbookDayIndex < len(m.logbookGroups) {
			return m.logbookGroups[m.logbookDayIndex].Tasks
		}
		return nil
	}
	return nil
}

func (m *Model) clampCursor() {
	tasks := m.currentViewTasks()
	if m.contentCursor >= len(tasks) {
		m.contentCursor = max(0, len(tasks)-1)
	}
	if m.sidebarCursor > 2 {
		m.sidebarCursor = 2
	}
}

func (m *Model) viewTaskCount(view int) int {
	switch view {
	case viewToday:
		return len(m.todayTasks)
	case viewUpcoming:
		count := 0
		for _, g := range m.upcomingGroups {
			count += len(g.Tasks)
		}
		return count
	case viewLogbook:
		if len(m.logbookGroups) > 0 && m.logbookDayIndex < len(m.logbookGroups) {
			return len(m.logbookGroups[m.logbookDayIndex].Tasks)
		}
		return 0
	}
	return 0
}

func (m Model) selectedTask() *Task {
	tasks := m.currentViewTasks()
	if len(tasks) == 0 || m.contentCursor >= len(tasks) {
		return nil
	}
	return &m.allTasks[tasks[m.contentCursor]]
}

func (m Model) reload() Model {
	return m.reloadPaths(nil)
}

func (m Model) reloadPaths(paths []string) Model {
	var (
		tasks []Task
		err   error
	)
	if m.cache != nil {
		tasks, err = m.cache.ReloadPaths(paths)
	} else {
		tasks, err = ScanDailyNotes(m.cfg)
	}
	if err != nil {
		m.err = err
		m.statusMsg = "Reload error: " + err.Error()
		return m
	}
	m.err = nil
	m.allTasks = tasks
	m.selected = make(map[int]bool)
	m.buildViews()
	return m
}

func (m Model) reloadTask(task *Task) Model {
	if task == nil {
		return m.reload()
	}
	return m.reloadPaths([]string{task.FilePath})
}

func (m Model) reloadTaskIndices(indices map[int]bool) Model {
	if len(indices) == 0 {
		return m.reload()
	}

	paths := make([]string, 0, len(indices))
	seen := make(map[string]struct{}, len(indices))
	for idx := range indices {
		path := m.allTasks[idx].FilePath
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return m.reloadPaths(paths)
}

func (m Model) nextWatchCmd() tea.Cmd {
	if m.watcher == nil {
		return nil
	}
	return m.watcher.nextCmd()
}

func (m *Model) markInternalWrite(status string) {
	m.statusMsg = status
	m.statusTime = time.Now()
	m.preserveStatusUntil = m.statusTime.Add(1200 * time.Millisecond)
}

func (m Model) Init() tea.Cmd {
	return m.nextWatchCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = m.width - 2*hPad - 16
		return m, nil

	case fileWatchMsg:
		cmd := m.nextWatchCmd()
		if msg.err != nil {
			m.statusMsg = "Auto-sync error: " + msg.err.Error()
			m.statusTime = time.Now()
			return m, cmd
		}
		m = m.reloadPaths(msg.paths)
		if m.err == nil {
			if !m.preserveStatusUntil.IsZero() && msg.at.Before(m.preserveStatusUntil) {
				return m, cmd
			}
			m.statusMsg = "Synced from files"
			m.statusTime = time.Now()
		}
		return m, cmd

	case tea.KeyMsg:
		if m.mode == modeNewTask || m.mode == modeEditTask || m.mode == modeFilter || m.mode == modeReschedule {
			return m.handleInputMode(msg)
		}
		if m.mode == modeHelp {
			m.mode = modeNormal
			return m, nil
		}
		if m.mode == modeConfirmDelete {
			return m.handleConfirmDelete(msg)
		}
		if m.mode == modePriority {
			return m.handlePriority(msg)
		}
		return m.handleNormalMode(msg)
	}

	return m, nil
}

func (m Model) handleConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		task := m.selectedTask()
		if task != nil {
			if err := CancelTask(task); err != nil {
				m.err = err
				m.statusMsg = "Error: " + err.Error()
			} else {
				m.markInternalWrite("Task cancelled")
				m = m.reloadTask(task)
			}
		}
		m.mode = modeNormal
	case "n", "N", "esc", "q":
		m.mode = modeNormal
	}
	return m, nil
}

func (m Model) handlePriority(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	priorityMap := map[string]int{
		"1": PriorityHighest,
		"2": PriorityHigh,
		"3": PriorityMedium,
		"4": PriorityLow,
		"5": PriorityLowest,
		"0": PriorityNone,
	}
	labels := map[int]string{
		PriorityHighest: "Highest",
		PriorityHigh:    "High",
		PriorityMedium:  "Medium",
		PriorityLow:     "Low",
		PriorityLowest:  "Lowest",
		PriorityNone:    "None",
	}

	key := msg.String()
	if p, ok := priorityMap[key]; ok {
		task := m.selectedTask()
		if task != nil {
			if err := SetPriority(task, p); err != nil {
				m.statusMsg = "Error: " + err.Error()
			} else {
				m.markInternalWrite("Priority → " + labels[p])
				m = m.reloadTask(task)
			}
		}
		m.mode = modeNormal
	} else if key == "esc" || key == "q" {
		m.mode = modeNormal
	}
	return m, nil
}

func (m Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil

	case "enter":
		value := m.input.Value()
		m.input.SetValue("")
		m.input.Blur()

		switch m.mode {
		case modeNewTask:
			m.mode = modeNormal
			if value == "" {
				return m, nil
			}
			defaultDueDate := localToday()
			if m.activeView == viewUpcoming && len(m.upcomingGroups) > 0 {
				groupIdx := m.groupIndexForCursor()
				if groupIdx >= 0 && groupIdx < len(m.upcomingGroups) {
					defaultDueDate = m.upcomingGroups[groupIdx].Date
				}
			}
			draft, err := parseTaskDraftInput(value, defaultDueDate, localToday())
			if err != nil {
				m.err = err
				m.statusMsg = "Error: " + err.Error()
				return m, nil
			}
			if err := CreateTask(m.cfg, draft.Description, draft.DueDate, draft.Priority); err != nil {
				m.err = err
				m.statusMsg = "Error: " + err.Error()
			} else {
				m.markInternalWrite("Task created")
				m = m.reload()
			}

		case modeEditTask:
			m.mode = modeNormal
			if value == "" {
				return m, nil
			}
			task := m.selectedTask()
			if task == nil {
				return m, nil
			}
			newLine := buildTaskLine(value, task.Tags, task.Priority, task.DueDate, task.Done, task.Cancelled, task.Blocked, task.CompletionDate, task.CancelledDate)
			if err := UpdateTaskLine(task, newLine); err != nil {
				m.err = err
				m.statusMsg = "Error: " + err.Error()
			} else {
				m.markInternalWrite("Task updated")
				m = m.reloadTask(task)
			}

		case modeFilter:
			m.mode = modeNormal
			m.filter = value
			m.buildViews()

		case modeReschedule:
			m.mode = modeNormal
			if value == "" {
				return m, nil
			}
			newDate, err := parseRelativeDate(value)
			if err != nil {
				m.statusMsg = "Invalid date: " + value
				m.statusTime = time.Now()
				return m, nil
			}
			if len(m.selected) > 0 {
				count := 0
				selected := m.selected
				for idx := range m.selected {
					if err := RescheduleTask(&m.allTasks[idx], newDate); err != nil {
						m.statusMsg = "Error: " + err.Error()
						m.statusTime = time.Now()
						break
					}
					count++
				}
				m.selected = make(map[int]bool)
				m.markInternalWrite(fmt.Sprintf("%d tasks → %s", count, newDate.Format("Jan 02")))
				m = m.reloadTaskIndices(selected)
			} else {
				task := m.selectedTask()
				if task == nil {
					return m, nil
				}
				if err := RescheduleTask(task, newDate); err != nil {
					m.err = err
					m.statusMsg = "Error: " + err.Error()
				} else {
					m.markInternalWrite("Rescheduled → " + newDate.Format("Jan 02"))
					m = m.reloadTask(task)
				}
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) groupIndexForCursor() int {
	cursor := m.contentCursor
	var groups []DateGroup
	switch m.activeView {
	case viewUpcoming:
		groups = m.upcomingGroups
	case viewLogbook:
		groups = m.logbookGroups
	default:
		return -1
	}
	offset := 0
	for i, g := range groups {
		if cursor < offset+len(g.Tasks) {
			return i
		}
		offset += len(g.Tasks)
	}
	return len(groups) - 1
}

func (m Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "1":
		m.activeView = viewToday
		m.sidebarCursor = 0
		m.contentCursor = 0
		m.scrollOffset = 0
		m.selected = make(map[int]bool)

	case "2":
		m.activeView = viewUpcoming
		m.sidebarCursor = 1
		m.contentCursor = 0
		m.scrollOffset = 0
		m.selected = make(map[int]bool)

	case "3":
		m.activeView = viewLogbook
		m.sidebarCursor = 2
		m.contentCursor = 0
		m.scrollOffset = 0
		m.selected = make(map[int]bool)

	case "tab":
		if m.focus == focusSidebar {
			m.focus = focusContent
		} else {
			m.focus = focusSidebar
		}

	case "h":
		m.focus = focusSidebar

	case "l":
		if m.focus == focusSidebar {
			m.focus = focusContent
		}

	case "j", "down":
		if m.focus == focusSidebar {
			if m.sidebarCursor < 2 {
				m.sidebarCursor++
				m.activeView = m.sidebarCursor
				m.contentCursor = 0
				m.scrollOffset = 0
			}
		} else {
			tasks := m.currentViewTasks()
			if len(tasks) > 0 && m.contentCursor < len(tasks)-1 {
				m.contentCursor++
			}
		}

	case "k", "up":
		if m.focus == focusSidebar {
			if m.sidebarCursor > 0 {
				m.sidebarCursor--
				m.activeView = m.sidebarCursor
				m.contentCursor = 0
				m.scrollOffset = 0
			}
		} else {
			if m.contentCursor > 0 {
				m.contentCursor--
			}
		}

	case "enter":
		if m.focus == focusSidebar {
			m.focus = focusContent
		} else {
			task := m.selectedTask()
			if task != nil {
				wasCancelled := task.Cancelled
				if err := ToggleDone(task); err != nil {
					m.err = err
					m.statusMsg = "Error: " + err.Error()
				} else {
					if wasCancelled {
						m.markInternalWrite("Reopened")
					} else if task.Done {
						m.markInternalWrite("Marked done")
					} else {
						m.markInternalWrite("Marked undone")
					}
					m = m.reloadTask(task)
				}
			}
		}

	case " ":
		if m.focus == focusContent && m.activeView != viewLogbook {
			tasks := m.currentViewTasks()
			if len(tasks) > 0 && m.contentCursor < len(tasks) {
				idx := tasks[m.contentCursor]
				if m.selected[idx] {
					delete(m.selected, idx)
				} else {
					m.selected[idx] = true
				}
				if m.contentCursor < len(tasks)-1 {
					m.contentCursor++
				}
			}
		}

	case "v":
		if m.focus == focusContent && m.activeView != viewLogbook {
			tasks := m.currentViewTasks()
			if len(m.selected) > 0 {
				m.selected = make(map[int]bool)
			} else {
				for _, idx := range tasks {
					m.selected[idx] = true
				}
			}
		}

	case "d":
		if m.focus == focusContent {
			if len(m.selected) > 0 && m.activeView != viewLogbook {
				count := 0
				for idx := range m.selected {
					if err := ToggleDone(&m.allTasks[idx]); err != nil {
						m.statusMsg = "Error: " + err.Error()
						m.statusTime = time.Now()
						break
					}
					count++
				}
				selected := m.selected
				m.selected = make(map[int]bool)
				m.markInternalWrite(fmt.Sprintf("%d tasks marked done", count))
				m = m.reloadTaskIndices(selected)
			} else {
				task := m.selectedTask()
				if task != nil {
					wasCancelled := task.Cancelled
					if err := ToggleDone(task); err != nil {
						m.err = err
						m.statusMsg = "Error: " + err.Error()
					} else {
						if wasCancelled {
							m.markInternalWrite("Reopened")
						} else if task.Done {
							m.markInternalWrite("Marked done")
						} else {
							m.markInternalWrite("Marked undone")
						}
						m = m.reloadTask(task)
					}
				}
			}
		}

	case "b":
		if m.focus == focusContent && m.activeView != viewLogbook {
			if len(m.selected) > 0 {
				targetBlocked := false
				for idx := range m.selected {
					if !m.allTasks[idx].Blocked {
						targetBlocked = true
						break
					}
				}

				count := 0
				selected := m.selected
				for idx := range m.selected {
					if m.allTasks[idx].Blocked == targetBlocked {
						continue
					}
					if err := ToggleBlocked(&m.allTasks[idx]); err != nil {
						m.statusMsg = "Error: " + err.Error()
						m.statusTime = time.Now()
						break
					}
					count++
				}
				m.selected = make(map[int]bool)
				if targetBlocked {
					m.markInternalWrite(fmt.Sprintf("%d tasks blocked", count))
				} else {
					m.markInternalWrite(fmt.Sprintf("%d tasks unblocked", count))
				}
				m = m.reloadTaskIndices(selected)
			} else {
				task := m.selectedTask()
				if task != nil {
					targetBlocked := !task.Blocked
					if err := ToggleBlocked(task); err != nil {
						m.err = err
						m.statusMsg = "Error: " + err.Error()
					} else {
						if targetBlocked {
							m.markInternalWrite("Marked blocked")
						} else {
							m.markInternalWrite("Marked unblocked")
						}
						m = m.reloadTask(task)
					}
				}
			}
		}

	case "f", "F":
		if m.focus == focusContent && m.activeView != viewLogbook {
			task := m.selectedTask()
			if task != nil {
				followUpDate, err := CreateFollowUpTask(m.cfg, *task)
				if err != nil {
					m.err = err
					m.statusMsg = "Error: " + err.Error()
				} else {
					m.markInternalWrite("Follow-up → " + followUpDate.Format("Jan 02"))
					m = m.reloadPaths([]string{dailyNotePath(m.cfg, followUpDate)})
				}
			}
		}

	case "D":
		if m.focus == focusContent && m.activeView != viewLogbook {
			task := m.selectedTask()
			if task != nil {
				m.mode = modeConfirmDelete
			}
		}

	case "n":
		if m.activeView != viewLogbook {
			m.mode = modeNewTask
			m.input.Placeholder = "Task description #tag p1-p5 📅 amanhã"
			m.input.SetValue("")
			m.input.Focus()
			return m, m.input.Cursor.BlinkCmd()
		}

	case "e":
		if m.focus == focusContent && m.activeView != viewLogbook {
			task := m.selectedTask()
			if task != nil {
				m.mode = modeEditTask
				m.input.Placeholder = "Edit description"
				m.input.SetValue(task.Description)
				m.input.Focus()
				return m, m.input.Cursor.BlinkCmd()
			}
		}

	case "p":
		if m.focus == focusContent && m.activeView != viewLogbook {
			task := m.selectedTask()
			if task != nil {
				m.mode = modePriority
			}
		}

	case "s":
		if m.focus == focusContent && m.activeView != viewLogbook {
			if len(m.selected) > 0 {
				m.mode = modeReschedule
				m.input.Placeholder = "Date: amanhã, próxima seg, em 2 semanas, 2026-03-01"
				m.input.SetValue("")
				m.input.Focus()
				return m, m.input.Cursor.BlinkCmd()
			}
			task := m.selectedTask()
			if task != nil {
				m.mode = modeReschedule
				m.input.Placeholder = "Date: amanhã, próxima seg, em 2 semanas, 2026-03-01"
				m.input.SetValue("")
				m.input.Focus()
				return m, m.input.Cursor.BlinkCmd()
			}
		}

	case "/":
		m.mode = modeFilter
		m.input.Placeholder = "Filter tasks..."
		m.input.SetValue(m.filter)
		m.input.Focus()
		return m, m.input.Cursor.BlinkCmd()

	case "esc":
		if len(m.selected) > 0 {
			m.selected = make(map[int]bool)
		} else if m.filter != "" {
			m.filter = ""
			m.buildViews()
		}

	case "left":
		if m.activeView == viewLogbook && len(m.logbookGroups) > 0 {
			if m.logbookDayIndex < len(m.logbookGroups)-1 {
				m.logbookDayIndex++
				m.contentCursor = 0
				m.scrollOffset = 0
			}
		}

	case "right":
		if m.activeView == viewLogbook && m.logbookDayIndex > 0 {
			m.logbookDayIndex--
			m.contentCursor = 0
			m.scrollOffset = 0
		}

	case "?":
		m.mode = modeHelp

	case "t":
		m.showPrioritySeparators = !m.showPrioritySeparators
		state := "off"
		if m.showPrioritySeparators {
			state = "on"
		}
		m.statusMsg = "Priority separators " + state
		m.statusTime = time.Now()

	case "r":
		m = m.reload()
		if m.err == nil {
			m.statusMsg = "Reloaded"
			m.statusTime = time.Now()
		}
	}

	return m, nil
}

var (
	subtleBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
	}
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.mode == modeHelp {
		return m.renderHelp()
	}

	totalWidth := m.width - 2*hPad - 2
	sidebarWidth := 22
	contentWidth := totalWidth - sidebarWidth - 1
	contentHeight := m.height - 4

	sidebar := m.renderSidebar(sidebarWidth, contentHeight)
	content := m.renderContent(contentWidth, contentHeight)

	board := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	footer := m.renderFooter(totalWidth)

	var inputArea string
	if m.mode == modeNewTask || m.mode == modeEditTask || m.mode == modeFilter || m.mode == modeReschedule {
		prefix := " New: "
		if m.mode == modeEditTask {
			prefix = " Edit: "
		} else if m.mode == modeFilter {
			prefix = " Filter: "
		} else if m.mode == modeReschedule {
			prefix = " Reschedule: "
		}
		prefixStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
			Bold(true)
		inputArea = "\n" + prefixStyle.Render(prefix) + m.input.View()
	}

	if m.mode == modeConfirmDelete {
		confirmStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Overdue)).
			Bold(true)
		inputArea = "\n" + confirmStyle.Render(" Cancel this task? [y/n]")
	}

	if m.mode == modePriority {
		prStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
			Bold(true)
		inputArea = "\n" + prStyle.Render(" Priority: 1🔺 2⏫ 3🔼 4🔽 5⏬ 0 none")
	}

	result := board + "\n" + footer + inputArea
	return lipgloss.NewStyle().Padding(0, hPad).Render(result)
}

func (m Model) renderSidebar(width, height int) string {
	isActive := m.focus == focusSidebar
	accent := lipgloss.Color(m.cfg.Theme.Accent)
	borderColor := lipgloss.Color("#3a3a3a")
	if isActive {
		borderColor = accent
	}

	type sidebarItem struct {
		icon  string
		label string
		view  int
	}
	items := []sidebarItem{
		{"☀️", "Today", viewToday},
		{"📅", "Upcoming", viewUpcoming},
		{"📓", "Logbook", viewLogbook},
	}

	var rows []string
	rows = append(rows, "")

	for _, item := range items {
		count := m.viewTaskCount(item.view)
		selected := m.activeView == item.view

		label := fmt.Sprintf(" %s %s", item.icon, item.label)
		if count > 0 {
			label = fmt.Sprintf(" %s %-8s %d", item.icon, item.label, count)
		}

		style := lipgloss.NewStyle().Width(width - 2)

		if selected && isActive {
			style = style.
				Foreground(accent).
				Bold(true).
				Background(lipgloss.Color("#2a2a3a")).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(accent)
		} else if selected {
			style = style.
				Foreground(accent).
				Bold(true)
		} else {
			style = style.
				Foreground(lipgloss.Color("#999999"))
		}

		rows = append(rows, style.Render(label))
	}

	content := strings.Join(rows, "\n")

	paneStyle := lipgloss.NewStyle().
		Border(subtleBorder).
		BorderForeground(borderColor).
		Width(width).
		Height(height)

	return paneStyle.Render(content)
}

func (m Model) renderContent(width, height int) string {
	isActive := m.focus == focusContent
	accent := lipgloss.Color(m.cfg.Theme.Accent)
	borderColor := lipgloss.Color("#3a3a3a")
	if isActive {
		borderColor = accent
	}

	viewportHeight := max(0, height-3)
	var body string
	switch m.activeView {
	case viewToday:
		body = m.renderTodayView(width-4, viewportHeight)
	case viewUpcoming:
		body = m.renderUpcomingView(width-4, viewportHeight)
	case viewLogbook:
		body = m.renderLogbookView(width-4, viewportHeight)
	}

	paneStyle := lipgloss.NewStyle().
		Border(subtleBorder).
		BorderForeground(borderColor).
		Width(width).
		Height(height)

	return paneStyle.Render(body)
}

func (m Model) renderTodayView(maxWidth, maxHeight int) string {
	rows, selectedLine := m.renderTodayRows(maxWidth)
	rows = m.scrollRows(rows, selectedLine, maxHeight)
	return strings.Join(rows, "\n")
}

func (m Model) renderTodayRows(maxWidth int) ([]string, int) {
	today := localToday()
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
		Bold(true)
	title := titleStyle.Render(fmt.Sprintf("  Today · %s", today.Format("Jan 02")))

	rows := []string{title, ""}
	selectedLine := -1

	isActive := m.focus == focusContent

	if len(m.todayTasks) == 0 {
		rows = append(rows, m.renderTodayEmptyState(maxWidth)...)
		return rows, selectedLine
	}

	selectedTaskIdx := -1
	if isActive && len(m.todayTasks) > m.contentCursor {
		selectedTaskIdx = m.todayTasks[m.contentCursor]
	}

	taskRows, taskSelectedLine := m.renderPrioritySeparatedRows(m.todayTasks, maxWidth, isActive, selectedTaskIdx, func(task Task) bool {
		return isTaskOverdue(task, today)
	})
	if taskSelectedLine >= 0 {
		selectedLine = len(rows) + taskSelectedLine
	}
	rows = append(rows, taskRows...)

	if len(taskRows) != len(m.todayTasks) {
		rows = append(rows, "")
	}

	return rows, selectedLine
}

func (m Model) renderTodayEmptyState(maxWidth int) []string {
	width := max(24, maxWidth-6)
	accent := lipgloss.Color(m.cfg.Theme.Accent)
	muted := lipgloss.Color(m.cfg.Theme.Muted)
	done := lipgloss.Color(m.cfg.Theme.Done)

	artStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
	kickerStyle := lipgloss.NewStyle().
		Foreground(done).
		Bold(true)
	titleStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
	subtitleStyle := lipgloss.NewStyle().
		Foreground(muted)

	cardStyle := lipgloss.NewStyle().
		Border(subtleBorder).
		BorderForeground(done).
		Padding(1, 3).
		Width(width)

	art := []string{
		" (')) ^v^  _           (`)_",
		"(__)_) ,--j j-------, (__)_)",
		"      /_.-.___.-.__/ \\",
		"    ,8| [_],-.[_] | oOo",
		",,,oO8|_o8_|_|_8o_|&888o,,,hjw",
	}

	content := []string{
		kickerStyle.Render("Day complete"),
		"",
		titleStyle.Render("All clear"),
		subtitleStyle.Render("Your list is quiet, sorted, and out of the way."),
		subtitleStyle.Render("Close the loop or enjoy the empty space."),
	}

	var body []string
	for _, line := range art {
		body = append(body, lipgloss.PlaceHorizontal(width, lipgloss.Center, artStyle.Render(line)))
	}
	body = append(body, "")
	for _, line := range content {
		body = append(body, lipgloss.PlaceHorizontal(width, lipgloss.Center, line))
	}

	return strings.Split(cardStyle.Render(strings.Join(body, "\n")), "\n")
}

func (m Model) renderUpcomingView(maxWidth, maxHeight int) string {
	rows, selectedLine := m.renderUpcomingRows(maxWidth)
	rows = m.scrollRows(rows, selectedLine, maxHeight)
	return strings.Join(rows, "\n")
}

func (m Model) renderUpcomingRows(maxWidth int) ([]string, int) {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.Accent)).Bold(true)
	title := titleStyle.Render("  Upcoming")

	rows := []string{title, ""}
	selectedLine := -1
	isActive := m.focus == focusContent
	selectedTaskIdx := -1
	if isActive {
		tasks := m.currentViewTasks()
		if len(tasks) > m.contentCursor {
			selectedTaskIdx = tasks[m.contentCursor]
		}
	}

	if len(m.upcomingGroups) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Muted)).
			Italic(true).
			PaddingLeft(2)
		rows = append(rows, emptyStyle.Render("Nothing upcoming"))
		return rows, selectedLine
	}

	for _, g := range m.upcomingGroups {
		headerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Upcoming))
		header := fmt.Sprintf("  ── %s %s", g.Label, strings.Repeat("─", max(0, maxWidth-len(g.Label)-6)))
		rows = append(rows, headerStyle.Render(header))

		taskRows, taskLine := m.renderPrioritySeparatedRows(g.Tasks, maxWidth, isActive, selectedTaskIdx, func(Task) bool { return false })
		if taskLine >= 0 {
			taskLine += len(rows)
			selectedLine = taskLine
		}
		rows = append(rows, taskRows...)
		rows = append(rows, "")
	}

	return rows, selectedLine
}

func (m Model) renderLogbookRows(maxWidth int) ([]string, int) {
	if len(m.logbookGroups) == 0 {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
			Bold(true)
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Muted)).
			Italic(true).
			PaddingLeft(2)
		rows := []string{
			titleStyle.Render("  Logbook"),
			"",
			emptyStyle.Render("Logbook is empty"),
		}
		return rows, -1
	}

	g := m.logbookGroups[m.logbookDayIndex]
	isActive := m.focus == focusContent

	leftArrow := "  "
	rightArrow := "  "
	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.Accent)).Bold(true)
	mutedArrow := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.Muted))
	if m.logbookDayIndex < len(m.logbookGroups)-1 {
		leftArrow = arrowStyle.Render("◀ ")
	} else {
		leftArrow = mutedArrow.Render("◀ ")
	}
	if m.logbookDayIndex > 0 {
		rightArrow = arrowStyle.Render(" ▶")
	} else {
		rightArrow = mutedArrow.Render(" ▶")
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
		Bold(true)
	dateLabel := g.Date.Format("Mon, Jan 02 2006")
	today := localToday()
	gDate := time.Date(g.Date.Year(), g.Date.Month(), g.Date.Day(), 0, 0, 0, 0, today.Location())
	if gDate.Equal(today) {
		dateLabel = "Today · " + g.Date.Format("Jan 02")
	} else if gDate.Equal(today.AddDate(0, 0, -1)) {
		dateLabel = "Yesterday · " + g.Date.Format("Jan 02")
	}
	title := leftArrow + titleStyle.Render(fmt.Sprintf(" %s ", dateLabel)) + rightArrow

	counterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.Muted))
	counter := counterStyle.Render(fmt.Sprintf("  %d/%d days", m.logbookDayIndex+1, len(m.logbookGroups)))

	rows := []string{"  " + title, counter, ""}
	selectedLine := -1

	for i, taskIdx := range g.Tasks {
		task := m.allTasks[taskIdx]
		selected := i == m.contentCursor && isActive
		row := m.renderLogbookTaskRow(task, selected, maxWidth)
		if selected {
			selectedLine = len(rows)
		}
		rows = append(rows, row)
	}

	if len(g.Tasks) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Muted)).
			Italic(true).
			PaddingLeft(2)
		rows = append(rows, emptyStyle.Render("No closed tasks"))
	}

	return rows, selectedLine
}

func (m Model) renderLogbookView(maxWidth, maxHeight int) string {
	rows, selectedLine := m.renderLogbookRows(maxWidth)
	rows = m.scrollRows(rows, selectedLine, maxHeight)
	return strings.Join(rows, "\n")
}

func prioritySectionLabel(priority int) (label string, color string) {
	switch priority {
	case PriorityHighest:
		return "P1 > Urgent Tasks", "#ff4d4f"
	case PriorityHigh:
		return "P2 > Important Tasks", "#f5c242"
	default:
		return "P3+ > Other Tasks", "#8e8e8e"
	}
}

func (m Model) renderPrioritySeparator(label string, maxWidth int, color string) string {
	sep := fmt.Sprintf("  ── %s ", label)
	pad := max(0, maxWidth-len(sep)-6)
	line := fmt.Sprintf("  ── %s %s", label, strings.Repeat("─", pad))
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Faint(true).
		Render(line)
}

func (m Model) renderPrioritySeparatedRows(taskIndices []int, maxWidth int, isActive bool, selectedTaskIdx int, isOverdue func(Task) bool) ([]string, int) {
	if len(taskIndices) == 0 {
		return nil, -1
	}

	var rows []string
	selectedRow := -1
	previous := ""

	if m.showPrioritySeparators {
		firstTask := m.allTasks[taskIndices[0]]
		firstSection, firstColor := prioritySectionLabel(firstTask.Priority)
		rows = append(rows, "")
		rows = append(rows, m.renderPrioritySeparator(firstSection, maxWidth, firstColor))
		rows = append(rows, "")
		previous = firstSection
	}

	for localIdx, taskIdx := range taskIndices {
		task := m.allTasks[taskIdx]
		section, color := prioritySectionLabel(task.Priority)
		if m.showPrioritySeparators && localIdx > 0 && section != previous {
			rows = append(rows, "")
			rows = append(rows, m.renderPrioritySeparator(section, maxWidth, color))
			rows = append(rows, "")
		}
		selected := isActive && taskIdx == selectedTaskIdx
		isOverdueTask := false
		if isOverdue != nil {
			isOverdueTask = isOverdue(task)
		}
		row := m.renderTaskRow(task, selected, maxWidth, isOverdueTask, m.selected[taskIdx])
		if selected {
			selectedRow = len(rows)
		}
		rows = append(rows, row)
		previous = section
	}

	return rows, selectedRow
}

func (m Model) scrollRows(rows []string, selectedLine int, maxHeight int) []string {
	if maxHeight <= 0 || len(rows) == 0 {
		return nil
	}

	offset := m.scrollOffset
	if offset < 0 {
		offset = 0
	}

	maxOffset := len(rows) - maxHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	if selectedLine >= 0 {
		if selectedLine < offset {
			offset = selectedLine
		}
		if selectedLine >= offset+maxHeight {
			offset = selectedLine - maxHeight + 1
		}
	}

	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	end := offset + maxHeight
	if end > len(rows) {
		end = len(rows)
	}

	return rows[offset:end]
}

func (m Model) renderTaskRow(task Task, cursor bool, maxWidth int, isOverdue bool, checked bool) string {
	bullet := "○"
	bulletColor := lipgloss.Color("#888888")
	if task.Done {
		bullet = "●"
		bulletColor = lipgloss.Color(m.cfg.Theme.Done)
	} else if task.Cancelled {
		bullet = "✕"
		bulletColor = lipgloss.Color(m.cfg.Theme.Overdue)
	} else if task.Blocked {
		bullet = "◆"
		bulletColor = lipgloss.Color("#d28b26")
	}
	if isOverdue {
		bulletColor = lipgloss.Color(m.cfg.Theme.Overdue)
	}

	prefix := "  "
	if checked {
		prefix = lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.Accent)).Render("▸ ")
	}

	bulletStyle := lipgloss.NewStyle().
		Foreground(bulletColor)

	desc := task.Description
	descMaxWidth := maxWidth - 8
	if task.Blocked {
		descMaxWidth -= 10
	}
	if descMaxWidth < 10 {
		descMaxWidth = 10
	}
	if len(desc) > descMaxWidth {
		desc = desc[:descMaxWidth-3] + "..."
	}

	descStyle := lipgloss.NewStyle().PaddingLeft(1)
	if task.IsCompleted() {
		descStyle = descStyle.
			Foreground(lipgloss.Color(m.cfg.Theme.Done)).
			Strikethrough(true)
	}
	if task.Cancelled {
		descStyle = descStyle.Foreground(lipgloss.Color(m.cfg.Theme.Overdue))
	}
	if task.Blocked {
		descStyle = descStyle.
			Foreground(lipgloss.Color("#d28b26")).
			Italic(true)
	}
	if isOverdue {
		descStyle = descStyle.Foreground(lipgloss.Color(m.cfg.Theme.Overdue))
	}

	var tagParts []string
	for _, tag := range task.Tags {
		tStyle := lipgloss.NewStyle().Foreground(tagColor(tag))
		tagParts = append(tagParts, tStyle.Render(tag))
	}
	tagStr := strings.Join(tagParts, " ")

	priorityStr := ""
	if emoji, ok := priorityEmojis[task.Priority]; ok {
		priorityStr = " " + emoji
	}

	stateStr := ""
	if task.Blocked {
		stateStr = " " + lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d28b26")).
			Bold(true).
			Render("blocked")
	}

	line := prefix + bulletStyle.Render(bullet) + descStyle.Render(desc)
	if priorityStr != "" {
		line += priorityStr
	}
	if stateStr != "" {
		line += stateStr
	}
	if tagStr != "" {
		line += " " + tagStr
	}

	rowStyle := lipgloss.NewStyle().Width(maxWidth)
	if cursor {
		rowStyle = rowStyle.
			Background(lipgloss.Color("#2a2a3a")).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color(m.cfg.Theme.Accent))
	}

	return rowStyle.Render(line)
}

func (m Model) renderLogbookTaskRow(task Task, selected bool, maxWidth int) string {
	bullet := "●"
	bulletColor := lipgloss.Color(m.cfg.Theme.Muted)
	textColor := lipgloss.Color(m.cfg.Theme.Muted)
	if task.Cancelled {
		bullet = "✕"
		bulletColor = lipgloss.Color(m.cfg.Theme.Overdue)
		textColor = lipgloss.Color(m.cfg.Theme.Overdue)
	}

	bulletStyle := lipgloss.NewStyle().
		Foreground(bulletColor).
		PaddingLeft(2)

	desc := task.Description
	descMaxWidth := maxWidth - 8
	if descMaxWidth < 10 {
		descMaxWidth = 10
	}
	if len(desc) > descMaxWidth {
		desc = desc[:descMaxWidth-3] + "..."
	}

	descStyle := lipgloss.NewStyle().
		PaddingLeft(1).
		Foreground(textColor).
		Strikethrough(true)

	var tagParts []string
	for _, tag := range task.Tags {
		tStyle := lipgloss.NewStyle().Foreground(textColor)
		tagParts = append(tagParts, tStyle.Render(tag))
	}
	tagStr := strings.Join(tagParts, " ")

	line := bulletStyle.Render(bullet) + descStyle.Render(desc)
	if tagStr != "" {
		line += " " + tagStr
	}

	rowStyle := lipgloss.NewStyle().Width(maxWidth)
	if selected {
		rowStyle = rowStyle.
			Background(lipgloss.Color("#2a2a3a")).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color(m.cfg.Theme.Accent))
	}

	return rowStyle.Render(line)
}

func (m Model) renderFooter(width int) string {
	var statusPart string
	if m.statusMsg != "" && time.Since(m.statusTime) < 3*time.Second {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
			Bold(true)
		statusPart = statusStyle.Render(" "+m.statusMsg) + "  "
	}

	var keys string
	if len(m.selected) > 0 {
		keys = fmt.Sprintf("(%d selected) d done  b blocked  s reschedule  v toggle all  esc clear  ? help  q quit", len(m.selected))
	} else if m.activeView == viewLogbook {
		keys = "←/→ prev/next day  d reopen  / filter  ? help  q quit"
	} else {
		toggleState := "off"
		if m.showPrioritySeparators {
			toggleState = "on"
		}
		keys = fmt.Sprintf("n new  d done  b blocked  f follow-up  s reschedule  p priority  e edit  D cancel  t separators(%s)  space select  v all  / filter  ? help", toggleState)
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666"))

	filterInfo := ""
	if m.filter != "" {
		filterStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.Accent)).
			Italic(true)
		filterInfo = filterStyle.Render(" [filter: " + m.filter + "] ")
	}

	return " " + statusPart + filterInfo + keyStyle.Render(keys)
}

func (m Model) renderHelp() string {
	helpText := `
  Obsidian Tasks TUI

  Navigation
    j/k  ↑/↓       Move up/down
    h/l             Sidebar / Content
    Tab             Toggle focus
    1/2/3           Today / Upcoming / Logbook
    ←/→             Logbook: prev/next day
    Enter           Toggle done

  Actions
    n               New task
    e               Edit task
    d               Toggle done/reopen
    b               Toggle blocked
    f               Create follow-up for tomorrow
    s               Reschedule task
    p               Set priority
    t               Toggle priority separators
    D               Cancel task
    /               Filter by text
    r               Reload from files

  Sync
    Auto-sync       Reloads when daily note files change
    r               Manual fallback reload

  Bulk Selection
    Space           Toggle select
    v               Select/deselect all
    d               Mark selected done
    b               Toggle selected blocked
    s               Reschedule selected
    Esc             Clear selection

  Press any key to close.
`
	style := lipgloss.NewStyle().
		Border(subtleBorder).
		BorderForeground(lipgloss.Color(m.cfg.Theme.Accent)).
		Padding(1, 4).
		Width(56)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		style.Render(helpText))
}

func parseRelativeDate(input string) (time.Time, error) {
	return parseNaturalDate(input, localToday())
}
