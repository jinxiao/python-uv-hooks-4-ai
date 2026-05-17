package hook

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var commandAvailable = func(name string) bool {
	return which(name) != nil
}

//go:embed assets/opencode-plugin.js
var opencodePluginSource string

func newInstaller(scope, cwd string) installer {
	if scope == "" {
		scope = "user"
	}
	return installer{scope: scope, cwd: cleanPath(cwd)}
}

func (i installer) install(targets []string) map[string]any {
	targetSelection := "explicit"
	if len(targets) == 0 {
		targetSelection = "auto"
		targets = i.autoInstallTargets()
	}
	targetMap := map[string]any{}
	if contains(targets, "codex") {
		targetMap["codex"] = i.installCodex()
	}
	if contains(targets, "opencode") {
		targetMap["opencode"] = i.installOpenCode()
	}
	return map[string]any{
		"scope":            i.scope,
		"activation":       "hook-only",
		"target_selection": targetSelection,
		"selected_targets": targets,
		"targets":          targetMap,
	}
}

func (i installer) uninstall(targets []string) map[string]any {
	targetSelection := "explicit"
	if len(targets) == 0 {
		targetSelection = "auto"
		targets = i.autoUninstallTargets()
	}
	var removed []string
	targetMap := map[string]any{}
	if contains(targets, "codex") {
		result := i.uninstallCodex()
		targetMap["codex"] = result
		if changed, _ := result["changed"].(bool); changed {
			removed = append(removed, i.codexHooksPath())
		}
	}
	if contains(targets, "opencode") {
		result := i.uninstallOpenCode()
		targetMap["opencode"] = result
		if changed, _ := result["changed"].(bool); changed {
			removed = append(removed, i.opencodePluginPath())
		}
	}
	return map[string]any{
		"scope":            i.scope,
		"target_selection": targetSelection,
		"selected_targets": targets,
		"removed":          removed,
		"targets":          targetMap,
	}
}

func (i installer) autoInstallTargets() []string {
	targets := []string{}
	if commandAvailable("codex") {
		targets = append(targets, "codex")
	}
	if commandAvailable("opencode") {
		targets = append(targets, "opencode")
	}
	return targets
}

func (i installer) autoUninstallTargets() []string {
	targets := []string{}
	if commandAvailable("codex") || fileExists(i.codexHooksPath()) {
		targets = append(targets, "codex")
	}
	if commandAvailable("opencode") || fileExists(i.opencodePluginPath()) {
		targets = append(targets, "opencode")
	}
	return targets
}

func (i installer) uninstallCodex() map[string]any {
	path := i.codexHooksPath()
	result := map[string]any{
		"hooks_json":    path,
		"exists":        fileExists(path),
		"changed":       false,
		"removed_hooks": 0,
	}
	if !fileExists(path) {
		return result
	}
	data, err := readJSONFile(path)
	if err != nil {
		result["error"] = err.Error()
		return result
	}
	removed := removeCodexHooks(data)
	result["removed_hooks"] = removed
	if removed == 0 {
		return result
	}
	writeJSONFile(path, data)
	result["changed"] = true
	return result
}

func (i installer) uninstallOpenCode() map[string]any {
	path := i.opencodePluginPath()
	result := map[string]any{
		"plugin":  path,
		"exists":  fileExists(path),
		"changed": false,
	}
	if !fileExists(path) {
		return result
	}
	if err := os.Remove(path); err != nil {
		result["error"] = err.Error()
		return result
	}
	result["changed"] = true
	return result
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

func removeCodexHooks(data map[string]any) int {
	hooks := asMap(data["hooks"])
	if hooks == nil {
		hooks = data
	}
	removed := 0
	for _, event := range []string{"PreToolUse", "PermissionRequest"} {
		entries := asSlice(hooks[event])
		if len(entries) == 0 {
			continue
		}
		var kept []any
		for _, entry := range entries {
			encoded, _ := json.Marshal(entry)
			if !isOurCodexHook(string(encoded)) {
				kept = append(kept, entry)
			} else {
				removed++
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	return removed
}

func isOurCodexHook(text string) bool {
	return strings.Contains(text, "uv-python-hook codex-pretool") ||
		strings.Contains(text, "uv-python-hook-runner") ||
		strings.Contains(text, "uv_python_agent_hooks")
}
