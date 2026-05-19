package hook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func uvPrefix() string {
	return commandToShellText([]string{"uv", "--cache-dir", defaultUVCacheDir()}, "")
}

func uvToolRunPrefixForTest() string {
	return commandToShellText([]string{"uv", "--cache-dir", defaultUVCacheDir(), "tool", "run"}, "")
}

func TestRewritePythonScriptUsesUVRunPython(t *testing.T) {
	result := rewriteCommand("python app.py", "", "")
	if !result.Changed {
		t.Fatal("expected rewrite to change command")
	}
	want := uvPrefix() + " run app.py"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteOpenCodeDefaultDoesNotForceUVCache(t *testing.T) {
	result := rewriteCommandWithOptions(rewriteOptions{
		command: "python app.py",
		target:  "opencode",
	})
	want := "uv run app.py"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteOpenCodeCanForceUVCache(t *testing.T) {
	result := rewriteCommandWithOptions(rewriteOptions{
		command:   "python app.py",
		target:    "opencode",
		cacheMode: "on",
	})
	want := uvPrefix() + " run app.py"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteVenvAddsDotVenvByDefault(t *testing.T) {
	result := rewriteCommandWithOptions(rewriteOptions{
		command: "python -m venv",
		target:  "opencode",
	})
	want := "uv venv .venv"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteVenvPreservesExplicitPathByDefault(t *testing.T) {
	result := rewriteCommandWithOptions(rewriteOptions{
		command: "python -m venv venv",
		target:  "opencode",
	})
	want := "uv venv venv"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteVenvCanForceDotVenvPath(t *testing.T) {
	t.Setenv(hookForceDotVenvEnv, "on")
	result := rewriteCommandWithOptions(rewriteOptions{
		command: "python -m venv venv",
		target:  "opencode",
	})
	want := "uv venv .venv"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteVenvForceDotVenvPreservesOptions(t *testing.T) {
	t.Setenv(hookForceDotVenvEnv, "on")
	result := rewriteCommandWithOptions(rewriteOptions{
		command: "python -m venv --python 3.12 --prompt demo venv",
		target:  "opencode",
	})
	want := "uv venv --python 3.12 --prompt demo .venv"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteVenvForceDotVenvPreservesUVValueOptions(t *testing.T) {
	t.Setenv(hookForceDotVenvEnv, "on")
	cases := []struct {
		command string
		want    string
	}{
		{
			command: "python -m venv --python-preference only-managed env",
			want:    "uv venv --python-preference only-managed .venv",
		},
		{
			command: "python -m venv --link-mode copy env",
			want:    "uv venv --link-mode copy .venv",
		},
		{
			command: "python -m venv --link-mode copy",
			want:    "uv venv --link-mode copy .venv",
		},
	}
	for _, tc := range cases {
		result := rewriteCommandWithOptions(rewriteOptions{
			command: tc.command,
			target:  "opencode",
		})
		if result.Command != tc.want {
			t.Fatalf("command = %q, want %q", result.Command, tc.want)
		}
	}
}

func TestRewritePipInstallPackageUsesUVPip(t *testing.T) {
	result := rewriteCommand("pip install requests", "", "")
	want := uvPrefix() + " pip install requests"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestRewriteCompoundCommand(t *testing.T) {
	result := rewriteCommand("cd src && python app.py", "", "")
	want := "cd src && " + uvPrefix() + " run app.py"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestAlreadyUVIsUnchanged(t *testing.T) {
	result := rewriteCommand("uv run python app.py", "", "")
	if result.Changed {
		t.Fatal("expected uv command to remain unchanged")
	}
}

func TestDetectSyncableProjectDependencies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = 'demo'\nversion = '0.1.0'\ndependencies = ['requests']\n")
	result := detectProject(dir)
	if !result.Syncable {
		t.Fatal("expected syncable project")
	}
	if !contains(result.Reasons, "project.dependencies") {
		t.Fatalf("reasons = %#v", result.Reasons)
	}
}

func TestDetectUVLockProjectWithoutPyproject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "uv.lock"), "")
	result := detectProject(dir)
	if result.Manager != "uv" || !result.Syncable {
		t.Fatalf("result = %#v", result)
	}
	if result.UVLock == nil {
		t.Fatal("expected uv lock path")
	}
}

func TestPoetryProjectIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "poetry.lock"), "")
	result := rewriteCommand("python app.py", dir, "")
	if result.Changed {
		t.Fatalf("expected Poetry command to remain unchanged, got %q", result.Command)
	}
	if result.Project.Manager != "poetry" {
		t.Fatalf("manager = %q", result.Project.Manager)
	}
}

func TestPDMProjectIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pdm.lock"), "")
	result := rewriteCommand("pip install requests", dir, "")
	if result.Changed {
		t.Fatalf("expected PDM command to remain unchanged, got %q", result.Command)
	}
	if result.Project.Manager != "pdm" {
		t.Fatalf("manager = %q", result.Project.Manager)
	}
}

func TestStandaloneToolUsesUVToolRunWithCache(t *testing.T) {
	result := rewriteCommand("ruff check .", "", "")
	want := uvToolRunPrefixForTest() + " ruff check ."
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestStandaloneToolUsesUVXForOpenCode(t *testing.T) {
	result := rewriteCommandWithOptions(rewriteOptions{
		command: "ruff check .",
		target:  "opencode",
	})
	want := "uvx ruff check ."
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestProjectToolUsesUVRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = 'demo'\nversion = '0.1.0'\n")
	result := rewriteCommand("ruff check .", dir, "")
	want := uvPrefix() + " run ruff check ."
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
	}
}

func TestDetectIncompleteProjectSection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\ndependencies = ['requests']\n")
	result := detectProject(dir)
	if result.Syncable {
		t.Fatal("expected incomplete project to be non-syncable")
	}
	if !contains(result.Issues, "[project].name is required for uv sync") {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Suggestion == nil {
		t.Fatal("expected suggestion")
	}
}

func TestProjectInstallGeneratesOnlyHookFiles(t *testing.T) {
	dir := t.TempDir()
	out := newInstaller("project", dir).install([]string{"codex", "opencode"})
	if out["activation"] != "hook-only" {
		t.Fatalf("activation = %#v", out["activation"])
	}
	if !fileExists(filepath.Join(dir, ".codex", "hooks.json")) {
		t.Fatal("missing codex hooks")
	}
	if !fileExists(filepath.Join(dir, ".opencode", "plugins", "uv-python-agent-hooks.js")) {
		t.Fatal("missing opencode plugin")
	}
	plugin, err := os.ReadFile(filepath.Join(dir, ".opencode", "plugins", "uv-python-agent-hooks.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plugin), "Managed file: uv-python-agent-hooks") {
		t.Fatalf("opencode plugin missing managed marker: %s", string(plugin))
	}
	if fileExists(filepath.Join(dir, ".uv-python-agent-hooks")) {
		t.Fatal("hook-only install should not create shim dir")
	}
}

func TestInstallArgsDefaultToUserScopeAndAutoTargets(t *testing.T) {
	opts := parseInstallArgs(nil)
	if opts.scope != "user" {
		t.Fatalf("scope = %q, want user", opts.scope)
	}
	if opts.targets != "" {
		t.Fatalf("targets = %q, want empty auto target selection", opts.targets)
	}
	if !opts.installBinary {
		t.Fatal("install should include binary install by default")
	}
	if !opts.updateBinaryPath {
		t.Fatal("expected binary PATH updates to be enabled when binary install is requested")
	}
}

func TestInstallArgsCanConfigureBinaryInstall(t *testing.T) {
	opts := parseInstallArgs([]string{"--bin-dir", "custom-bin", "--no-path"})
	if !opts.installBinary {
		t.Fatal("expected binary install to be enabled by default")
	}
	if opts.binaryDir != "custom-bin" {
		t.Fatalf("binaryDir = %q, want custom-bin", opts.binaryDir)
	}
	if opts.updateBinaryPath {
		t.Fatal("expected --no-path to disable binary PATH updates")
	}

	opts = parseInstallArgs([]string{"--debug"})
	if !opts.debug {
		t.Fatal("expected --debug to enable detailed install output")
	}
}

func TestInstallArgsCanDisableBinaryInstall(t *testing.T) {
	opts := parseInstallArgs([]string{"--hooks-only"})
	if opts.installBinary {
		t.Fatal("expected --hooks-only to disable binary install")
	}

	opts = parseInstallArgs([]string{"--no-binary"})
	if opts.installBinary {
		t.Fatal("expected --no-binary to disable binary install")
	}
}

func TestInstallBinaryArgsDefaultToUpdatingPath(t *testing.T) {
	opts := parseInstallBinaryArgs(nil)
	if !opts.updatePath {
		t.Fatal("expected install-bin to update PATH by default")
	}
	if opts.dir != "" {
		t.Fatalf("dir = %q, want empty default", opts.dir)
	}

	opts = parseInstallBinaryArgs([]string{"--dir", "custom-bin", "--no-path"})
	if opts.updatePath {
		t.Fatal("expected --no-path to disable PATH updates")
	}
	if opts.dir != "custom-bin" {
		t.Fatalf("dir = %q, want custom-bin", opts.dir)
	}

	opts = parseInstallBinaryArgs([]string{"--debug"})
	if !opts.debug {
		t.Fatal("expected --debug to enable detailed install-bin output")
	}
}

func TestInstallBinaryCopiesExecutableWithoutPathUpdate(t *testing.T) {
	dir := t.TempDir()
	out := installBinary(installBinaryOptions{dir: dir, updatePath: false})

	if out["error"] != nil {
		t.Fatalf("install-bin error: %#v", out["error"])
	}
	destination := filepath.Join(dir, binaryInstallName())
	if out["destination"] != destination {
		t.Fatalf("destination = %#v, want %q", out["destination"], destination)
	}
	if !fileExists(destination) {
		t.Fatal("expected installed binary")
	}
	pathResult := asMap(out["path"])
	if pathResult["enabled"] != false {
		t.Fatalf("path result = %#v, want disabled", pathResult)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("installed binary is not executable: %v", info.Mode())
		}
	}
}

func TestInstallBinaryIsIdempotentWhenDestinationContentMatches(t *testing.T) {
	dir := t.TempDir()
	first := installBinary(installBinaryOptions{dir: dir, updatePath: false})
	if first["error"] != nil {
		t.Fatalf("first install-bin error: %#v", first["error"])
	}

	second := installBinary(installBinaryOptions{dir: dir, updatePath: false})
	if second["error"] != nil {
		t.Fatalf("second install-bin error: %#v", second["error"])
	}
	if second["changed"] != false {
		t.Fatalf("second install changed = %#v, want false", second["changed"])
	}
	if second["already_installed"] != true {
		t.Fatalf("already_installed = %#v, want true", second["already_installed"])
	}
}

func TestInstallBinaryUpdatesExistingDifferentDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, binaryInstallName())
	writeFile(t, destination, "old binary")

	out := installBinary(installBinaryOptions{dir: dir, updatePath: false})
	if out["error"] != nil {
		t.Fatalf("install-bin error: %#v", out["error"])
	}
	if out["action"] != "updated" {
		t.Fatalf("action = %#v, want updated", out["action"])
	}
	if out["changed"] != true {
		t.Fatalf("changed = %#v, want true", out["changed"])
	}
	source, err := currentExecutablePath()
	if err != nil {
		t.Fatal(err)
	}
	if !sameFileContent(source, destination) {
		t.Fatal("destination was not replaced with current executable")
	}
}

