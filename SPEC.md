# Obsidian Tasks TUI — Specification

A beautiful terminal UI for managing Obsidian tasks, built with Go + Bubble Tea.

## Overview

Read and write tasks directly from Obsidian daily notes (Markdown files). No intermediate format — the markdown files are the source of truth, and the TUI should auto-sync when they change.

## Vault Configuration

- Vault path: configurable via `~/.config/obsidian-tasks/config.toml` or `--vault` flag
- Daily notes location: `<vault>/Notes/Daily Notes/`
- Daily note filename format: `YYYY-MM-DD.md`

## Task Format

Tasks live inside daily notes as Markdown checkboxes:

```markdown
- [ ] Task description #tag/subtag 📅 2026-02-28
- [x] Completed task #project/kanastra 📅 2026-02-27 ✅ 2026-02-27
```

### Fields:
- `[ ]` or `[x]` — status (open/done)
- Task text — the description
- `#tag/path` — optional tags (can have multiple), used for project/context grouping
- `📅 YYYY-MM-DD` — due date
- `✅ YYYY-MM-DD` — completion date (added when marking done)

### Parsing rules:
- Tasks are lines starting with `- [ ]` or `- [x]` (with optional leading whitespace)
- Tags: `#word` or `#word/word/word` patterns
- Due date: `📅 YYYY-MM-DD` anywhere in the line
- Completion date: `✅ YYYY-MM-DD` anywhere in the line
- Everything else between the checkbox and the first # or 📅 is the task description

## TUI Layout

### IMPORTANT: Clone the Yatto UI style!

Reference implementation is in `yatto-reference/` folder. Study `internal/models/` for the Bubble Tea patterns:
- `projectList.go` — left pane with project list
- `taskList.go` — right pane with task list  
- `taskForm.go` — form for creating/editing tasks
- `taskPager.go` — task detail view
- `definitions.go` — main model, Update(), View()
- `helpers.go` — shared helpers

The yatto UI uses a **2-pane layout**: project list on the left, task list on the right. We adapt this to:
- **Left pane**: Date list (Overdue dates, Today, Upcoming dates) — like yatto's project list
- **Right pane**: Tasks for the selected date — like yatto's task list
- **Task detail/form**: For creating/editing tasks — like yatto's task form

```
┌─ Dates ──────────┬─ Tasks ──────────────────────────────────────┐
│ ▸ Overdue (3)    │  ○ Fix bug                     #kanastra    │
│   Feb 25         │  ● Send invoice                #personal    │
│   Feb 26         │  ○ Call dentist                 #reminders   │
│ ▸ Today          │                                              │
│   Feb 28         │                                              │
│ ▸ Upcoming       │                                              │
│   Mar 01         │                                              │
│   Mar 02         │                                              │
│   Mar 03         │                                              │
└──────────────────┴──────────────────────────────────────────────┘
 n new · d done · f follow-up · e edit · D cancel · s reschedule · / filter · ? help
```

### Design principles:
- **Bubble Tea** for the TUI framework  
- **Lip Gloss** for styling (borders, colors, padding)
- **Same aesthetic as Yatto** — clean, minimal, beautiful borders
- 2-pane layout: Dates (left) | Tasks (right)
- Color coding:
  - Overdue dates: red/warm accent
  - Today: primary/blue accent, bold
  - Upcoming dates: muted/gray
  - Done tasks: strikethrough + dimmed green
  - Tags: colored badges (consistent color per tag hash)
- Selected item highlighted with a subtle background
- Status indicators: ○ open, ● done
- Smooth cursor movement between panes (Tab/h/l) and items (j/k)

### Keybindings:

| Key | Action |
|-----|--------|
| `j/k` or `↑/↓` | Navigate items in current column |
| `h/l` or `Tab` | Switch between panes (dates/tasks) |
| `Enter` or `d` | Toggle done/reopen |
| `n` | New task (inline input at bottom) |
| `e` | Edit task description (inline) |
| `f` | Create a follow-up task for tomorrow |
| `D` | Cancel task |
| `s` | Reschedule task to a different date |
| `/` | Filter by text |
| `p` | Set priority (reorder) |
| `r` | Manual reload from files |
| `?` | Help overlay |
| `q` | Quit |

### New Task Flow:
1. Press `n`
2. Inline text input appears at the bottom
3. Type: `Task description #tag para próxima seg`, `Task description #tag due tomorrow`, or `Task description #tag 📅 2026-03-01` (date optional, defaults to today)
4. Press Enter → task is written to the appropriate daily note

## File Operations

### Reading tasks:
- On startup, scan all daily notes from today - 7 days to today + 14 days
- Parse each for task lines
- Tasks without a 📅 date inherit the date of their daily note filename

### Writing tasks:
- When toggling done: change `[ ]` → `[x]` and append ` ✅ YYYY-MM-DD` (today)
- When reopening a completed task: change `[x]` or `[-]` → `[ ]` and remove ` ✅ YYYY-MM-DD` / ` ❌ YYYY-MM-DD`
- When creating: append task line under the `## :LiPencil: Open Space` heading in the target daily note
  - If daily note doesn't exist, create it with minimal frontmatter
- When creating a follow-up: append `Follow up: <original description>` to tomorrow's daily note, preserving tags and priority
- When editing: replace the exact line in the file
- When cancelling via the former delete flow: change `[ ]` → `[-]` and append ` ❌ YYYY-MM-DD` (today)
- When moving date: remove from source file, add to target file
- All writes are immediate (no save button) — the .md file is the source of truth

### Sync behavior:
- Watch the daily notes directory for markdown changes
- Reload automatically on external create/write/remove/rename events
- Keep `r` as a manual fallback if the watcher misses an event

### Daily note template (when creating new):
```markdown
---
created: YYYY-MM-DD
---

## :LiPencil: Open Space

- [ ] New task here 📅 YYYY-MM-DD

---
```

## Config File (`~/.config/obsidian-tasks/config.toml`)

```toml
[vault]
path = "/Users/thalysguimaraes/Library/Mobile Documents/iCloud~md~obsidian/Documents/Obsidian"
daily_notes_dir = "Notes/Daily Notes"
daily_note_format = "2006-01-02" # Go date format

[tasks]
section_heading = "## :LiPencil: Open Space"
lookback_days = 7
lookahead_days = 14

[theme]
# Optional color overrides
accent = "#7571F9"
overdue = "#FE5F86"
today = "#1e90ff"
upcoming = "#888888"
done = "#02BF87"
```

## Build & Install

```bash
cd ~/Projects/folio
go build -o obsidian-tasks .
# or: go install .
```

Binary name: `obsidian-tasks`
