package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type taskDraft struct {
	Description string
	DueDate     time.Time
	Priority    int
}

var (
	taskDueEmojiRe     = regexp.MustCompile(`(?i)^(.*?)(?:\s+📅\s*(.+))$`)
	taskDueColonRe     = regexp.MustCompile(`(?i)^(.*?)(?:\s+due:\s+(.+))$`)
	taskDueKeywordRe   = regexp.MustCompile(`(?i)^(.*?)(?:\s+due\s+(.+))$`)
	taskWhenKeywordRe  = regexp.MustCompile(`(?i)^(.*?)(?:\s+(?:para|pra|ate|até)\s+(.+))$`)
	relativeDateRe     = regexp.MustCompile(`^(?:in\s+)?([+-]?\d+)\s*(d|day|days|w|week|weeks|m|month|months)$`)
	priorityTokenRe    = regexp.MustCompile(`(?i)(^|\s)(p[1-5])(\s|$)`)
	dateAccentReplacer = strings.NewReplacer(
		"á", "a",
		"à", "a",
		"ã", "a",
		"â", "a",
		"é", "e",
		"ê", "e",
		"í", "i",
		"ó", "o",
		"ô", "o",
		"õ", "o",
		"ú", "u",
		"ü", "u",
		"ç", "c",
	)
)

func normalizeDate(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func parseStoredDate(input string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(input), loc)
	if err != nil {
		return time.Time{}, err
	}
	return normalizeDate(t, loc), nil
}

func parseNaturalDate(input string, reference time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(input)
	normalized := normalizeNaturalDateInput(trimmed)
	reference = normalizeDate(reference, reference.Location())
	loc := reference.Location()

	if normalized == "" {
		return time.Time{}, fmt.Errorf("empty date input")
	}

	if t, err := parseStoredDate(trimmed, loc); err == nil {
		return t, nil
	}

	if t, ok := parseMonthDayWithYear(trimmed, loc); ok {
		return t, nil
	}

	if t, ok := parseMonthDayWithoutYear(trimmed, reference); ok {
		return t, nil
	}

	switch normalized {
	case "today", "tod":
		return reference, nil
	case "tomorrow", "tmr", "tom":
		return reference.AddDate(0, 0, 1), nil
	case "yesterday":
		return reference.AddDate(0, 0, -1), nil
	case "next week", "nw":
		return nextWeekday(reference, time.Monday, false), nil
	}

	if t, ok := parseWeekdayExpression(normalized, reference); ok {
		return t, nil
	}

	if t, ok := parseRelativeOffset(normalized, reference); ok {
		return t, nil
	}

	var plainDays int
	if _, err := fmt.Sscanf(normalized, "%d", &plainDays); err == nil {
		return reference.AddDate(0, 0, plainDays), nil
	}

	return time.Time{}, fmt.Errorf("unrecognized date: %s", trimmed)
}

func parseTaskDraftInput(input string, defaultDueDate, reference time.Time) (taskDraft, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return taskDraft{}, fmt.Errorf("empty task input")
	}

	draft := taskDraft{
		DueDate:  normalizeDate(defaultDueDate, reference.Location()),
		Priority: PriorityNone,
	}

	var foundPriority bool
	value, draft.Priority, foundPriority = extractPriorityToken(value)
	if !foundPriority {
		draft.Priority = PriorityNone
	}

	description, dueDate, matchedDue, err := extractTrailingDueDate(value, draft.DueDate, reference)
	if err != nil {
		return taskDraft{}, err
	}
	if matchedDue {
		draft.DueDate = dueDate
	}

	draft.Description = strings.TrimSpace(description)
	if draft.Description == "" {
		return taskDraft{}, fmt.Errorf("task description cannot be empty")
	}

	return draft, nil
}

func extractPriorityToken(input string) (string, int, bool) {
	loc := priorityTokenRe.FindStringSubmatchIndex(input)
	if loc == nil {
		return strings.TrimSpace(input), PriorityNone, false
	}

	token := strings.ToLower(input[loc[4]:loc[5]])
	priority := map[string]int{
		"p1": PriorityHighest,
		"p2": PriorityHigh,
		"p3": PriorityMedium,
		"p4": PriorityLow,
		"p5": PriorityLowest,
	}[token]

	leadingBoundary := input[loc[2]:loc[3]]
	trailingBoundary := input[loc[6]:loc[7]]
	replacement := leadingBoundary
	if replacement == "" {
		replacement = trailingBoundary
	}
	value := strings.TrimSpace(input[:loc[0]] + replacement + input[loc[1]:])
	return strings.Join(strings.Fields(value), " "), priority, true
}

