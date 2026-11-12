## v1.0.0 — Initial release

OpenCode v2 Session Finder for Windows + WSL, based on the original [oos](https://github.com/wsaaaqqq/oos) workflow.

### Features

- Search across all projects: session metadata, user prompts and assistant responses.
- Live AND/exclusion keyword filtering, Chinese text and Windows/WSL project paths.
- Recent sessions first; remaining sessions and full history load in the background.
- Shift+Up/Down range selection with batch open, confirmed delete and deduplicated path copying.
- User-question picker with Fork/Copy actions; 2-second live monitor.
- New Windows Terminal tabs; session changes use OpenCode v2 APIs, not direct database writes.

### Downloads

- `oos_linux_amd64`: Linux amd64 binary to install **inside WSL**.
- `oos.cmd`: Windows CMD launcher (defaults to `/root/.local/bin/oos`; adapt the path/distribution/user if needed).
- `checksums.txt`: SHA-256 checksums for the downloadable files.

This is version **1.0.0 of the new tool**; it targets **OpenCode v2**, not OpenCode v1. Tested with OpenCode v2.2.7 on Debian WSL. See the [README](https://github.com/wsaaaqqq/opencode_v2_tool_oos#install) for setup and shortcut details.
