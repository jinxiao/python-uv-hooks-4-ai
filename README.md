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

The rewrite rules intentionally follow uv project boundaries:

- `pyproject.toml` and `uv.lock` mark uv-managed projects.
- `poetry.lock`, `[tool.poetry]`, `pdm.lock`, and `[tool.pdm]` mark
  projects managed by other tools; commands in those projects are left
  unchanged.
- Python tool commands such as `ruff`, `pytest`, and `ty` use `uv run` inside
  uv projects, and `uvx`/`uv tool run` outside projects.

## Build

From the repository root.

For release-style builds, use `CGO_ENABLED=0`. This project is a pure Go CLI
with no C dependencies, so disabling CGO makes the binary easier to distribute
across machines without requiring a matching system C runtime. It is not
strictly required for local development, but it is the recommended default for
published binaries. If future dependencies require CGO, remove this setting.

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force .\bin | Out-Null
$env:CGO_ENABLED = "0"
go build -buildvcs=false -trimpath -ldflags="-s -w" -o .\bin\uv-python-hook.exe .\cmd\uv-python-hook
```

Linux or macOS:

```sh
mkdir -p ./bin
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o ./bin/uv-python-hook ./cmd/uv-python-hook
```

Common cross-compile targets from PowerShell:

```powershell
New-Item -ItemType Directory -Force .\bin | Out-Null
$env:CGO_ENABLED = "0"

$env:GOOS = "linux";  $env:GOARCH = "amd64"
go build -buildvcs=false -trimpath -ldflags="-s -w" -o .\bin\uv-python-hook-linux-amd64 .\cmd\uv-python-hook

$env:GOOS = "linux";  $env:GOARCH = "arm64"
go build -buildvcs=false -trimpath -ldflags="-s -w" -o .\bin\uv-python-hook-linux-arm64 .\cmd\uv-python-hook

$env:GOOS = "darwin"; $env:GOARCH = "amd64"
go build -buildvcs=false -trimpath -ldflags="-s -w" -o .\bin\uv-python-hook-darwin-amd64 .\cmd\uv-python-hook

$env:GOOS = "darwin"; $env:GOARCH = "arm64"
go build -buildvcs=false -trimpath -ldflags="-s -w" -o .\bin\uv-python-hook-darwin-arm64 .\cmd\uv-python-hook

Remove-Item Env:\GOOS, Env:\GOARCH
```

Common cross-compile targets from Linux or macOS shells:

```sh
mkdir -p ./bin
CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags="-s -w" -o ./bin/uv-python-hook-linux-amd64  ./cmd/uv-python-hook
CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags="-s -w" -o ./bin/uv-python-hook-linux-arm64  ./cmd/uv-python-hook
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags="-s -w" -o ./bin/uv-python-hook-darwin-amd64 ./cmd/uv-python-hook
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags="-s -w" -o ./bin/uv-python-hook-darwin-arm64 ./cmd/uv-python-hook
```

To install the command into your Go binary directory instead:

```powershell
$env:CGO_ENABLED = "0"
go install -buildvcs=false .\cmd\uv-python-hook
```

`-buildvcs=false` avoids Go VCS stamping failures in checkouts where Git marks
the directory as unsafe or unavailable.

After building or installing, confirm the command is reachable:

```powershell
uv-python-hook doctor
uv-python-hook --version
```

If you built into `.\bin`, either add that directory to `PATH` or run the binary
by path:

```powershell
.\bin\uv-python-hook.exe doctor
```

## Release

Releases are automated with GoReleaser and GitHub Actions.

- Pushes to `main` create a stable SemVer tag such as `v1.2.3`, then publish a
  GitHub release.
- Pushes to `dev` create a prerelease tag such as `v1.2.3-beta.1`, then publish
  a GitHub prerelease.
- Version bumps follow Conventional Commits:
  - `feat:` increments minor.
  - `fix:` and other changes increment patch.
  - `BREAKING CHANGE:` or a `!` marker, such as `feat!:` increments major.

GoReleaser builds Linux, Windows, and macOS binaries for `amd64` and `arm64`.
The release build injects the release version into the binary, so:

```powershell
uv-python-hook --version
```

prints the released version and build metadata.

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
uv-python-hook rewrite-command --target opencode "python app.py"
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
command. Codex suggestions force a temporary uv cache by default, so standalone
tools use `uv tool run` instead of the `uvx` shorthand.

Examples:

- `python app.py` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache run app.py`
- `py app.py` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache run app.py`
- `pytest -q` inside a uv project -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache run pytest -q`
- `pytest -q` outside a project -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache tool run pytest -q`
- `pip install requests` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install requests`
- `python -m pip install requests` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install requests`
- `python -m venv` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache venv .venv`

For requirements installs, the hook checks the nearest `pyproject.toml` or
`uv.lock`:

- If the project has uv-syncable dependency metadata, `pip install -r requirements.txt` becomes `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache sync`.
- If `[project]` metadata is incomplete, it falls back to `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install -r requirements.txt`.
- If the project is managed by Poetry or PDM, the command is left unchanged.

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

## Usage With OpenCode

Install the OpenCode plugin:

```powershell
uv-python-hook install --user --targets opencode
```

For sandboxed OpenCode environments, prefer a project-local install so the
plugin is available from the working directory:

```powershell
uv-python-hook install --project --targets opencode
```

OpenCode rewrites Python-related commands directly. Unlike the Codex target, it
does not force `uv` to use the hook's temporary cache by default, so standalone
tools use the `uvx` shorthand:

- `python app.py` -> `uv run app.py`
- `pytest -q` inside a uv project -> `uv run pytest -q`
- `pytest -q` outside a project -> `uvx pytest -q`
- `pip install requests` -> `uv pip install requests`

Set `UV_PYTHON_AGENT_HOOKS_CACHE_MODE=on` to force the same temporary cache mode
used by Codex.

## Cache Directory

Codex hooks force `UV_CACHE_DIR` to a dedicated temporary cache by default:

```text
%TEMP%\uv-python-agent-hooks\uv-cache
```

OpenCode hooks leave `uv` cache behavior unchanged by default. Control this
with:

```powershell
$env:UV_PYTHON_AGENT_HOOKS_CACHE_MODE = "auto" # codex on, opencode off
$env:UV_PYTHON_AGENT_HOOKS_CACHE_MODE = "on"   # force temp cache
$env:UV_PYTHON_AGENT_HOOKS_CACHE_MODE = "off"  # never force temp cache
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
uv-python-hook rewrite-command --target opencode "python app.py"
```
