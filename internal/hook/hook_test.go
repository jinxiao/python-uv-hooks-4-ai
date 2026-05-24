package hook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	out := newInstaller("project", dir).install([]string{"codex", "claude", "opencode"})
	if out["activation"] != "hook-only" {
		t.Fatalf("activation = %#v", out["activation"])
	}
	if !fileExists(filepath.Join(dir, ".codex", "hooks.json")) {
		t.Fatal("missing codex hooks")
	}
	if !fileExists(filepath.Join(dir, ".claude", "settings.json")) {
		t.Fatal("missing claude settings")
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

func TestAutoInstallUsesDetectedClaudeCommand(t *testing.T) {
	withCommandAvailable(t, func(name string) bool {
		return name == "claude"
	})
	dir := t.TempDir()
	out := newInstaller("project", dir).install(nil)

	if out["target_selection"] != "auto" {
		t.Fatalf("target_selection = %#v, want auto", out["target_selection"])
	}
	if !reflect.DeepEqual(out["selected_targets"], []string{"claude"}) {
		t.Fatalf("selected_targets = %#v", out["selected_targets"])
	}
	if !fileExists(filepath.Join(dir, ".claude", "settings.json")) {
		t.Fatal("missing claude settings")
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
	if fileExists(filepath.Join(dir, ".claude", "settings.json")) {
		t.Fatal("claude settings should not be installed")
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

func TestProjectInstallIsIdempotentForClaude(t *testing.T) {
	dir := t.TempDir()
	inst := newInstaller("project", dir)
	inst.install([]string{"claude"})
	inst.install([]string{"claude"})
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	hooks := asMap(payload["hooks"])
	preToolUse := asSlice(hooks["PreToolUse"])
	if len(preToolUse) != 1 {
		t.Fatalf("hooks not idempotent: %s", string(data))
	}
	entry := asMap(preToolUse[0])
	if entry["matcher"] != "Bash" {
		t.Fatalf("matcher = %#v, want Bash", entry["matcher"])
	}
	handlers := asSlice(entry["hooks"])
	if len(handlers) != 1 {
		t.Fatalf("handlers = %#v, want one", handlers)
	}
	handler := asMap(handlers[0])
	if handler["type"] != "command" || handler["command"] != claudeOwnedPretoolCommand {
		t.Fatalf("unexpected claude hook handler: %#v", handler)
	}
}

func TestProjectInstallClaudeLeavesUnownedPretoolHookAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	ensureParent(path)
	writeFile(t, path, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "uv-python-hook claude-pretool"
          }
        ]
      }
    ]
  }
}`)

	out := newInstaller("project", dir).install([]string{"claude"})
	targets := asMap(out["targets"])
	claude := asMap(targets["claude"])
	if claude["changed"] != false {
		t.Fatalf("changed = %#v, want false", claude["changed"])
	}
	warnings := claude["warnings"].([]string)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one warning", warnings)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, claudeOwnedPretoolCommand) {
		t.Fatalf("owned hook should not be added when unowned hook exists: %s", text)
	}
	if strings.Count(text, claudeBarePretoolCommand) != 1 {
		t.Fatalf("unowned hook should be preserved without duplication: %s", text)
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

func TestProjectUninstallRemovesOnlyManagedClaudeHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	ensureParent(path)
	writeFile(t, path, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "existing-pretool"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "existing-posttool"
          }
        ]
      }
    ]
  }
}`)

	inst := newInstaller("project", dir)
	inst.install([]string{"claude"})
	out := inst.uninstall([]string{"claude"})
	targets := asMap(out["targets"])
	claude := asMap(targets["claude"])
	if claude["changed"] != true || claude["removed_hooks"] != 1 {
		t.Fatalf("unexpected uninstall result: %#v", claude)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "uv-python-hook claude-pretool") {
		t.Fatalf("managed hook was not removed: %s", text)
	}
	if !strings.Contains(text, "existing-pretool") || !strings.Contains(text, "existing-posttool") {
		t.Fatalf("existing hooks were not preserved: %s", text)
	}
}

