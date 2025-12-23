# TUI Improvement Plan

## Summary (Best-Practice Patterns)
- Use list + detail panes for agents/tasks to improve scanability.
- Add persistent status and help bars for context and keybindings.
- Provide command palette and search for fast navigation.
- Use spinners and inline banners for async feedback.
- Show response history and full task detail views.
- Expose runtime config (orchestrator agents, exec paths, transports) in UI.

## Step 1: Layout + Navigation
- Replace simple tabs with a two-pane layout:
  - Left pane: list (Agents/Tasks/Contexts) with selection.
  - Right pane: detail view for selected item.
- Add top status bar (hub state + last refresh) and bottom help bar.
- Keep a lightweight tab selector for switching list content.

## Step 2: Interaction Model
- Implement keymap + help overlay (`?`).
- Add command palette (`/` or `:`) for actions.
- Add search/filter for lists.

## Step 3: Data UX
- Use `bubbles/list` for Agents/Tasks.
- Use `bubbles/viewport` for detailed text.
- Add response history list + drilldown.

## Step 4: Progress + Feedback
- Add spinners for refresh/send.
- Show task state transitions in real time.
- Add error banner + log panel.

## Step 5: Send Workflow
- Use `textarea` for multi-line input.
- Display last response and a link to detailed view.

## Step 6: Config + Orchestration
- Add settings screen (delegate list, exec paths, transports, data dir).
- Allow runtime editing of orchestrator delegates.

## Step 7: Polish
- Harmonize spacing and styles.
- Responsive layout for narrow terminals.
- Confirm quit if send is in progress.
