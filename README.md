# Obsidian Tasks TUI

A terminal UI for managing [Obsidian Tasks](https://publish.obsidian.md/tasks/Introduction) directly from your daily notes — inspired by [Things 3](https://culturedcode.com/things/).

```
+--------------++---------------------------------------------+
|              ||                                             |
|  * Today  3  ||  Today . Feb 28                             |
|  > Upcoming  ||                                             |
|  > Logbook   ||  o Fix login bug                   #work   |
|              ||  o Call dentist                #personal    |
|              ||  * Send invoice          #work  + today    |
|              ||                                             |
|              ||  -- Overdue -------------------------------- |
|              ||  o Review PR from Feb 26          #work    |
|              ||                                             |
+--------------++---------------------------------------------+
 n new  d done  f follow-up  e edit  / filter  ? help  q quit
```

## Features

- **Three views** — Today (due today + overdue), Upcoming (future tasks by date), Logbook (closed tasks)
- **Obsidian Tasks compatible** — reads `- [ ]` / `- [x]` syntax with date markers
- **Section-scoped parsing** — only reads tasks from a configured section heading
- **Tag filtering** — filter by tag, excludes `#habit` by default
- **Create, edit, block, cancel, toggle** — changes are written back to daily note files
- **Follow-up shortcut** — press `f` to create a follow-up in tomorrow's daily note
- **Auto-sync** — watches the daily notes folder and reloads on external changes
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
| `1` `2` `3` | Today / Upcoming / Logbook |
| `n` | New task |
| `e` | Edit task |
| `d` | Toggle done / reopen |
| `b` | Toggle blocked |
| `f` | Create follow-up for tomorrow |
| `D` | Cancel task |
| `/` | Filter by text |
| `r` | Reload from files |
| `q` | Quit |

## Task format

```markdown
- [ ] Task description #tag 📅 2026-03-01
- [b] Blocked task #tag 📅 2026-03-01
- [x] Completed task #tag 📅 2026-02-28 ✅ 2026-02-28
- [-] Cancelled task #tag 📅 2026-02-28 ❌ 2026-02-28
```

Due dates accept natural language: `📅 amanha`, `due next mon`, `em 2 semanas`, or explicit `📅 2026-03-01`.

## Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling
- [Bubbles](https://github.com/charmbracelet/bubbles) — text input component

## License

MIT