func TestDefaultInstallDestinationPrefersExistingPathBinary(t *testing.T) {
	source := filepath.Join(t.TempDir(), binaryInstallName())
	writeFile(t, source, "source")
	pathDir := t.TempDir()
	pathBinary := filepath.Join(pathDir, binaryInstallName())
	writeFile(t, pathBinary, "old binary")
	t.Setenv("PATH", pathDir)

	installDir, destination := binaryInstallDestination("", source)

	if !samePath(installDir, pathDir) {
		t.Fatalf("installDir = %q, want %q", installDir, pathDir)
	}
	if !samePath(destination, pathBinary) {
		t.Fatalf("destination = %q, want %q", destination, pathBinary)
	}
}

func TestDetectInstalledBinariesScansProcessPath(t *testing.T) {
	source := filepath.Join(t.TempDir(), binaryInstallName())
	writeFile(t, source, "source")
	pathDir := t.TempDir()
	pathBinary := filepath.Join(pathDir, binaryInstallName())
	writeFile(t, pathBinary, "old binary")
	t.Setenv("PATH", pathDir)

	detected := detectInstalledBinaries(source, filepath.Join(t.TempDir(), binaryInstallName()))

	var found bool
	for _, candidate := range detected {
		kind, _ := candidate["kind"].(string)
		path, _ := candidate["path"].(string)
		if strings.Contains(kind, "process-path") && samePath(path, pathBinary) {
			found = true
		}
	}
	if !found {
		t.Fatalf("process PATH binary was not detected: %#v", detected)
	}
}

