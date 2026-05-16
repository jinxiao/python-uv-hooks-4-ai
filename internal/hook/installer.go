package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

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
	_ = os.WriteFile(path, []byte(opencodePluginSource()), 0o644)
	return map[string]any{
		"plugin": path,
		"mode":   "direct-rewrite",
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

func opencodePluginSource() string {
	return `import { spawnSync } from "node:child_process"
import fs from "node:fs"
import os from "node:os"
import path from "node:path"

export const UvPythonAgentHooks = async () => {
  const runner = "uv-python-hook"

  return {
    "tool.execute.before": async (input, output) => {
      if (!output.args || typeof output.args.command !== "string") {
        return
      }
      const payload = JSON.stringify({
        command: output.args.command,
        cwd: input.cwd || process.cwd(),
        target: "opencode",
      })
      const cwd = input.cwd || process.cwd()
      const env = { ...process.env }
      const cacheMode = (env.UV_PYTHON_AGENT_HOOKS_CACHE_MODE || "auto").toLowerCase()
      if (["1", "true", "yes", "on", "force", "forced"].includes(cacheMode)) {
        const cacheDir = env.UV_PYTHON_AGENT_HOOKS_CACHE_DIR || path.join(os.tmpdir(), "uv-python-agent-hooks", "uv-cache")
        fs.mkdirSync(cacheDir, { recursive: true })
        env.UV_CACHE_DIR = cacheDir
        env.UV_PYTHON_AGENT_HOOKS_CACHE_DIR = cacheDir
      }
      const result = spawnSync(runner, ["rewrite-command", "--target", "opencode"], {
        input: payload,
        cwd,
        encoding: "utf8",
        env,
        windowsHide: true,
      })
      if (result.status !== 0 || !result.stdout) {
        return
      }
      const parsed = JSON.parse(result.stdout)
      if (parsed.changed && parsed.command) {
        output.args.command = parsed.command
      }
    },
  }
}
`
}
