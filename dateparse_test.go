package main

import (
	"testing"
	"time"
)

func TestParseNaturalDate(t *testing.T) {
	ref := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)

	tests := []struct {
		input string
		want  time.Time
	}{
		{input: "2026-04-05", want: time.Date(2026, time.April, 5, 0, 0, 0, 0, time.Local)},
		{input: "04/05", want: time.Date(2026, time.April, 5, 0, 0, 0, 0, time.Local)},
		{input: "1/2", want: time.Date(2027, time.January, 2, 0, 0, 0, 0, time.Local)},
		{input: "April 5", want: time.Date(2026, time.April, 5, 0, 0, 0, 0, time.Local)},
		{input: "jan 2 2028", want: time.Date(2028, time.January, 2, 0, 0, 0, 0, time.Local)},
		{input: "today", want: time.Date(2026, time.March, 27, 0, 0, 0, 0, time.Local)},
		{input: "hoje", want: time.Date(2026, time.March, 27, 0, 0, 0, 0, time.Local)},
		{input: "tomorrow", want: time.Date(2026, time.March, 28, 0, 0, 0, 0, time.Local)},
		{input: "amanhã", want: time.Date(2026, time.March, 28, 0, 0, 0, 0, time.Local)},
		{input: "next week", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "próxima semana", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "mon", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "segunda", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "next mon", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "próxima seg", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "this fri", want: time.Date(2026, time.March, 27, 0, 0, 0, 0, time.Local)},
		{input: "esta sexta", want: time.Date(2026, time.March, 27, 0, 0, 0, 0, time.Local)},
		{input: "+3d", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "in 2 weeks", want: time.Date(2026, time.April, 10, 0, 0, 0, 0, time.Local)},
		{input: "em 2 semanas", want: time.Date(2026, time.April, 10, 0, 0, 0, 0, time.Local)},
		{input: "3 days", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "3 dias", want: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local)},
		{input: "2m", want: time.Date(2026, time.May, 27, 0, 0, 0, 0, time.Local)},
		{input: "2 meses", want: time.Date(2026, time.May, 27, 0, 0, 0, 0, time.Local)},
		{input: "5", want: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.Local)},
		{input: "abril 5", want: time.Date(2026, time.April, 5, 0, 0, 0, 0, time.Local)},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseNaturalDate(tc.input, ref)
			if err != nil {
				t.Fatalf("parseNaturalDate returned error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("unexpected date for %q\nexpected: %s\nactual:   %s", tc.input, tc.want.Format("2006-01-02"), got.Format("2006-01-02"))
			}
		})
	}
}

func TestParseNaturalDateRejectsUnknownInput(t *testing.T) {
	ref := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)

	if _, err := parseNaturalDate("soon-ish", ref); err == nil {
		t.Fatal("expected unknown date input to fail")
	}
}

func TestParseTaskDraftInput(t *testing.T) {
	ref := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)
	defaultDueDate := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.Local)

	tests := []struct {
		name        string
		input       string
		wantDesc    string
		wantDueDate time.Time
		wantPrio    int
	}{
		{
			name:        "default due date preserved without marker",
			input:       "Ship release #work",
			wantDesc:    "Ship release #work",
			wantDueDate: defaultDueDate,
			wantPrio:    PriorityNone,
		},
		{
			name:        "emoji due marker supports natural language",
			input:       "Ship release #work 📅 tomorrow",
			wantDesc:    "Ship release #work",
			wantDueDate: time.Date(2026, time.March, 28, 0, 0, 0, 0, time.Local),
			wantPrio:    PriorityNone,
		},
		{
			name:        "due keyword and priority in either order",
			input:       "Ship release due next mon p2",
			wantDesc:    "Ship release",
			wantDueDate: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local),
			wantPrio:    PriorityHigh,
		},
		{
			name:        "portuguese marker supports natural language",
			input:       "Enviar proposta #work para próxima seg p2",
			wantDesc:    "Enviar proposta #work",
			wantDueDate: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.Local),
			wantPrio:    PriorityHigh,
		},
		{
			name:        "priority before due marker",
			input:       "Ship release p4 📅 2026-04-05",
			wantDesc:    "Ship release",
			wantDueDate: time.Date(2026, time.April, 5, 0, 0, 0, 0, time.Local),
			wantPrio:    PriorityLow,
		},
		{
			name:        "ordinary due wording stays in description without explicit marker",
			input:       "Investigate due process tomorrow",
			wantDesc:    "Investigate due process tomorrow",
			wantDueDate: defaultDueDate,
			wantPrio:    PriorityNone,
		},
		{
			name:        "ordinary para wording stays in description without explicit marker",
			input:       "Enviar proposta para cliente amanhã",
			wantDesc:    "Enviar proposta para cliente amanhã",
			wantDueDate: defaultDueDate,
			wantPrio:    PriorityNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			draft, err := parseTaskDraftInput(tc.input, defaultDueDate, PriorityNone, ref)
			if err != nil {
				t.Fatalf("parseTaskDraftInput returned error: %v", err)
			}
			if draft.Description != tc.wantDesc {
				t.Fatalf("unexpected description\nexpected: %q\nactual:   %q", tc.wantDesc, draft.Description)
			}
			if !draft.DueDate.Equal(tc.wantDueDate) {
				t.Fatalf("unexpected due date\nexpected: %s\nactual:   %s", tc.wantDueDate.Format("2006-01-02"), draft.DueDate.Format("2006-01-02"))
			}
			if draft.Priority != tc.wantPrio {
				t.Fatalf("unexpected priority\nexpected: %d\nactual:   %d", tc.wantPrio, draft.Priority)
			}
		})
	}
}

func TestParseTaskDraftInputRejectsInvalidExplicitDueDate(t *testing.T) {
	ref := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)
	defaultDueDate := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.Local)

	if _, err := parseTaskDraftInput("Ship release due: sometime", defaultDueDate, PriorityNone, ref); err == nil {
		t.Fatal("expected invalid explicit due date to fail")
	}
}

func TestParseTaskDraftInputUsesInheritedPriorityWhenMissing(t *testing.T) {
	ref := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)
	defaultDueDate := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.Local)

	draft, err := parseTaskDraftInput("Ship release #work", defaultDueDate, PriorityMedium, ref)
	if err != nil {
		t.Fatalf("parseTaskDraftInput returned error: %v", err)
	}
	if draft.Priority != PriorityMedium {
		t.Fatalf("expected inherited priority %d, got %d", PriorityMedium, draft.Priority)
	}
}

func TestParseTaskDraftInputExplicitPriorityOverridesInheritedPriority(t *testing.T) {
	ref := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.Local)
	defaultDueDate := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.Local)

	draft, err := parseTaskDraftInput("Ship release p1", defaultDueDate, PriorityLow, ref)
	if err != nil {
		t.Fatalf("parseTaskDraftInput returned error: %v", err)
	}
	if draft.Priority != PriorityHighest {
		t.Fatalf("expected explicit priority %d, got %d", PriorityHighest, draft.Priority)
	}
}

func TestParseStoredDateUsesRequestedLocation(t *testing.T) {
	loc := time.FixedZone("UTC-03", -3*60*60)
	got, err := parseStoredDate("2026-03-27", loc)
	if err != nil {
		t.Fatalf("parseStoredDate returned error: %v", err)
	}

	want := time.Date(2026, time.March, 27, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("unexpected stored date\nexpected: %s\nactual:   %s", want, got)
	}
	if got.Location() != loc {
		t.Fatalf("expected location %s, got %s", loc, got.Location())
	}
}
