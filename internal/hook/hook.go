package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	uvCacheEnv          = "UV_CACHE_DIR"
	hookCacheEnv        = "UV_PYTHON_AGENT_HOOKS_CACHE_DIR"
	hookCacheModeEnv    = "UV_PYTHON_AGENT_HOOKS_CACHE_MODE"
	hookForceDotVenvEnv = "UV_PYTHON_AGENT_HOOKS_FORCE_DOT_VENV"
)

type projectDetection struct {
	CWD        string   `json:"cwd"`
	Pyproject  *string  `json:"pyproject"`
	UVLock     *string  `json:"uv_lock"`
	Root       *string  `json:"root"`
	Manager    string   `json:"manager"`
	Syncable   bool     `json:"syncable"`
	Reasons    []string `json:"reasons"`
	Issues     []string `json:"issues"`
	Suggestion *string  `json:"suggestion"`
}

type rewriteResult struct {
	Original string           `json:"original"`
	Command  string           `json:"command"`
	Changed  bool             `json:"changed"`
	Reason   *string          `json:"reason"`
	Project  projectDetection `json:"project"`
}

type installer struct {
	scope string
	cwd   string
}

type rewriteOptions struct {
	cwd       string
	shell     string
	command   string
	target    string
	cacheMode string
}

func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	switch args[0] {
	case "--version", "version":
		fmt.Println(VersionString())
		return 0
	case "install-bin":
		opts := parseInstallBinaryArgs(args[1:])
		result := installBinary(opts)
		if opts.debug {
			printJSON(result)
		} else {
			printInstallBinarySummary(result)
		}
		return 0
	case "install":
		opts := parseInstallArgs(args[1:])
		var binaryResult map[string]any
		if opts.installBinary {
			binaryResult = installBinary(installBinaryOptions{
				dir:        opts.binaryDir,
				updatePath: opts.updateBinaryPath,
			})
		}
		result := newInstaller(opts.scope, opts.cwd).install(splitTargets(opts.targets))
		if binaryResult != nil {
			result["activation"] = "binary-and-hooks"
			result["binary"] = binaryResult
		}
		if opts.debug {
			printJSON(result)
		} else {
			printInstallSummary(result)
		}
		return 0
	case "uninstall":
		opts := parseInstallArgs(args[1:])
		printJSON(newInstaller(opts.scope, opts.cwd).uninstall(splitTargets(opts.targets)))
		return 0
	case "doctor":
		cwd := parseValueFlag(args[1:], "--cwd")
		printJSON(doctor(cwd))
		if _, err := exec.LookPath("uv"); err != nil {
			return 1
		}
		return 0
	case "detect-project":
		cwd := parseValueFlag(args[1:], "--cwd")
		printJSON(detectProject(cwd))
		return 0
	case "rewrite-command":
		opts := parseRewriteArgs(args[1:])
		if opts.command == "" {
			payload := readJSONStdin()
			if value, ok := payload["command"].(string); ok {
				opts.command = value
			}
			if value, ok := payload["cwd"].(string); ok && opts.cwd == "" {
				opts.cwd = value
			}
			if value, ok := payload["shell"].(string); ok && opts.shell == "" {
				opts.shell = value
			}
			if value, ok := payload["target"].(string); ok && opts.target == "" {
				opts.target = value
			}
			if value, ok := payload["cache_mode"].(string); ok && opts.cacheMode == "" {
				opts.cacheMode = value
			}
		}
		printJSON(rewriteCommandWithOptions(opts))
		return 0
	case "codex-pretool":
		cwd := parseValueFlag(args[1:], "--cwd")
		return codexPretool(cwd)
	default:
		printUsage()
		return 2
	}
}

func printUsage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: uv-python-hook <install-bin|install|uninstall|doctor|detect-project|rewrite-command|codex-pretool|version|--version>")
}

type installOptions struct {
	scope            string
	targets          string
	cwd              string
	installBinary    bool
	binaryDir        string
	updateBinaryPath bool
	debug            bool
}

type installBinaryOptions struct {
	dir        string
	updatePath bool
	debug      bool
}

func parseInstallBinaryArgs(args []string) installBinaryOptions {
	opts := installBinaryOptions{updatePath: true}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				i++
				opts.dir = args[i]
			}
		case "--no-path":
			opts.updatePath = false
		case "--debug":
			opts.debug = true
		}
	}
	return opts
}

