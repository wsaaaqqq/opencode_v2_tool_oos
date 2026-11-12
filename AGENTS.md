# oos v2 — OpenCode v2 session finder

Single-binary Go TUI that searches every OpenCode **v2** session and resumes it
in a new Windows Terminal tab. The binary runs inside WSL (Debian) and is
started from Windows CMD by `oos.cmd`.

## Commands

```bash
# Go 1.25+ inside WSL
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o oos-linux .

# install
install -Dm755 oos-linux "$HOME/.local/bin/oos"
# Put oos.cmd on the Windows PATH; adapt its WSL user/path if not using root.

# quick checks without the TUI
./oos-linux --print keyword
./oos-linux -v
```

TUI Update/selection/batch tests use synthetic sessions and stubbed workers.
Verification path: `go build` + `go vet` +
`./oos-linux --print <keyword>` + the Go tests:

```bash
OOS_TEST_DB="$HOME/.local/share/opencode/opencode.db" go test -v ./...
# optional clipboard round-trip (overwrites the Windows clipboard)
OOS_TEST_CLIPBOARD=1 go test -run TestClipboard -v
# optional fork + delete round-trip (creates and removes a real fork)
OOS_TEST_DB="$HOME/.local/share/opencode/opencode.db" OOS_TEST_FORK=1 go test -run TestForkSessionRoundTrip -v
```

Tests only read the database. Delete/fork tests must use a throwaway session or
fork and delete it immediately.

## Architecture

```
session.go          — read-only SQLite: session_v2 + session_message
filter.go           — keyword parsing + in-memory matching
tui.go              — bubbletea TUI: search, results, user-question picker
selection.go        — Shift+arrow range selection by session ID, action targets
batch.go            — confirmed sequential open/delete jobs and retry state
monitor.go          — live message monitor (2s refresh)
opencode_api.go     — v2 HTTP API: delete, fork, service discovery
open_session_linux.go — WSL→Windows interop: wt.exe / cmd.exe / clip.exe
paths.go            — /mnt/c ↔ C:\ path conversion
main.go             — flags, entrypoint
```

All files are `package main`. The binary is **Linux**; Windows only provides
`oos.cmd`, which runs `wsl.exe -e /root/.local/bin/oos %*`.

## Database details

Path: `~/.local/share/opencode/opencode.db` (respects `OPENCODE_DB`).
Opened read-only with `PRAGMA query_only=ON` and one connection.

V2 tables (the V1 `session` / `message` / `part` tables no longer exist):

- `session_v2` — id, project_id, parent_id, fork_session_id, fork_boundary,
  slug, directory, title, model (JSON), agent, time_created, time_updated,
  time_archived, tokens_*, cost
- `session_message` — id, session_id, type, seq, time_created, data (JSON)
- `session_part` exists but is empty in current V2 databases; message content
  lives inside `session_message.data`.

Message types and text locations:

| type | text |
|---|---|
| `user` | `data.text` |
| `assistant` | `data.content[]` where item `type == "text"` → `item.text` |
| `synthetic` | `data.text` — system reminders, excluded from search |
| `system` | `data.text` — instructions, excluded |
| `compaction` | `data.summary` — excluded |

Search indexes **user prompts + assistant text only**. Tool output, reasoning,
system instructions and synthetic reminders are intentionally skipped. The
assistant query prefilters with `instr(data, '"type":"text"') > 0`; only then
does Go decode content. Do not decode every assistant message, and do not add
reasoning/tool payloads to the in-memory index.

`assistantContentItem.UnmarshalJSON` reads `type` first and only retains `text`
for text items. This keeps reasoning/tool payloads out of memory.

## Session visibility

Top-level, non-archived sessions only:

```sql
WHERE time_archived IS NULL
  AND (parent_id IS NULL OR parent_id = '')
```

Subagent sessions have `parent_id`; archived sessions have `time_archived`.
Forked sessions are normal top-level sessions and are shown.

## OpenCode v2 API

Service discovery: `~/.local/state/opencode/service.json` (url + password).
Basic auth username is always `opencode` (V2 no longer supports
`OPENCODE_SERVER_USERNAME`). `ensureService` pings `/api/info`, then runs
`opencode service start` and polls if needed.

- Delete: `DELETE /api/session/{id}` (removes child sessions too). Falls back to
  `opencode session delete <id>`.
- Fork: `POST /api/session/{id}/fork` body `{"before": messageID|null}`.
  `before` excludes the boundary message. To include the selected question,
  pass the ID of the **next** message after it. If the question is last, pass
  `null` (fork everything, which ends at that question).