func extractTrailingDueDate(input string, defaultDueDate, reference time.Time) (string, time.Time, bool, error) {
	value := strings.TrimSpace(input)
	if description, dueDate, matched, err := extractDueDateWithPattern(value, reference, taskDueEmojiRe, true); matched || err != nil {
		return description, dueDate, matched, err
	}
	if description, dueDate, matched, err := extractDueDateWithPattern(value, reference, taskDueColonRe, true); matched || err != nil {
		return description, dueDate, matched, err
	}
	if description, dueDate, matched, err := extractDueDateWithPattern(value, reference, taskDueKeywordRe, false); matched || err != nil {
		return description, dueDate, matched, err
	}
	if description, dueDate, matched, err := extractDueDateWithPattern(value, reference, taskWhenKeywordRe, false); matched || err != nil {
		return description, dueDate, matched, err
	}

	return value, normalizeDate(defaultDueDate, reference.Location()), false, nil
}

func extractDueDateWithPattern(input string, reference time.Time, pattern *regexp.Regexp, strict bool) (string, time.Time, bool, error) {
	matches := pattern.FindStringSubmatch(input)
	if matches == nil {
		return "", time.Time{}, false, nil
	}

	description := strings.TrimSpace(matches[1])
	dateExpr := strings.TrimSpace(matches[2])
	if dateExpr == "" {
		return "", time.Time{}, true, fmt.Errorf("missing due date after marker")
	}

	if !strict && !looksLikeDateExpression(dateExpr) {
		return "", time.Time{}, false, nil
	}

	dueDate, err := parseNaturalDate(dateExpr, reference)
	if err != nil {
		return "", time.Time{}, true, fmt.Errorf("invalid due date %q", dateExpr)
	}
	return description, dueDate, true, nil
}

func parseMonthDayWithYear(input string, loc *time.Location) (time.Time, bool) {
	input = canonicalizeDateWords(normalizeNaturalDateInput(input))
	layouts := []string{
		"01/02/2006",
		"1/2/2006",
		"Jan 2 2006",
		"Jan 02 2006",
		"January 2 2006",
		"January 02 2006",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, input, loc); err == nil {
			return normalizeDate(t, loc), true
		}
	}
	return time.Time{}, false
}

func parseMonthDayWithoutYear(input string, reference time.Time) (time.Time, bool) {
	input = canonicalizeDateWords(normalizeNaturalDateInput(input))
	layouts := []string{
		"01/02",
		"1/2",
		"Jan 2",
		"Jan 02",
		"January 2",
		"January 02",
	}

	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, input, reference.Location())
		if err != nil {
			continue
		}

		result := time.Date(reference.Year(), t.Month(), t.Day(), 0, 0, 0, 0, reference.Location())
		if result.Before(reference) {
			result = result.AddDate(1, 0, 0)
		}
		return result, true
	}
	return time.Time{}, false
}

func parseWeekdayExpression(input string, reference time.Time) (time.Time, bool) {
	fields := strings.Fields(input)
	if len(fields) == 0 || len(fields) > 2 {
		return time.Time{}, false
	}

	includeToday := false
	dayToken := fields[0]
	if len(fields) == 2 {
		switch fields[0] {
		case "this":
			includeToday = true
		case "next":
			includeToday = false
		default:
			return time.Time{}, false
		}
		dayToken = fields[1]
	}

	weekdays := map[string]time.Weekday{
		"mon":       time.Monday,
		"monday":    time.Monday,
		"tue":       time.Tuesday,
		"tues":      time.Tuesday,
		"tuesday":   time.Tuesday,
		"wed":       time.Wednesday,
		"wednesday": time.Wednesday,
		"thu":       time.Thursday,
		"thurs":     time.Thursday,
		"thursday":  time.Thursday,
		"fri":       time.Friday,
		"friday":    time.Friday,
		"sat":       time.Saturday,
		"saturday":  time.Saturday,
		"sun":       time.Sunday,
		"sunday":    time.Sunday,
	}

	weekday, ok := weekdays[dayToken]
	if !ok {
		return time.Time{}, false
	}

	return nextWeekday(reference, weekday, includeToday), true
}

func nextWeekday(reference time.Time, weekday time.Weekday, includeToday bool) time.Time {
	delta := (int(weekday) - int(reference.Weekday()) + 7) % 7
	if delta == 0 && !includeToday {
		delta = 7
	}
	return reference.AddDate(0, 0, delta)
}

