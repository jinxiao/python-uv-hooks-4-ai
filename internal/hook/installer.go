package hook

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/opencode-plugin.js
var opencodePluginSource string

func newInstaller(scope, cwd string) installer {
	if scope == "" {
		scope = "user"
	}
	return installer{scope: scope, cwd: cleanPath(cwd)}
}

func (i installer) install(targets []string) map[string]any {
	targetMap := map[string]any{}
	if contains(targets, "codex") {
		targetMap["codex"] = i.installCodex()
	}
	if contains(targets, "opencode") {
		targetMap["opencode"] = i.installOpenCode()
	}
	return map[string]any{
		"scope":      i.scope,
		"activation": "hook-only",
		"targets":    targetMap,
	}
}

func (i installer) uninstall(targets []string) map[string]any {
	var removed []string
	if contains(targets, "codex") {
		path := i.codexHooksPath()
		if fileExists(path) {
			data, err := readJSONFile(path)
			if err != nil {
				data = map[string]any{"hooks": map[string]any{}}
			}
			removeCodexHooks(data)
			writeJSONFile(path, data)
			removed = append(removed, path)
		}
	}
	if contains(targets, "opencode") {
		path := i.opencodePluginPath()
		if fileExists(path) {
			_ = os.Remove(path)
			removed = append(removed, path)
		}
	}
	return map[string]any{
		"scope":   i.scope,
		"removed": removed,
	}
}

func (i installer) installState() map[string]any {
	codexHooks := i.codexHooksPath()
	opencodePlugin := i.opencodePluginPath()
	return map[string]any{
		"codex_hooks":            codexHooks,
		"codex_hooks_exists":     fileExists(codexHooks),
		"opencode_plugin":        opencodePlugin,
		"opencode_plugin_exists": fileExists(opencodePlugin),
	}
}

func (i installer) installCodex() map[string]any {
	path := i.codexHooksPath()
	ensureParent(path)
	data, err := readJSONFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			backupInvalidJSON(path)
		}
		data = map[string]any{"hooks": map[string]any{}}
	}
	hooks, ok := data["hooks"].(map[string]any)
	if !ok {
		hooks = data
		data = map[string]any{"hooks": hooks}
	}
	removeCodexHooks(data)
	for _, event := range []string{"PreToolUse", "PermissionRequest"} {
		entries := asSlice(hooks[event])
		entries = append(entries, map[string]any{
			"matcher": "*",
			"hooks": []any{
				map[string]any{
					"type":          "command",
					"command":       "uv-python-hook codex-pretool",
					"timeout":       10,
					"statusMessage": "Checking Python command uv policy",
				},
			},
		})
		hooks[event] = entries
	}
	writeJSONFile(path, data)
	return map[string]any{
		"hooks_json": path,
		"mode":       "deny-and-suggest",
	}
}

func (i installer) installOpenCode() map[string]any {
	path := i.opencodePluginPath()
	ensureParent(path)
	_ = os.WriteFile(path, []byte(opencodePluginSource), 0o644)
	runner := which("uv-python-hook")
	warnings := []string{}
	if runner == nil {
		warnings = append(warnings, "uv-python-hook is not on PATH; Opencode can load the plugin, but rewrites will not run until the runner is reachable.")
	}
	return map[string]any{
		"plugin":           path,
		"mode":             "direct-rewrite",
		"runner":           runner,
		"runner_available": runner != nil,
		"warnings":         warnings,
	}
}

func (i installer) codexHooksPath() string {
	if i.scope == "project" {
		return filepath.Join(i.cwd, ".codex", "hooks.json")
	}
	return filepath.Join(homeDir(), ".codex", "hooks.json")
}

func (i installer) opencodePluginPath() string {
	if i.scope == "project" {
		return filepath.Join(i.cwd, ".opencode", "plugins", "uv-python-agent-hooks.js")
	}
	return filepath.Join(homeDir(), ".config", "opencode", "plugins", "uv-python-agent-hooks.js")
}

func removeCodexHooks(data map[string]any) {
	hooks := asMap(data["hooks"])
	if hooks == nil {
		hooks = data
	}
	for _, event := range []string{"PreToolUse", "PermissionRequest"} {
		entries := asSlice(hooks[event])
		var kept []any
		for _, entry := range entries {
			encoded, _ := json.Marshal(entry)
			if !isOurCodexHook(string(encoded)) {
				kept = append(kept, entry)
			}
		}
		hooks[event] = kept
	}
}

func isOurCodexHook(text string) bool {
	return strings.Contains(text, "uv-python-hook codex-pretool") ||
		strings.Contains(text, "uv-python-hook-runner") ||
		strings.Contains(text, "uv_python_agent_hooks")
}