func TestResolvedBinaryPathFallsBackToManualPathScan(t *testing.T) {
	pathDir := t.TempDir()
	pathBinary := filepath.Join(pathDir, binaryInstallName())
	writeFile(t, pathBinary, "old binary")
	t.Setenv("PATH", pathDir)

	resolved := resolvedBinaryPath()

	if resolved == nil || !samePath(*resolved, pathBinary) {
		t.Fatalf("resolved = %#v, want %q", resolved, pathBinary)
	}
}

func TestEnsureDirFirstInProcessPathReordersExistingDir(t *testing.T) {
	oldDir := t.TempDir()
	installDir := t.TempDir()
	t.Setenv("PATH", oldDir+string(os.PathListSeparator)+installDir)

	ensureDirFirstInProcessPath(installDir)

	parts := filepath.SplitList(os.Getenv("PATH"))
	if len(parts) == 0 || !samePath(parts[0], installDir) {
		t.Fatalf("PATH = %q, want install dir first", os.Getenv("PATH"))
	}
	if strings.Count(os.Getenv("PATH"), installDir) != 1 {
		t.Fatalf("PATH duplicated install dir: %q", os.Getenv("PATH"))
	}
}

func TestShellProfileRecognizesExistingHomeLocalBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell profile PATH detection is for Unix-like systems")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	profile := shellProfilePath()
	writeFile(t, profile, "export PATH=\"$HOME/.local/bin:$PATH\"\n")

	if !shellProfileHasDir(filepath.Join(home, ".local", "bin")) {
		t.Fatal("expected $HOME/.local/bin to be recognized")
	}
	changed, _, _, err := ensureShellProfilePath(filepath.Join(home, ".local", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected existing profile entry to be idempotent")
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), ".local/bin") != 1 {
		t.Fatalf("profile entry duplicated:\n%s", string(data))
	}
}