func parseRelativeOffset(input string, reference time.Time) (time.Time, bool) {
	offsetStr := input
	if strings.HasPrefix(offsetStr, "+") {
		offsetStr = offsetStr[1:]
	}
	if len(offsetStr) >= 2 {
		unit := offsetStr[len(offsetStr)-1]
		numStr := offsetStr[:len(offsetStr)-1]
		if n, err := strconv.Atoi(numStr); err == nil {
			switch unit {
			case 'd':
				return reference.AddDate(0, 0, n), true
			case 'w':
				return reference.AddDate(0, 0, n*7), true
			case 'm':
				return reference.AddDate(0, n, 0), true
			}
		}
	}

	matches := relativeDateRe.FindStringSubmatch(input)
	if matches == nil {
		return time.Time{}, false
	}

	amount, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Time{}, false
	}

	switch matches[2] {
	case "d", "day", "days":
		return reference.AddDate(0, 0, amount), true
	case "w", "week", "weeks":
		return reference.AddDate(0, 0, amount*7), true
	case "m", "month", "months":
		return reference.AddDate(0, amount, 0), true
	default:
		return time.Time{}, false
	}
}

func canonicalizeDateWords(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	for i, field := range fields {
		if field == "" {
			continue
		}
		if field[0] < 'a' || field[0] > 'z' {
			continue
		}
		fields[i] = strings.ToUpper(field[:1]) + field[1:]
	}
	return strings.Join(fields, " ")
}

func looksLikeDateExpression(input string) bool {
	input = normalizeNaturalDateInput(input)
	if input == "" {
		return false
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return false
	}

	first := fields[0]
	if first == "today" || first == "tod" ||
		first == "tomorrow" || first == "tmr" || first == "tom" ||
		first == "yesterday" ||
		first == "next" || first == "this" ||
		first == "in" || first == "nw" {
		return true
	}

	weekdays := map[string]bool{
		"mon": true, "monday": true,
		"tue": true, "tues": true, "tuesday": true,
		"wed": true, "wednesday": true,
		"thu": true, "thurs": true, "thursday": true,
		"fri": true, "friday": true,
		"sat": true, "saturday": true,
		"sun": true, "sunday": true,
	}
	if weekdays[first] {
		return true
	}

	months := map[string]bool{
		"jan": true, "january": true,
		"feb": true, "february": true,
		"mar": true, "march": true,
		"apr": true, "april": true,
		"may": true,
		"jun": true, "june": true,
		"jul": true, "july": true,
		"aug": true, "august": true,
		"sep": true, "sept": true, "september": true,
		"oct": true, "october": true,
		"nov": true, "november": true,
		"dec": true, "december": true,
	}
	if months[first] {
		return true
	}

	switch first[0] {
	case '+':
		if len(first) > 1 {
			return true
		}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	}

	return false
}

func normalizeNaturalDateInput(input string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
	for i, field := range fields {
		fields[i] = normalizeDateToken(field)
	}
	return strings.Join(fields, " ")
}

func normalizeDateToken(token string) string {
	token = dateAccentReplacer.Replace(token)
	token = strings.TrimSpace(token)
	token = strings.Trim(token, ".,;")

	switch token {
	case "hoje":
		return "today"
	case "amanha":
		return "tomorrow"
	case "ontem":
		return "yesterday"
	case "proxima", "proximo":
		return "next"
	case "esta", "este", "nessa", "nesse":
		return "this"
	case "em":
		return "in"
	case "seg", "segunda", "segunda-feira":
		return "monday"
	case "ter", "terca", "terca-feira":
		return "tuesday"
	case "qua", "quarta", "quarta-feira":
		return "wednesday"
	case "qui", "quinta", "quinta-feira":
		return "thursday"
	case "sex", "sexta", "sexta-feira":
		return "friday"
	case "sab", "sabado", "sabado-feira":
		return "saturday"
	case "dom", "domingo":
		return "sunday"
	case "jan", "janeiro":
		return "jan"
	case "fev", "fevereiro":
		return "feb"
	case "mar", "marco":
		return "mar"
	case "abr", "abril":
		return "apr"
	case "mai", "maio":
		return "may"
	case "jun", "junho":
		return "jun"
	case "jul", "julho":
		return "jul"
	case "ago", "agosto":
		return "aug"
	case "set", "setembro":
		return "sep"
	case "out", "outubro":
		return "oct"
	case "nov", "novembro":
		return "nov"
	case "dez", "dezembro":
		return "dec"
	case "dia":
		return "day"
	case "dias":
		return "days"
	case "semana":
		return "week"
	case "semanas":
		return "weeks"
	case "mes":
		return "month"
	case "meses":
		return "months"
	default:
		return token
	}
}
