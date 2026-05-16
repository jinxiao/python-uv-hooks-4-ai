# uv Python Agent Hooks

Go hook-only integration that nudges Python-related commands in AI coding
agents toward `uv`.

## Status

Tested:

- Windows + Codex CLI.

Implemented but not yet verified:

- OpenCode hook installation and command rewriting.
- macOS and Linux behavior.

This repository should be treated as tested only for the Windows + Codex CLI
path until the other targets are validated.

## Prerequisites

Install these tools and make sure they are available on `PATH`:

- Go 1.22 or newer.
- `uv`.
- `codex` for the tested Codex CLI path.

The hook does not install Codex, OpenCode, Python, Go, or `uv`.

## Build

From the repository root:

```powershell
go build -buildvcs=false -o .\bin\uv-python-hook.exe .\cmd\uv-python-hook
```

To install the command into your Go binary directory instead:

```powershell
go install -buildvcs=false .\cmd\uv-python-hook
```

`-buildvcs=false` avoids Go VCS stamping failures in checkouts where Git marks
the directory as unsafe or unavailable.

After building or installing, confirm the command is reachable:

```powershell
uv-python-hook doctor
```

If you built into `.\bin`, either add that directory to `PATH` or run the binary
by path:

```powershell
.\bin\uv-python-hook.exe doctor
```

## Test

Run the full Go test suite:

```powershell
go test ./...
```

If your Go cache is not writable in the current environment, point it to a local
cache directory:

```powershell
$env:GOCACHE = "$PWD\.tmp-go-cache"
go test ./...
```

You can also test a rewrite without installing hooks:

```powershell
uv-python-hook rewrite-command "python app.py"
uv-python-hook rewrite-command "pip install -r requirements.txt"
uv-python-hook detect-project
```

## Usage With Codex CLI

Install the tested Codex hook for the current Windows user:

```powershell
uv-python-hook install --user --targets codex
```

For a project-local install, run this inside the target repository:

```powershell
uv-python-hook install --project --targets codex
```

User installs write Codex hook entries to:

```text
%USERPROFILE%\.codex\hooks.json
```

Project installs write Codex hook entries to:

```text
.\.codex\hooks.json
```

The Codex hook runs before shell tool execution. When it sees a Python-related
command, it denies the original command and suggests the equivalent `uv`
command.

Examples:

- `python app.py` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache run python app.py`
- `py app.py` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache run python app.py`
- `pytest -q` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache run pytest -q`
- `pip install requests` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install requests`
- `python -m pip install requests` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install requests`
- `python -m venv` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache venv .venv`

For requirements installs, the hook checks the nearest `pyproject.toml`:

- If the project has uv-syncable dependency metadata, `pip install -r requirements.txt` becomes `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache sync`.
- If `[project]` metadata is incomplete, it falls back to `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install -r requirements.txt`.

Run `doctor` after installation to inspect tool availability and hook paths:

```powershell
uv-python-hook doctor
```

Uninstall the Codex hook:

```powershell
uv-python-hook uninstall --user --targets codex
```

For a project-local install:

```powershell
uv-python-hook uninstall --project --targets codex
```

## Cache Directory

Hooks force `UV_CACHE_DIR` to a dedicated temporary cache:

```text
%TEMP%\uv-python-agent-hooks\uv-cache
```

Override it with:

```powershell
$env:UV_PYTHON_AGENT_HOOKS_CACHE_DIR = "D:\path\to\uv-cache"
```

## Commands

```powershell
uv-python-hook install --user --targets codex
uv-python-hook uninstall --user --targets codex
uv-python-hook doctor
uv-python-hook detect-project
uv-python-hook rewrite-command "pip install -r requirements.txt"
```
