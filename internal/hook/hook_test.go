package hook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uvPrefix() string {
	return commandToShellText([]string{"uv", "--cache-dir", defaultUVCacheDir()}, "")
}

func TestRewritePythonScriptUsesUVRunPython(t *testing.T) {
	result := rewriteCommand("python app.py", "", "")
	if !result.Changed {
		t.Fatal("expected rewrite to change command")
	}
	want := uvPrefix() + " run python app.py"
	if result.Command != want {
		t.Fatalf("command = %q, want %q", result.Command, want)
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
	want := "cd src && " + uvPrefix() + " run python app.py"
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
	if fileExists(filepath.Join(dir, ".uv-python-agent-hooks")) {
		t.Fatal("hook-only install should not create shim dir")
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

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