func TestInstallBinaryPersistsUserPathEvenWhenProcessPathAlreadyHasDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell profile PATH mutation is for Unix-like systems")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	installDir := filepath.Join(home, ".local", "bin")
	t.Setenv("PATH", installDir)

	out := installBinary(installBinaryOptions{updatePath: true})
	if out["error"] != nil {
		t.Fatalf("install-bin error: %#v", out["error"])
	}
	pathResult := asMap(out["path"])
	if pathResult["persistent_path_changed"] != true {
		t.Fatalf("path result = %#v, want persistent PATH change", pathResult)
	}
	if pathResult["process_path_precedence_changed"] != false {
		t.Fatalf("path result = %#v, process PATH should already contain dir", pathResult)
	}
	data, err := os.ReadFile(shellProfilePath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), pathProfileMarker) {
		t.Fatalf("missing managed profile entry:\n%s", string(data))
	}
}

func TestExpandWindowsPercentEnvironmentVariables(t *testing.T) {
	t.Setenv("LOCALAPPDATA", filepath.Join("C:", "Users", "demo", "AppData", "Local"))
	got := expandWindowsEnv(`%LOCALAPPDATA%\Programs\uv-python-hook`)
	if !strings.Contains(got, filepath.Join("C:", "Users", "demo", "AppData", "Local")) {
		t.Fatalf("expanded path = %q", got)
	}
}