V2 has no `/tui/append-prompt` equivalent. The fork picker therefore copies the
question to the clipboard after forking; it does not restore it into the new
TUI input. Do not auto-submit the question.

## Windows interop

- Window/tab: `wt.exe -w 0 nt -d <C:\dir> cmd.exe /k oo -s <id>` for `/mnt/*`
  project directories. This reuses the user's `oo.cmd` (UTF-8, proxy, sing-box)
  and keeps the tab open after OpenCode exits.
- Non-`/mnt` dirs: `wt.exe -w 0 nt cmd.exe /k wsl.exe --cd <dir> -e <opencode> -s <id>`.
- `wt.exe` is located via `%LOCALAPPDATA%\Microsoft\WindowsApps\wt.exe`.
- Fallback when `wt.exe` is missing: `cmd.exe /c start "" cmd.exe /k ...`.
- Clipboard: `clip.exe` fed UTF-16LE (no BOM; a BOM becomes a leading
  zero-width character when pasted). `atotto/clipboard` does not work in a WSL
  console (no X11).

## Display quirks

### Main-result multi-selection

- Only Shift+Up/Down extends/shrinks the inclusive range from a stable anchor
  session ID. Plain navigation resets to single selection; query edits clear
  marks. No Ctrl+A/Ctrl+Space selection bindings; Esc still quits directly.
- Marks and focus use session IDs, never stale result indices. Startup loads
  must preserve these identities and must not automatically mark new matches.
- Enter opens marked sessions (over 5 tabs needs Enter again); Ctrl+D confirms
  the exact target ID set before sequential deletion, including child sessions.
  Other keys cancel confirmation. Alt+Q copies deduplicated project paths.
- Each worker returns a batchItemDoneMsg through Update; ignore other action
  keys while busy, show progress, remove successful targets, retain failures
  for retry. Tombstones prevent late startup loads from restoring deleted rows.
- Alt+S/Fork and Alt+M retain their single-focused-session behavior. Question
  picker shortcuts are unchanged. selection_test.go uses synthetic sessions
  and stubbed worker results: never execute real delete/open commands in it.

Column widths are hardcoded in `tui.go`: `colTitle=24`, `colMsg=56`,
`colTime=11`. Separator is ` │ ` (3 cols).

`displayWidth()` counts runes > 127 as 2 columns (CJK). It must match terminal
rendering or columns misalign.

`highlightMatches()` runs **after** `truncateCols()`; ANSI codes must never be
counted as display width.

`formatDir()` receives `toWindowsPath(s.Directory)` so WSL paths display as
`C:\...`. Search still matches both the raw WSL path and the Windows form.

## Staged startup

1. `LoadSessions(db, 100)` — recent sessions + first user text; paints fast.
2. `LoadSessionsExcluding(db, ids)` — remaining sessions stream in.
3. `LoadAllMessages` — full text index, one sequential scan.

The search tag shows `last 100 session loaded` until stages 1–3 finish, then
`all session loaded`. Typing is never blocked; results grow as data arrives.

`LoadAllMessages` is deliberately one unfiltered scan. Do not turn it into
per-session `IN (...)` batches; V1 measured index seeks at ~34s cold vs ~13s
for the sequential scan on a 3GB database.

## Deployment

- Linux binary: `$HOME/.local/bin/oos` inside the same WSL account as OpenCode.
- Windows wrapper: `oos.cmd` in a Windows PATH directory. The supplied wrapper
  defaults to `/root/.local/bin/oos`; edit it for other users/distributions.
- To coexist with the original V1 tool, rename its `oos.exe` to `ooss.exe`.

## Release

- Repository: `wsaaaqqq/opencode_v2_tool_oos`, MIT, first version `v1.0.0`.
- Do not tag, push or publish unless the user explicitly asks.
- CI runs build, vet and synthetic-data tests on main/PRs.
- Pushing an annotated `v*` tag triggers release.yml: tests, Linux amd64 build,
  CRLF Windows launcher and SHA-256 checksums. Release notes come from the tag.
- Never include databases, credentials, session dumps or local build artifacts
  in commits. Release binaries belong in Release assets, not Git history.

## Gotchas

- Do not build with GOOS unset. The Windows Go toolchain defaults to
  `windows/amd64`; `WSLENV` or `cmd.exe /c set GOOS=linux` is required. A PE
  binary will otherwise be produced and `os.Stat("/root/...")` fails.
- The TUI is a Linux binary started through `wsl.exe`; run it from a real
  terminal (Windows Terminal or conhost), not a non-interactive pipe.
- Do not write to the database. All mutations go through the v2 API.
- Delete/fork tests must not touch real sessions; create a throwaway session or
  fork and delete it immediately.
