package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	PriorityNone    = 3
	PriorityHighest = 0
	PriorityHigh    = 1
	PriorityMedium  = 2
	PriorityLow     = 4
	PriorityLowest  = 5
)

var priorityEmojis = map[int]string{
	PriorityHighest: "🔺",
	PriorityHigh:    "⏫",
	PriorityMedium:  "🔼",
	PriorityLow:     "🔽",
	PriorityLowest:  "⏬",
}

var emojiToPriority = map[string]int{
	"🔺": PriorityHighest,
	"⏫": PriorityHigh,
	"🔼": PriorityMedium,
	"🔽": PriorityLow,
	"⏬": PriorityLowest,
}

type Task struct {
	Description    string
	Blocked        bool
	Done           bool
	Cancelled      bool
	Tags           []string
	Priority       int
	DueDate        time.Time
	CompletionDate time.Time
	CancelledDate  time.Time
	FilePath       string
	LineNumber     int
	RawLine        string
}

var (
	taskRe          = regexp.MustCompile(`^(\s*)-\s\[([ xXbB-])\]\s*(.*)$`)
	tagRe           = regexp.MustCompile(`#[\w]+(?:/[\w]+)*`)
	dueDateRe       = regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`)
	doneDateRe      = regexp.MustCompile(`✅\s*(\d{4}-\d{2}-\d{2})`)
	cancelledDateRe = regexp.MustCompile(`❌\s*(\d{4}-\d{2}-\d{2})`)
	priorityRe      = regexp.MustCompile(`[🔺⏫🔼🔽⏬]`)
)

// ParseTask parses a single markdown line into a Task, if it matches.
// noteDate is the date derived from the daily note filename (fallback due date).
func ParseTask(line string, filePath string, lineNumber int, noteDate time.Time) (*Task, bool) {
	m := taskRe.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}

	done := m[2] == "x" || m[2] == "X"
	cancelled := m[2] == "-"
	blocked := m[2] == "b" || m[2] == "B"
	rest := m[3]

	// Extract tags
	tags := tagRe.FindAllString(rest, -1)

	// Extract due date
	var dueDate time.Time
	if dm := dueDateRe.FindStringSubmatch(rest); dm != nil {
		if t, err := parseStoredDate(dm[1], noteDate.Location()); err == nil {
			dueDate = t
		}
	}
	if dueDate.IsZero() {
		dueDate = noteDate
	}

	// Extract completion date
	var completionDate time.Time
	if cm := doneDateRe.FindStringSubmatch(rest); cm != nil {
		if t, err := parseStoredDate(cm[1], noteDate.Location()); err == nil {
			completionDate = t
		}
	}

	var cancelledDate time.Time
	if cm := cancelledDateRe.FindStringSubmatch(rest); cm != nil {
		if t, err := parseStoredDate(cm[1], noteDate.Location()); err == nil {
			cancelledDate = t
		}
	}

	priority := PriorityNone
	if pm := priorityRe.FindString(rest); pm != "" {
		if p, ok := emojiToPriority[pm]; ok {
			priority = p
		}
	}

	desc := rest
	desc = tagRe.ReplaceAllString(desc, "")
	desc = dueDateRe.ReplaceAllString(desc, "")
	desc = doneDateRe.ReplaceAllString(desc, "")
	desc = cancelledDateRe.ReplaceAllString(desc, "")
	desc = priorityRe.ReplaceAllString(desc, "")
	desc = strings.TrimSpace(desc)

	return &Task{
		Description:    desc,
		Blocked:        blocked,
		Done:           done,
		Cancelled:      cancelled,
		Tags:           tags,
		Priority:       priority,
		DueDate:        dueDate,
		CompletionDate: completionDate,
		CancelledDate:  cancelledDate,
		FilePath:       filePath,
		LineNumber:     lineNumber,
		RawLine:        line,
	}, true
}

func (t Task) IsCompleted() bool {
	return t.Done || t.Cancelled
}

func (t Task) ClosedDate() time.Time {
	if t.Done && !t.CompletionDate.IsZero() {
		return t.CompletionDate
	}
	if t.Cancelled && !t.CancelledDate.IsZero() {
		return t.CancelledDate
	}
	return time.Time{}
}

// ParseFile reads a daily note and extracts tasks within the given sections.
// If sectionHeadings is empty, all tasks in the file are returned.
func ParseFile(filePath string, noteDate time.Time, sectionHeadings []string) ([]Task, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Build a set of headings to match and determine the common heading level
	headingSet := make(map[string]bool, len(sectionHeadings))
	sectionLevel := ""
	for _, h := range sectionHeadings {
		headingSet[h] = true
		if sectionLevel == "" {
			for _, ch := range h {
				if ch == '#' {
					sectionLevel += "#"
				} else {
					break
				}
			}
		}
	}

	noFilter := len(sectionHeadings) == 0

	var tasks []Task
	scanner := bufio.NewScanner(f)
	lineNum := 0
	inSection := noFilter

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !noFilter {
			if headingSet[trimmed] {
				inSection = true
				continue
			}
			// Exit section when hitting another heading at the same level
			if inSection && strings.HasPrefix(trimmed, sectionLevel+" ") && !strings.HasPrefix(trimmed, sectionLevel+"#") {
				inSection = false
				continue
			}
		}

		if inSection {
			if t, ok := ParseTask(line, filePath, lineNum, noteDate); ok {
				tasks = append(tasks, *t)
			}
		}
	}
	return tasks, scanner.Err()
}

type cachedDailyNote struct {
	modTime time.Time
	size    int64
	tasks   []Task
}

type DailyNotesCache struct {
	cfg   Config
	files map[string]cachedDailyNote
}

func NewDailyNotesCache(cfg Config) (*DailyNotesCache, []Task, error) {
	cache := &DailyNotesCache{
		cfg:   cfg,
		files: make(map[string]cachedDailyNote),
	}
	tasks, err := cache.ReloadAll()
	if err != nil {
		return nil, nil, err
	}
	return cache, tasks, nil
}

func dailyNotePath(cfg Config, date time.Time) string {
	dir := filepath.Join(cfg.Vault.Path, cfg.Vault.DailyNotesDir)
	filename := date.Format(cfg.Vault.DailyNoteFormat) + ".md"
	return filepath.Join(dir, filename)
}

func dailyNotesRange(cfg Config, now time.Time) map[string]time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := today.AddDate(0, 0, -cfg.Tasks.LogbookDays)
	end := today.AddDate(0, 0, cfg.Tasks.LookaheadDays)

	paths := make(map[string]time.Time)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		paths[dailyNotePath(cfg, d)] = d
	}
	return paths
}

func shouldExcludeTask(cfg Config, task Task) bool {
	for _, tag := range task.Tags {
		for _, ex := range cfg.Tasks.ExcludeTags {
			if tag == ex || strings.HasPrefix(tag, ex+"/") {
				return true
			}
		}
	}
	return false
}

func (c *DailyNotesCache) ReloadAll() ([]Task, error) {
	relevant := dailyNotesRange(c.cfg, time.Now())
	for path, noteDate := range relevant {
		if err := c.refreshFile(path, noteDate); err != nil {
			return nil, err
		}
	}

	for path := range c.files {
		if _, ok := relevant[path]; !ok {
			delete(c.files, path)
		}
	}

	return c.collectTasks(relevant), nil
}

func (c *DailyNotesCache) ReloadPaths(paths []string) ([]Task, error) {
	if len(paths) == 0 {
		return c.ReloadAll()
	}

	relevant := dailyNotesRange(c.cfg, time.Now())
	for path := range c.files {
		if _, ok := relevant[path]; !ok {
			delete(c.files, path)
		}
	}
	for path, noteDate := range relevant {
		if _, ok := c.files[path]; ok {
			continue
		}
		if err := c.refreshFile(path, noteDate); err != nil {
			return nil, err
		}
	}

	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		noteDate, ok := relevant[path]
		if !ok {
			delete(c.files, path)
			continue
		}
		if err := c.refreshFile(path, noteDate); err != nil {
			return nil, err
		}
	}

	return c.collectTasks(relevant), nil
}

func (c *DailyNotesCache) refreshFile(path string, noteDate time.Time) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			delete(c.files, path)
			return nil
		}
		return err
	}

	if cached, ok := c.files[path]; ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return nil
	}

	tasks, err := ParseFile(path, noteDate, c.cfg.Tasks.EffectiveSectionHeadings())
	if err != nil {
		return nil
	}

	filtered := tasks[:0]
	for _, task := range tasks {
		if shouldExcludeTask(c.cfg, task) {
			continue
		}
		filtered = append(filtered, task)
	}

	c.files[path] = cachedDailyNote{
		modTime: info.ModTime(),
		size:    info.Size(),
		tasks:   append([]Task(nil), filtered...),
	}
	return nil
}

func (c *DailyNotesCache) collectTasks(relevant map[string]time.Time) []Task {
	paths := make([]string, 0, len(relevant))
	for path := range relevant {
		if _, ok := c.files[path]; ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	var allTasks []Task
	for _, path := range paths {
		allTasks = append(allTasks, c.files[path].tasks...)
	}
	return allTasks
}

// ScanDailyNotes scans the daily notes directory for tasks within the configured date range.
func ScanDailyNotes(cfg Config) ([]Task, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := today.AddDate(0, 0, -cfg.Tasks.LogbookDays)
	end := today.AddDate(0, 0, cfg.Tasks.LookaheadDays)

	var allTasks []Task

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		fp := dailyNotePath(cfg, d)
		if _, err := os.Stat(fp); err != nil {
			continue
		}
		tasks, err := ParseFile(fp, d, cfg.Tasks.EffectiveSectionHeadings())
		if err != nil {
			continue
		}
		for _, t := range tasks {
			if !shouldExcludeTask(cfg, t) {
				allTasks = append(allTasks, t)
			}
		}
	}

	return allTasks, nil
}

func ToggleDone(task *Task) error {
	lines, err := readLines(task.FilePath)
	if err != nil {
		return err
	}
	idx := task.LineNumber - 1
	if err := verifyLine(lines, idx, task.RawLine); err != nil {
		return err
	}

	line := lines[idx]
	if task.IsCompleted() {
		// Reopen: [x]/[-] → [ ], remove completion markers
		line = replaceTaskStatus(line, " ")
		line = doneDateRe.ReplaceAllString(line, "")
		line = cancelledDateRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		task.Blocked = false
		task.Done = false
		task.Cancelled = false
		task.CompletionDate = time.Time{}
		task.CancelledDate = time.Time{}
	} else {
		// Done: [ ]/[b] → [x], append ✅ date
		line = replaceTaskStatus(line, "x")
		line = doneDateRe.ReplaceAllString(line, "")
		line = cancelledDateRe.ReplaceAllString(line, "")
		now := time.Now()
		todayLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		line = strings.TrimRight(line, " ")
		line = line + " ✅ " + todayLocal.Format("2006-01-02")
		task.Blocked = false
		task.Done = true
		task.Cancelled = false
		task.CompletionDate = todayLocal
		task.CancelledDate = time.Time{}
	}

	lines[idx] = line
	task.RawLine = line
	return writeLines(task.FilePath, lines)
}

func CancelTask(task *Task) error {
	lines, err := readLines(task.FilePath)
	if err != nil {
		return err
	}
	idx := task.LineNumber - 1
	if err := verifyLine(lines, idx, task.RawLine); err != nil {
		return err
	}

	line := replaceTaskStatus(lines[idx], "-")
	line = doneDateRe.ReplaceAllString(line, "")
	line = cancelledDateRe.ReplaceAllString(line, "")
	now := time.Now()
	todayLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	line = strings.TrimRight(line, " ")
	line = line + " ❌ " + todayLocal.Format("2006-01-02")

	lines[idx] = line
	task.Blocked = false
	task.Done = false
	task.Cancelled = true
	task.CompletionDate = time.Time{}
	task.CancelledDate = todayLocal
	task.RawLine = line
	return writeLines(task.FilePath, lines)
}

func ToggleBlocked(task *Task) error {
	lines, err := readLines(task.FilePath)
	if err != nil {
		return err
	}
	idx := task.LineNumber - 1
	if err := verifyLine(lines, idx, task.RawLine); err != nil {
		return err
	}

	line := lines[idx]
	if task.Blocked {
		line = replaceTaskStatus(line, " ")
		task.Blocked = false
	} else {
		line = replaceTaskStatus(line, "b")
		line = doneDateRe.ReplaceAllString(line, "")
		line = cancelledDateRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		task.Blocked = true
		task.Done = false
		task.Cancelled = false
		task.CompletionDate = time.Time{}
		task.CancelledDate = time.Time{}
	}

	lines[idx] = line
	task.RawLine = line
	return writeLines(task.FilePath, lines)
}

func buildTaskLine(description string, tags []string, priority int, dueDate time.Time, done bool, cancelled bool, blocked bool, completionDate time.Time, cancelledDate time.Time) string {
	status := "[ ]"
	if done {
		status = "[x]"
	} else if cancelled {
		status = "[-]"
	} else if blocked {
		status = "[b]"
	}

	var b strings.Builder
	b.WriteString("- ")
	b.WriteString(status)
	b.WriteString(" ")
	b.WriteString(strings.TrimSpace(description))

	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			continue
		}
		b.WriteString(" ")
		b.WriteString(tag)
	}

	if emoji, ok := priorityEmojis[priority]; ok {
		b.WriteString(" ")
		b.WriteString(emoji)
	}

	if !dueDate.IsZero() {
		b.WriteString(" 📅 ")
		b.WriteString(dueDate.Format("2006-01-02"))
	}

	if done && !completionDate.IsZero() {
		b.WriteString(" ✅ ")
		b.WriteString(completionDate.Format("2006-01-02"))
	}

	if cancelled && !cancelledDate.IsZero() {
		b.WriteString(" ❌ ")
		b.WriteString(cancelledDate.Format("2006-01-02"))
	}

	return b.String()
}

func appendTaskLine(cfg Config, dueDate time.Time, taskLine string) error {
	dir := filepath.Join(cfg.Vault.Path, cfg.Vault.DailyNotesDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Use the first effective heading for task creation
	heading := cfg.Tasks.EffectiveSectionHeadings()[0]

	fp := dailyNotePath(cfg, dueDate)

	// If file doesn't exist, create with template
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		content := fmt.Sprintf(`---
created: %s
---

%s

%s

---
`, dueDate.Format("2006-01-02"), heading, taskLine)
		return os.WriteFile(fp, []byte(content), 0644)
	}

	// File exists — insert under first matching section heading
	lines, err := readLines(fp)
	if err != nil {
		return err
	}

	insertIdx := -1
	for _, h := range cfg.Tasks.EffectiveSectionHeadings() {
		for i, l := range lines {
			if strings.TrimSpace(l) == h {
				insertIdx = i + 1
				// Skip blank lines after heading
				for insertIdx < len(lines) && strings.TrimSpace(lines[insertIdx]) == "" {
					insertIdx++
				}
				break
			}
		}
		if insertIdx >= 0 {
			break
		}
	}

	if insertIdx == -1 {
		// No heading found, append with first heading
		lines = append(lines, "", heading, "", taskLine)
	} else {
		// Insert at position
		lines = append(lines[:insertIdx], append([]string{taskLine}, lines[insertIdx:]...)...)
	}

	return writeLines(fp, lines)
}

// CreateTask appends a new task to the appropriate daily note file.
func CreateTask(cfg Config, description string, dueDate time.Time, priority int) error {
	taskLine := buildTaskLine(description, nil, priority, dueDate, false, false, false, time.Time{}, time.Time{})
	return appendTaskLine(cfg, dueDate, taskLine)
}

func CreateFollowUpTask(cfg Config, task Task) (time.Time, error) {
	followUpDate := localToday().AddDate(0, 0, 1)
	description := strings.TrimSpace(task.Description)
	switch {
	case strings.HasPrefix(strings.ToLower(description), "follow up:"):
	case strings.HasPrefix(strings.ToLower(description), "follow-up:"):
	default:
		description = "Follow up: " + description
	}

	taskLine := buildTaskLine(description, task.Tags, task.Priority, followUpDate, false, false, false, time.Time{}, time.Time{})
	if err := appendTaskLine(cfg, followUpDate, taskLine); err != nil {
		return time.Time{}, err
	}

	return followUpDate, nil
}

func RescheduleTask(task *Task, newDate time.Time) error {
	lines, err := readLines(task.FilePath)
	if err != nil {
		return err
	}
	idx := task.LineNumber - 1
	if err := verifyLine(lines, idx, task.RawLine); err != nil {
		return err
	}

	line := lines[idx]
	newDateStr := newDate.Format("2006-01-02")
	if dueDateRe.MatchString(line) {
		line = dueDateRe.ReplaceAllString(line, "📅 "+newDateStr)
	} else {
		line = line + " 📅 " + newDateStr
	}

	lines[idx] = line
	task.RawLine = line
	task.DueDate = newDate
	return writeLines(task.FilePath, lines)
}

func SetPriority(task *Task, priority int) error {
	lines, err := readLines(task.FilePath)
	if err != nil {
		return err
	}
	idx := task.LineNumber - 1
	if err := verifyLine(lines, idx, task.RawLine); err != nil {
		return err
	}

	line := lines[idx]
	line = priorityRe.ReplaceAllString(line, "")
	cbIdx := strings.Index(line, "] ")
	if cbIdx >= 0 {
		prefix := line[:cbIdx+2]
		rest := line[cbIdx+2:]
		for strings.Contains(rest, "  ") {
			rest = strings.Replace(rest, "  ", " ", 1)
		}
		line = prefix + strings.TrimSpace(rest)
	}

	if emoji, ok := priorityEmojis[priority]; ok {
		if loc := dueDateRe.FindStringIndex(line); loc != nil {
			line = line[:loc[0]] + emoji + " " + line[loc[0]:]
		} else {
			line = line + " " + emoji
		}
	}

	lines[idx] = line
	task.RawLine = line
	task.Priority = priority
	return writeLines(task.FilePath, lines)
}

func UpdateTaskLine(task *Task, newLine string) error {
	lines, err := readLines(task.FilePath)
	if err != nil {
		return err
	}
	idx := task.LineNumber - 1
	if err := verifyLine(lines, idx, task.RawLine); err != nil {
		return err
	}
	lines[idx] = newLine
	task.RawLine = newLine
	return writeLines(task.FilePath, lines)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	if content == "" {
		return []string{}, nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func writeLines(path string, lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

func verifyLine(lines []string, idx int, rawLine string) error {
	if idx < 0 || idx >= len(lines) {
		return fmt.Errorf("line %d out of range (file has %d lines)", idx+1, len(lines))
	}
	if lines[idx] != rawLine {
		return fmt.Errorf("file changed externally, please reload (r)")
	}
	return nil
}

func replaceTaskStatus(line string, statusSymbol string) string {
	m := taskRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	return fmt.Sprintf("%s- [%s] %s", m[1], statusSymbol, m[3])
}