func TestAutoInstallUsesDetectedCommands(t *testing.T) {
	withCommandAvailable(t, func(name string) bool {
		return name == "codex"
	})
	dir := t.TempDir()
	out := newInstaller("project", dir).install(nil)

	if out["target_selection"] != "auto" {
		t.Fatalf("target_selection = %#v, want auto", out["target_selection"])
	}
	if !reflect.DeepEqual(out["selected_targets"], []string{"codex"}) {
		t.Fatalf("selected_targets = %#v", out["selected_targets"])
	}
	if !fileExists(filepath.Join(dir, ".codex", "hooks.json")) {
		t.Fatal("missing codex hooks")
	}
	if fileExists(filepath.Join(dir, ".opencode", "plugins", "uv-python-agent-hooks.js")) {
		t.Fatal("opencode plugin should not be installed when opencode is not detected")
	}
}

func TestAutoInstallSkipsWhenNoTargetsDetected(t *testing.T) {
	withCommandAvailable(t, func(string) bool {
		return false
	})
	dir := t.TempDir()
	out := newInstaller("project", dir).install(nil)

	if !reflect.DeepEqual(out["selected_targets"], []string{}) {
		t.Fatalf("selected_targets = %#v, want empty", out["selected_targets"])
	}
	if len(asMap(out["targets"])) != 0 {
		t.Fatalf("targets = %#v, want none", out["targets"])
	}
	if fileExists(filepath.Join(dir, ".codex", "hooks.json")) {
		t.Fatal("codex hooks should not be installed")
	}
	if fileExists(filepath.Join(dir, ".opencode", "plugins", "uv-python-agent-hooks.js")) {
		t.Fatal("opencode plugin should not be installed")
	}
}

func TestProjectInstallIsIdempotentForCodex(t *testing.T) {
	dir := t.TempDir()
	inst := newInstaller("project", dir)
	inst.install([]string{"codex"})
	inst.install([]string{"codex"})
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	hooks := asMap(payload["hooks"])
	if len(asSlice(hooks["PreToolUse"])) != 1 || len(asSlice(hooks["PermissionRequest"])) != 1 {
		t.Fatalf("hooks not idempotent: %s", string(data))
	}
}

