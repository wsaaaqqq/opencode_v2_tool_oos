# oos — OpenCode v2 Session Finder

> Find any OpenCode v2 session across all projects and resume it instantly — type keywords, Enter to continue the conversation. A Go TUI session finder for Windows + WSL.

<p align="center">
  <img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=flat&logo=go&logoColor=white" alt="Go">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT License"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%2B%20WSL-lightgrey" alt="Windows + WSL">
  <a href="https://github.com/wsaaaqqq/opencode_v2_tool_oos/releases/latest"><img src="https://img.shields.io/github/v/release/wsaaaqqq/opencode_v2_tool_oos" alt="Latest release"></a>
</p>

## Why

You discussed a bug fix weeks ago, across several projects. Instead of opening each project and scrolling through its sessions, run `oos`, search once, and press Enter to resume the right conversation.

This is the **OpenCode v2 / Windows + WSL** successor to [oos for OpenCode v1](https://github.com/wsaaaqqq/oos). The tool version starts at **v1.0.0**; “v2” refers to the OpenCode version it supports.

跨项目搜索 OpenCode **v2** 的用户提问和助手正文，回车在 Windows Terminal 新标签页继续。搜索程序运行在 WSL，Windows CMD 使用 `oos.cmd` 启动。

## Features

- **Cross-project search**: session titles, project paths, models, agents, user prompts and assistant responses.
- **Live filtering**: space-separated AND terms, `!keyword` to exclude, keyword-centered snippets.
- **Fast staged startup**: recent 100 sessions first, remaining sessions and full history loaded in the background.
- **Batch actions**: Shift+↑/↓ range selection, batch open/delete, deduplicated project-path copying.
- **Question picker**: Alt+S lists all user questions; copy a question or fork at it.
- **Live monitor**: one session with its subagents, or all top-level sessions.
- **Read-only search**: SQLite reads stay inside WSL; delete and fork use OpenCode's v2 API.
- 中文搜索、路径显示和剪贴板复制。

## Install

**Requirements:** Windows with WSL, OpenCode v2 installed in that WSL distribution, and WSL Windows interop enabled. Windows Terminal is recommended; the tool falls back to a new CMD window when Terminal is unavailable.

The first release provides a **Linux amd64** binary for x86-64 WSL, not a native Windows `.exe`. Other architectures and non-WSL desktop environments are not release targets yet. It was tested with OpenCode v2.2.7 on Debian WSL.

| Asset | Download |
|---|---|
| WSL Linux amd64 binary | [oos_linux_amd64](https://github.com/wsaaaqqq/opencode_v2_tool_oos/releases/latest/download/oos_linux_amd64) |
| Windows CMD launcher | [oos.cmd](https://github.com/wsaaaqqq/opencode_v2_tool_oos/releases/latest/download/oos.cmd) |
| SHA-256 checksums | [checksums.txt](https://github.com/wsaaaqqq/opencode_v2_tool_oos/releases/latest/download/checksums.txt) |

### 1. Install the binary inside WSL

Download `oos_linux_amd64` and `checksums.txt` into the same WSL directory, then:

```bash
grep '  oos_linux_amd64$' checksums.txt | sha256sum -c -
install -Dm755 oos_linux_amd64 "$HOME/.local/bin/oos"
"$HOME/.local/bin/oos" -v
```

### 2. Add the Windows command

Download `oos.cmd` and place it in a Windows directory on your `PATH`.

The supplied launcher uses a root WSL account by default:

```bat
@echo off
chcp 65001 >nul
wsl.exe -e /root/.local/bin/oos %*
```

**If your WSL user is not root**, change `/root/.local/bin/oos` to the absolute path printed by `echo "$HOME/.local/bin/oos"` in WSL. The launcher and OpenCode must run in the **same WSL distribution and user account**. For a non-default distribution/user, add `-d <distribution> -u <user>` to `wsl.exe`.

If the v1 command is still installed, rename that `oos.exe` to `ooss.exe` so the new `oos.cmd` is not shadowed. The two tools use their own OpenCode environments; this project does not migrate v1 databases.

### 3. Search from Windows CMD

```bat
oos
oos java spring
oos bug fix !plan
oos --print keyword
oos -m
oos -m ses_xxxxxxxx
oos -v
```

When opening sessions in Windows-mounted project directories, the tool reuses `oo` if that command is available, preserving your existing OpenCode launcher setup. Otherwise it starts the WSL OpenCode executable directly. The new tab stays open after OpenCode exits.

## Keyboard Shortcuts

| Key | Action |
|---|---|
| Type keywords | Real-time filtering; space-separated AND terms, `!term` excludes |
| `↑` / `↓` | Move the focused row |
| `Shift+↑` / `Shift+↓` | 连续多选；以当前行为起点扩选，反向移动缩小选区 |
| `Enter` | Open selected sessions in new tabs; more than 5 tabs requires a second Enter |
| `Alt+S` | List user questions in the focused session |
| `Ctrl+W` | Remove the last search term |
| `Alt+Q` | Copy project paths; deduplicated, one per line when multiple sessions are selected |
| `Alt+M` | Monitor the focused session; no focused session means global monitor |
| `Ctrl+D` | Delete selected sessions, including child sessions; press twice to confirm |
| `Esc` / `Ctrl+C` | Quit |

Selected rows show `[✓]` and the footer shows the selection count. Ordinary navigation returns to single selection; changing the search query clears the selection. There are no additional select-all or non-contiguous selection bindings. `Esc` still quits immediately.

Batch open/delete runs sequentially with progress and success/failure counts. Failed items remain selected for retry. Other keys cancel a pending confirmation. Question picking, forking and monitoring stay focused on a single session.

## Search Scope

Keywords match session metadata (including session ID, title, slug, WSL/Windows directory path, model and agent) and the full user/assistant text history. Archived sessions and subagent sessions are hidden from the search results by default. Reasoning, tool output, system instructions and synthetic reminders are not indexed.

The search status changes from `last 100 session loaded` to `all session loaded` when loading finishes. Typing works throughout; additional matches may arrive in the background.

## User Question Picker

Select a session and press `Alt+S`. Filter its complete user-question list, then press Enter for Fork/Copy actions.

- **Fork** includes the selected question and opens the new session in a new tab. The question text is copied to the clipboard; it is **not auto-submitted** or inserted into the new TUI input.
- **Copy** copies the question text.
- `Ctrl+D` inside this picker immediately deletes **that single session**, as in the original tool. `Esc` goes back.

## Monitor

```bat
oos -m ses_xxxxxxxx
oos -m
```

Refreshes every 2 seconds. Single-session mode includes its subagent sessions; global mode shows recent messages across top-level sessions. Use ↑/↓ or PgUp/PgDn to scroll; `q`, Esc or Ctrl+C quits.

## How It Works

Reads `~/.local/share/opencode/opencode.db` from **inside WSL**, using the v2 `session_v2` and `session_message` tables. `OPENCODE_DB`, `XDG_DATA_HOME` and `--db <path>` allow alternate database locations. All searching happens in memory after loading.

| Stage | Data |
|---|---|
| Startup | Most recent 100 top-level, non-archived sessions + first user prompt |
| Background sessions | Remaining matching sessions |
| History index | User prompts and assistant text responses |
| Per-keystroke filtering | In-memory keyword matching |

The database schema is version-dependent. Unsupported future schemas may require an update. Search never writes to the OpenCode database; destructive operations are routed through OpenCode.

## Build & Test

Requires **Go 1.25+**. In WSL:

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o oos-linux .
install -Dm755 oos-linux "$HOME/.local/bin/oos"
```

Selection/batch tests use synthetic data and stub workers. Optional database integration tests are read-only:

```bash
OOS_TEST_DB="$HOME/.local/share/opencode/opencode.db" go test ./...
```

Automatic upgrading is not implemented yet; download a newer release or rebuild to update.

## Tech Stack

| Component | Purpose |
|---|---|
| Go 1.25+ | Language |
| Bubble Tea / Bubbles / Lip Gloss | Terminal interface |
| modernc.org/sqlite | Pure-Go SQLite, no CGO |
| OpenCode v2 API | Session deletion and forking |

## Related

- [oos for OpenCode v1](https://github.com/wsaaaqqq/oos) — original project; this successor retains its MIT license and search workflow.
- [OpenCode](https://opencode.ai)
- [Releases](https://github.com/wsaaaqqq/opencode_v2_tool_oos/releases)

## License

[MIT](LICENSE) © 2026 wsaaaqqq