func parseInstallArgs(args []string) installOptions {
	opts := installOptions{scope: "user", installBinary: true, updateBinaryPath: true}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			opts.scope = "user"
		case "--project":
			opts.scope = "project"
		case "--targets":
			if i+1 < len(args) {
				i++
				opts.targets = args[i]
			}
		case "--cwd":
			if i+1 < len(args) {
				i++
				opts.cwd = args[i]
			}
		case "--with-binary", "--binary":
			opts.installBinary = true
		case "--hooks-only", "--no-binary":
			opts.installBinary = false
		case "--bin-dir":
			if i+1 < len(args) {
				i++
				opts.binaryDir = args[i]
			}
		case "--no-path":
			opts.updateBinaryPath = false
		case "--debug":
			opts.debug = true
		}
	}
	return opts
}

func parseValueFlag(args []string, name string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func parseRewriteArgs(args []string) rewriteOptions {
	var opts rewriteOptions
	var commandParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cwd":
			if i+1 < len(args) {
				i++
				opts.cwd = args[i]
			}
		case "--shell":
			if i+1 < len(args) {
				i++
				opts.shell = args[i]
			}
		case "--target":
			if i+1 < len(args) {
				i++
				opts.target = args[i]
			}
		case "--cache-mode":
			if i+1 < len(args) {
				i++
				opts.cacheMode = args[i]
			}
		case "--":
			commandParts = append(commandParts, args[i+1:]...)
			i = len(args)
		default:
			commandParts = append(commandParts, args[i])
		}
	}
	opts.command = stripWrappingQuote(strings.Join(commandParts, " "))
	return opts
}

func splitTargets(text string) []string {
	var targets []string
	for _, item := range strings.Split(text, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			targets = append(targets, item)
		}
	}
	return targets
}

func readJSONStdin() map[string]any {
	var payload map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func printJSON(value any) {
	encoded, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(encoded))
}

func printInstallBinarySummary(result map[string]any) {
	if errText, _ := result["error"].(string); errText != "" {
		fmt.Println("uv-python-hook install-bin failed: " + errText)
		return
	}
	action, _ := result["action"].(string)
	destination, _ := result["destination"].(string)
	fmt.Println("uv-python-hook binary " + installActionText(action) + ": " + destination)
	printPathSummary(asMap(result["path"]))
	printWarnings(result["warnings"])
	fmt.Println("Run with --debug for details.")
}

func printInstallSummary(result map[string]any) {
	if binary := asMap(result["binary"]); binary != nil {
		if errText, _ := binary["error"].(string); errText != "" {
			fmt.Println("uv-python-hook install failed: " + errText)
			return
		}
		action, _ := binary["action"].(string)
		destination, _ := binary["destination"].(string)
		fmt.Println("uv-python-hook binary " + installActionText(action) + ": " + destination)
		printPathSummary(asMap(binary["path"]))
		printWarnings(binary["warnings"])
	}

	targets := stringSliceFromAny(result["selected_targets"])
	switch len(targets) {
	case 0:
		fmt.Println("Hooks: no Codex or OpenCode targets detected.")
	default:
		fmt.Println("Hooks: installed for " + strings.Join(targets, ", ") + ".")
	}
	fmt.Println("Run with --debug for details.")
}

func installActionText(action string) string {
	switch action {
	case "installed":
		return "installed"
	case "updated":
		return "updated"
	case "unchanged", "current-executable-is-destination":
		return "already current"
	default:
		return action
	}
}

func printPathSummary(pathResult map[string]any) {
	if pathResult == nil {
		return
	}
	if enabled, _ := pathResult["enabled"].(bool); !enabled {
		fmt.Println("PATH: not changed (--no-path).")
		return
	}
	if errText, _ := pathResult["error"].(string); errText != "" {
		fmt.Println("PATH: update failed: " + errText)
		return
	}
	if changed, _ := pathResult["changed"].(bool); changed {
		fmt.Println("PATH: updated; open a new terminal if the command is still shadowed.")
		return
	}
	fmt.Println("PATH: already configured.")
}

