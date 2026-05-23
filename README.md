# uv Python Agent Hooks

Go hook-only integration that nudges Python-related commands in AI coding
agents toward `uv`.

## Status

Tested:

- Windows + Codex CLI.
- Linux binary installer smoke test.

Implemented but not yet verified:

- OpenCode hook installation and command rewriting.
- macOS installer behavior.
- macOS and Linux Codex/OpenCode agent behavior.

This repository should be treated as tested only for the Windows + Codex CLI
path and the Linux binary installer path until the other targets are validated.

## Prerequisites

Install these tools and make sure they are available on `PATH`:

- `uv`.
- `codex` for the tested Codex CLI path.

Go 1.22 or newer is only required when building from source.

The hook installer does not install Codex, OpenCode, Python, Go, or `uv`.

The rewrite rules intentionally follow uv project boundaries:

- `pyproject.toml` and `uv.lock` mark uv-managed projects.
- `poetry.lock`, `[tool.poetry]`, `pdm.lock`, and `[tool.pdm]` mark
  projects managed by other tools; commands in those projects are left
  unchanged.
- Python tool commands such as `ruff`, `pytest`, and `ty` use `uv run` inside
  uv projects, and `uvx`/`uv tool run` outside projects.

## Install Binary

Install the latest stable GitHub release into a user-level binary directory.

Linux:

```sh
curl -LsSf https://uv-python-hook.jinxiao2010.uk/install.sh | sh
```

macOS:

```sh
curl -LsSf https://uv-python-hook.jinxiao2010.uk/install.sh | sh
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://uv-python-hook.jinxiao2010.uk/install.ps1 | iex"
```

The installer fetches the latest stable release from GitHub, downloads the
matching archive for your OS and CPU architecture, verifies it against
`checksums.txt`, installs the binary, and then runs `uv-python-hook install` to
configure detected Codex/OpenCode hooks.

Default install locations:

- Linux: `$HOME/.local/bin/uv-python-hook`
- macOS: `$HOME/.local/bin/uv-python-hook`
- Windows: `$HOME\.local\bin\uv-python-hook.exe`

The installer also tries to make the command available on `PATH`:

- macOS: appends a small `uv-python-hook` block to the detected shell profile
  (`~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`).
- Linux: appends a small `uv-python-hook` block to the detected shell profile
  (`~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`).
- Windows: appends the install directory to the user `Path` environment
  variable.

Installer environment variables:

- `UV_PYTHON_HOOK_INSTALL_DIR`: install to a custom directory.
- `UV_PYTHON_HOOK_NO_MODIFY_PATH=1`: install the binary without editing PATH.
- `UV_PYTHON_HOOK_NO_INSTALL_HOOKS=1`: install the binary without running
  `uv-python-hook install`.
- `UV_PYTHON_HOOK_REPO`: override the GitHub repository, useful for forks and
  release testing.

When using the piped Unix installer, pass skip variables to `sh`:

```sh
curl -LsSf https://uv-python-hook.jinxiao2010.uk/install.sh | UV_PYTHON_HOOK_NO_INSTALL_HOOKS=1 sh
```

To remove the binary:

```sh
rm ~/.local/bin/uv-python-hook
```

```powershell
Remove-Item "$HOME\.local\bin\uv-python-hook.exe"
```

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
It also uploads `install.sh` and `install.ps1` as release assets. The release
build injects the release version into the binary, so:

```powershell
uv-python-hook --version
```

prints the released version and build metadata.

## Test

Run the full Go test suite:

```powershell
go test ./...
```

On Linux, also run the installer smoke test:

```sh
sh -n scripts/install.sh
sh -n scripts/test-install-sh.sh
sh scripts/test-install-sh.sh
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

By default, `install` uses user scope and auto-detects installed targets. It
installs the Codex hook only when `codex` is found on `PATH`, and installs the
OpenCode plugin only when `opencode` is found on `PATH`:

```powershell
uv-python-hook install
```

The release installers run this command automatically after installing the
binary unless `UV_PYTHON_HOOK_NO_INSTALL_HOOKS=1` is set.

To force a specific target for the current user:

```powershell
uv-python-hook install --user --targets codex
```

For a project-local install, run this inside the target repository and specify
`--project`:

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
- `python -m venv venv` -> `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache venv venv`

For requirements installs, the hook checks the nearest `pyproject.toml` or
`uv.lock`:

- If the project has uv-syncable dependency metadata, `pip install -r requirements.txt` becomes `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache sync`.
- If `[project]` metadata is incomplete, it falls back to `uv --cache-dir <temp>\uv-python-agent-hooks\uv-cache pip install -r requirements.txt`.
- If the project is managed by Poetry or PDM, the command is left unchanged.

Run `doctor` after installation to inspect tool availability and hook paths:

```powershell
uv-python-hook doctor
```

By default, `uninstall` also uses user scope and auto-detects targets. It
removes existing hook files even if the corresponding command is no longer on
`PATH`:

```powershell
uv-python-hook uninstall
```

To force a specific target:

```powershell
uv-python-hook uninstall --user --targets codex
```

For a project-local install:

```powershell
uv-python-hook uninstall --project --targets codex
```

## Usage With OpenCode

Install the OpenCode plugin explicitly:

```powershell
uv-python-hook install --user --targets opencode
```

Use the explicit command when `uv-python-hook install` did not auto-detect
OpenCode, for example when the `opencode` command is not on `PATH` in the
installing shell.

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

## Virtual Environment Directory

By default, venv rewrites preserve an explicit path:

```text
python -m venv      -> uv venv .venv
python -m venv venv -> uv venv venv
```

Set `UV_PYTHON_AGENT_HOOKS_FORCE_DOT_VENV=on` to force the venv target path to
`.venv` even when the original command names another path:

```powershell
$env:UV_PYTHON_AGENT_HOOKS_FORCE_DOT_VENV = "on"
```

With the switch enabled:

```text
python -m venv venv                  -> uv venv .venv
python -m venv --python 3.12 venv    -> uv venv --python 3.12 .venv
virtualenv env                       -> uv venv .venv
```

## Commands

```powershell
uv-python-hook install
uv-python-hook uninstall
uv-python-hook install --project --targets codex
uv-python-hook uninstall --project --targets codex
uv-python-hook doctor
uv-python-hook detect-project
uv-python-hook rewrite-command "pip install -r requirements.txt"
uv-python-hook rewrite-command --target opencode "python app.py"
```

## License

Copyright 2026 uv Python Agent Hooks contributors.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
