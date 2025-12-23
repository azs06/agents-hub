# Plan: Bubble Tea TUI as Default Entry

## Goals
- Launch into a Bubble Tea TUI by default when running `agents-hub` with no subcommand.
- Keep existing CLI commands available (e.g., `agents-hub start`, `status`, etc.).
- Provide a full‑screen interactive UI inspired by `sst/opencode` for browsing status, agents, tasks, and sending messages.
- Move hub lifecycle (start/stop/health) into the TUI flow so the hub starts in‑process and can be controlled there.

## Assumptions
- We can add external dependencies (Bubble Tea + Bubbles + Lip Gloss).
- Network access will be needed to fetch Go modules.

## Implementation Steps
1) **TUI architecture + routing**
   - Create `internal/tui/` package with Bubble Tea model, views, and update loop.
   - Define screens: Dashboard, Agents, Tasks, Send Message, Logs/Errors.
   - Add keybindings inspired by `opencode` (e.g., `?` help, `tab`/`shift+tab`, `/` search, `q` quit).

2) **Hub integration**
   - Start the hub server in-process when TUI starts.
   - Use the local JSON‑RPC handler (no socket/HTTP) for TUI actions.
   - Add polling or message-driven refresh for status/agents/tasks.

3) **CLI entry changes**
   - When no subcommand is provided, launch the Bubble Tea TUI.
   - Keep existing subcommands intact.
   - Add `tui` subcommand for explicit launch.

4) **UX polish**
   - Layout with header/status bar, main content pane, and footer help.
   - Add list components for agents/tasks with detail pane.
   - Support sending messages and showing results inline.

5) **Docs**
   - Update `README.md` to mention Bubble Tea TUI as default.
   - Add keybindings and quick usage.

## Dependencies
- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles`
- `github.com/charmbracelet/lipgloss`

## Validation
- Run `go build ./cmd/agents-hub`.
- Launch `./agents-hub` and verify:
  - TUI starts, hub is running in-process.
  - Status, agents, tasks views work.
  - Sending a message returns a response.