func printWarnings(value any) {
	var warnings []string
	switch typed := value.(type) {
	case []string:
		warnings = typed
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				warnings = append(warnings, text)
			}
		}
	}
	for _, warning := range warnings {
		fmt.Println("Warning: " + warning)
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		var out []string
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func stripWrappingQuote(text string) string {
	if len(text) >= 2 && text[0] == text[len(text)-1] && (text[0] == '\'' || text[0] == '"') {
		return text[1 : len(text)-1]
	}
	return text
}

func defaultUVCacheDir() string {
	return filepath.Join(os.TempDir(), "uv-python-agent-hooks", "uv-cache")
}

func commandCacheDir() string {
	if configured := os.Getenv(hookCacheEnv); configured != "" {
		return configured
	}
	return defaultUVCacheDir()
}

func shouldUseHookCache(target, cacheMode string) bool {
	if cacheMode == "" {
		cacheMode = os.Getenv(hookCacheModeEnv)
	}
	switch strings.ToLower(strings.TrimSpace(cacheMode)) {
	case "1", "true", "yes", "on", "force", "forced":
		return true
	case "0", "false", "no", "off", "disabled", "disable":
		return false
	}
	return strings.ToLower(strings.TrimSpace(target)) != "opencode"
}

func shouldForceDotVenv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(hookForceDotVenvEnv))) {
	case "1", "true", "yes", "on", "force", "forced":
		return true
	default:
		return false
	}
}

func cacheEnv() []string {
	env := os.Environ()
	cacheDir := os.Getenv(hookCacheEnv)
	if cacheDir == "" {
		cacheDir = defaultUVCacheDir()
		env = append(env, hookCacheEnv+"="+cacheDir)
	}
	env = appendWithoutEnv(env, uvCacheEnv)
	env = append(env, uvCacheEnv+"="+cacheDir)
	_ = os.MkdirAll(cacheDir, 0o755)
	return env
}

func appendWithoutEnv(env []string, key string) []string {
	prefix := key + "="
	var out []string
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func doctor(cwd string) map[string]any {
	env := cacheEnv()
	uvPythonPath := commandOutput(env, "uv", "python", "find")
	return map[string]any{
		"uv": map[string]any{
			"path":    which("uv"),
			"version": commandOutput(env, "uv", "--version"),
		},
		"python": map[string]any{
			"path":    which("python"),
			"version": commandOutput(nil, "python", "--version"),
		},
		"uv_python": map[string]any{
			"path":      uvPythonPath,
			"available": uvPythonPath != nil,
		},
		"codex":           which("codex"),
		"opencode":        which("opencode"),
		"binary":          binaryInstallState(""),
		"project":         detectProject(cwd),
		"user_install":    newInstaller("user", cwd).installState(),
		"project_install": newInstaller("project", cwd).installState(),
	}
}

func which(name string) *string {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil
	}
	return &path
}

func commandOutput(env []string, name string, args ...string) *string {
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	text := strings.TrimSpace(string(output))
	return &text
}

func codexPretool(cwd string) int {
	payload := readJSONStdin()
	toolInput, _ := payload["tool_input"].(map[string]any)
	command, _ := toolInput["command"].(string)
	if command == "" {
		return 0
	}
	if cwd == "" {
		if value, ok := payload["cwd"].(string); ok {
			cwd = value
		}
	}
	result := rewriteCommandWithOptions(rewriteOptions{
		command: command,
		cwd:     cwd,
		target:  "codex",
	})
	if !result.Changed {
		return 0
	}
	eventName, _ := payload["hook_event_name"].(string)
	if eventName == "" {
		eventName = "PreToolUse"
	}
	message := "Python-related command must run through uv. Use: " + result.Command
	if eventName == "PermissionRequest" {
		printJSON(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": "PermissionRequest",
				"decision": map[string]any{
					"behavior": "deny",
					"message":  message,
				},
			},
		})
		return 0
	}
	printJSON(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": message,
		},
	})
	return 0
}

func cleanPath(path string) string {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "."
		}
		path = cwd
	}
	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		path = filepath.Dir(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return path
}

func isWindowsShell(shell string) bool {
	if shell != "" {
		lowered := strings.ToLower(shell)
		return strings.Contains(lowered, "powershell") || strings.HasSuffix(lowered, "cmd") || strings.Contains(lowered, "cmd.exe")
	}
	return runtime.GOOS == "windows"
}

func stringPtr(value string) *string {
	return &value
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ensureParent(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
}

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeJSONFile(path string, payload any) {
	ensureParent(path)
	data, _ := json.MarshalIndent(payload, "", "  ")
	data = append(data, '\n')
	_ = os.WriteFile(path, data, 0o644)
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

func backupInvalidJSON(path string) {
	backup := path + ".bak"
	if fileExists(path) && !fileExists(backup) {
		_ = copyFile(path, backup)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func asMap(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func asSlice(value any) []any {
	s, _ := value.([]any)
	return s
}

func commandError(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return strings.TrimSpace(string(exitErr.Stderr))
	}
	return err.Error()
}
