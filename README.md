# Obsidian Tasks TUI

A terminal UI for managing [Obsidian Tasks](https://publish.obsidian.md/tasks/Introduction) directly from your daily notes — inspired by [Things 3](https://culturedcode.com/things/).

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)

```
╭──────────────╮╭─────────────────────────────────────────────╮
│              ││                                             │
│  ☀ Today  3  ││  Today · Feb 28                             │
│  📅 Upcoming ││                                             │
│  📓 Logbook  ││  ○ Fix login bug                   #work   │
│              ││  ○ Call dentist                #personal    │
│              ││  ● Send invoice          #work  ✓ today    │
│              ││                                             │
│              ││  ── Overdue ─────────────────────────────── │
│              ││  ○ Review PR from Feb 26          #work    │
│              ││                                             │
╰──────────────╯╰─────────────────────────────────────────────╯
 n new  d done  f follow-up  e edit  / filter  ? help  q quit
```

## Features

- **Three views** — Today (due today + overdue), Upcoming (future tasks by date), Logbook (closed tasks)
- **Sidebar navigation** — switch views with `1` `2` `3` or `j`/`k`
- **Obsidian Tasks compatible** — reads `- [ ]` / `- [x]` syntax with `📅` due dates and `✅` completion dates
- **Section-scoped parsing** — only reads tasks from your configured section heading (e.g. `## Open Space`)
- **Tag filtering** — mirrors Obsidian Tasks queries: requires tags, excludes `#habit` by default
- **Create, edit, block, cancel, toggle** — changes are written back to the daily note files
- **Follow-up shortcut** — press `f` on a task to create `Follow up: ...` in tomorrow's daily note before closing the current one
- **Auto-sync** — watches the daily notes folder and reloads when markdown files change externally
- **Tag-based colors** — consistent color per tag across the UI

## Install

```bash
go install github.com/thalysguimaraes/obsidian-tasks-tui@latest
```

Or build from source:

```bash
git clone https://github.com/thalysguimaraes/obsidian-tasks-tui
cd obsidian-tasks-tui
go build
```

## Configuration

Create `~/.config/obsidian-tasks/config.toml`:

```toml
[vault]
path = "/path/to/your/obsidian/vault"
daily_notes_dir = "Notes/Daily Notes"
daily_note_format = "2006-01-02"

[tasks]
section_heading = "## Open Space"
logbook_days = 30
lookahead_days = 14
exclude_tags = ["#habit"]

[theme]
accent = "#7571F9"
overdue = "#FE5F86"
today = "#1e90ff"
upcoming = "#888888"
done = "#02BF87"
muted = "#555555"
```

The only required field is `vault.path`. Everything else has sensible defaults.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Move up / down |
| `h` / `l` | Sidebar / Content |
| `Tab` | Toggle focus |
| `1` `2` `3` | Today / Upcoming / Logbook |
| `Enter` | Select view or toggle done |
| `n` | New task |
| `e` | Edit task |
| `d` | Toggle done / reopen |
| `b` | Toggle blocked |
| `f` | Create follow-up for tomorrow |
| `t` | Toggle priority separators |
| `D` | Cancel task |
| `/` | Filter by text |
| `Esc` | Clear filter |
| `r` | Reload from files |
| `?` | Help |
| `q` | Quit |

## Task format

Tasks follow the [Obsidian Tasks](https://publish.obsidian.md/tasks/Introduction) format:

```markdown
- [ ] Task description #tag 📅 2026-03-01
- [b] Blocked task #tag 📅 2026-03-01
- [x] Completed task #tag 📅 2026-02-28 ✅ 2026-02-28
- [-] Cancelled task #tag 📅 2026-02-28 ❌ 2026-02-28
```

New tasks created via the TUI are written into the daily note file under the configured section heading. Creation accepts explicit due-date markers such as `📅 amanhã`, `para próxima seg`, `em 2 semanas` after `📅`, `due next mon`, or `📅 2026-03-01`.

Follow-ups use the same write path, preserving the current task's tags and priority, and schedule the new task for the next local day.

Cancelling a task keeps it in the markdown file using the Obsidian Tasks cancelled status, so it drops out of the active lists but remains queryable in past notes and the logbook.

Blocked tasks are stored as the custom checkbox status `[b]`. The TUI treats them as active tasks with distinct styling. If you want Obsidian Tasks queries to understand them semantically too, add a custom status for `b` in Obsidian Tasks settings and map it to an `ON_HOLD`-style status.

## Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling
- [Bubbles](https://github.com/charmbracelet/bubbles) — text input component

## License

MIT