func TestProjectUninstallRemovesOnlyManagedCodexHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "hooks.json")
	ensureParent(path)
	writeFile(t, path, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Shell",
        "hooks": [
          {
            "type": "command",
            "command": "existing-pretool"
          }
        ]
      }
    ],
    "PermissionRequest": [
      {
        "matcher": "Shell",
        "hooks": [
          {
            "type": "command",
            "command": "existing-permission"
          }
        ]
      }
    ]
  }
}`)

	inst := newInstaller("project", dir)
	inst.install([]string{"codex"})
	out := inst.uninstall([]string{"codex"})
	targets := asMap(out["targets"])
	codex := asMap(targets["codex"])
	if codex["changed"] != true || codex["removed_hooks"] != 2 {
		t.Fatalf("unexpected uninstall result: %#v", codex)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "uv-python-hook codex-pretool") {
		t.Fatalf("managed hook was not removed: %s", text)
	}
	if !strings.Contains(text, "existing-pretool") || !strings.Contains(text, "existing-permission") {
		t.Fatalf("existing hooks were not preserved: %s", text)
	}
}

func TestProjectUninstallCodexIsNoopWhenManagedHookMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "hooks.json")
	ensureParent(path)
	original := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Shell",
        "hooks": [
          {
            "type": "command",
            "command": "existing-pretool"
          }
        ]
      }
    ]
  }
}
`
	writeFile(t, path, original)

	out := newInstaller("project", dir).uninstall([]string{"codex"})
	targets := asMap(out["targets"])
	codex := asMap(targets["codex"])
	if codex["changed"] != false || codex["removed_hooks"] != 0 {
		t.Fatalf("unexpected uninstall result: %#v", codex)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("uninstall changed unrelated hooks:\n%s", string(data))
	}
}

func TestProjectUninstallRemovesOpenCodePlugin(t *testing.T) {
	dir := t.TempDir()
	inst := newInstaller("project", dir)
	inst.install([]string{"opencode"})
	path := filepath.Join(dir, ".opencode", "plugins", "uv-python-agent-hooks.js")
	if !fileExists(path) {
		t.Fatal("missing opencode plugin before uninstall")
	}

	out := inst.uninstall([]string{"opencode"})
	targets := asMap(out["targets"])
	opencode := asMap(targets["opencode"])
	if opencode["changed"] != true {
		t.Fatalf("unexpected uninstall result: %#v", opencode)
	}
	if fileExists(path) {
		t.Fatal("opencode plugin still exists after uninstall")
	}
}

func TestAutoUninstallRemovesExistingHooksEvenWhenCommandsAreMissing(t *testing.T) {
	withCommandAvailable(t, func(string) bool {
		return false
	})
	dir := t.TempDir()
	inst := newInstaller("project", dir)
	inst.install([]string{"codex", "opencode"})

	out := inst.uninstall(nil)
	if out["target_selection"] != "auto" {
		t.Fatalf("target_selection = %#v, want auto", out["target_selection"])
	}
	if !reflect.DeepEqual(out["selected_targets"], []string{"codex", "opencode"}) {
		t.Fatalf("selected_targets = %#v", out["selected_targets"])
	}
	if fileExists(filepath.Join(dir, ".opencode", "plugins", "uv-python-agent-hooks.js")) {
		t.Fatal("opencode plugin still exists after auto uninstall")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "uv-python-hook codex-pretool") {
		t.Fatalf("managed codex hook was not removed: %s", string(data))
	}
}

func TestCodexPretoolPayloadShape(t *testing.T) {
	payload := map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "python app.py"},
	}
	encoded, _ := json.Marshal(payload)
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	_, _ = w.Write(encoded)
	_ = w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	var b strings.Builder
	oldStdout := os.Stdout
	readOut, writeOut, _ := os.Pipe()
	os.Stdout = writeOut
	code := codexPretool("")
	_ = writeOut.Close()
	os.Stdout = oldStdout
	_, _ = io.Copy(&b, readOut)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(b.String(), `"permissionDecision": "deny"`) {
		t.Fatalf("unexpected output: %s", b.String())
	}
}

func TestVersionStringIncludesInjectedMetadata(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = oldVersion, oldCommit, oldDate
	})

	Version = "1.2.3"
	Commit = "abc123"
	Date = "2026-05-17T08:00:00Z"

	want := "uv-python-hook 1.2.3 (commit=abc123, date=2026-05-17T08:00:00Z)"
	if got := VersionString(); got != want {
		t.Fatalf("VersionString() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withCommandAvailable(t *testing.T, available func(string) bool) {
	t.Helper()
	original := commandAvailable
	commandAvailable = available
	t.Cleanup(func() {
		commandAvailable = original
	})
}