func TestProjectUninstallClaudeIsNoopWhenManagedHookMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	ensureParent(path)
	original := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
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

	out := newInstaller("project", dir).uninstall([]string{"claude"})
	targets := asMap(out["targets"])
	claude := asMap(targets["claude"])
	if claude["changed"] != false || claude["removed_hooks"] != 0 {
		t.Fatalf("unexpected uninstall result: %#v", claude)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("uninstall changed unrelated hooks:\n%s", string(data))
	}
}

func TestProjectUninstallClaudePreservesCoLocatedHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	ensureParent(path)
	writeFile(t, path, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "existing-pretool"
          },
          {
            "type": "command",
            "command": "uv-python-hook claude-pretool # uv-python-hook-owned",
            "timeout": 10
          }
        ]
      }
    ]
  }
}`)

	out := newInstaller("project", dir).uninstall([]string{"claude"})
	targets := asMap(out["targets"])
	claude := asMap(targets["claude"])
	if claude["changed"] != true || claude["removed_hooks"] != 1 {
		t.Fatalf("unexpected uninstall result: %#v", claude)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	preToolUse := asSlice(asMap(payload["hooks"])["PreToolUse"])
	if len(preToolUse) != 1 {
		t.Fatalf("pretool entries = %#v, want one preserved entry", preToolUse)
	}
	entry := asMap(preToolUse[0])
	entryHooks := asSlice(entry["hooks"])
	if len(entryHooks) != 1 {
		t.Fatalf("entry hooks = %#v, want only user hook preserved", entryHooks)
	}
	hook := asMap(entryHooks[0])
	if hook["command"] != "existing-pretool" {
		t.Fatalf("unexpected preserved hook: %#v", hook)
	}
}

func TestProjectUninstallClaudeLeavesUnownedPretoolHookAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	ensureParent(path)
	original := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "uv-python-hook claude-pretool"
          }
        ]
      }
    ]
  }
}
`
	writeFile(t, path, original)

	out := newInstaller("project", dir).uninstall([]string{"claude"})
	targets := asMap(out["targets"])
	claude := asMap(targets["claude"])
	if claude["changed"] != false || claude["removed_hooks"] != 0 {
		t.Fatalf("unexpected uninstall result: %#v", claude)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("uninstall changed unowned hook:\n%s", string(data))
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
	inst.install([]string{"codex", "claude", "opencode"})

	out := inst.uninstall(nil)
	if out["target_selection"] != "auto" {
		t.Fatalf("target_selection = %#v, want auto", out["target_selection"])
	}
	if !reflect.DeepEqual(out["selected_targets"], []string{"codex", "claude", "opencode"}) {
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
	data, err = os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "uv-python-hook claude-pretool") {
		t.Fatalf("managed claude hook was not removed: %s", string(data))
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

func TestClaudePretoolPayloadShape(t *testing.T) {
	code, output := runPretoolWithPayload(t, claudePretool, map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "python app.py"},
	})
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(output, `"permissionDecision": "deny"`) {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "uv --cache-dir") || !strings.Contains(output, "run app.py") {
		t.Fatalf("output did not suggest uv rewrite: %s", output)
	}
}

func TestClaudePretoolPipInstallPayloadShape(t *testing.T) {
	code, output := runPretoolWithPayload(t, claudePretool, map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "pip install requests"},
	})
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(output, `"permissionDecision": "deny"`) {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "uv --cache-dir") || !strings.Contains(output, "pip install requests") {
		t.Fatalf("output did not suggest uv pip rewrite: %s", output)
	}
}

func TestClaudePretoolLeavesUnrelatedCommandSilent(t *testing.T) {
	code, output := runPretoolWithPayload(t, claudePretool, map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "echo hello"},
	})
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if output != "" {
		t.Fatalf("output = %q, want empty", output)
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

func runPretoolWithPayload(t *testing.T, run func(string) int, payload map[string]any) (int, string) {
	t.Helper()
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
	code := run("")
	_ = writeOut.Close()
	os.Stdout = oldStdout
	_, _ = io.Copy(&b, readOut)
	return code, b.String()
}
