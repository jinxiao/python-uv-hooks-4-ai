package hook

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	interpreterCommands       = []string{"python", "python3", "py"}
	pipCommands               = []string{"pip", "pip3"}
	pythonToolCommands        = []string{"pytest", "ruff", "mypy", "black", "isort", "coverage", "tox", "nox", "pyright", "pylint", "flake8", "ty"}
	venvCommands              = []string{"virtualenv"}
	windowsExecutableSuffixes = []string{".exe", ".cmd", ".bat"}
	python3Pattern            = regexp.MustCompile(`^python3(?:\.\d+)?$`)
	pythonPattern             = regexp.MustCompile(`^python(?:\d+(?:\.\d+)?)?$`)
)

func rewriteCommand(command, cwd, shell string) rewriteResult {
	return rewriteCommandWithOptions(rewriteOptions{
		command: command,
		cwd:     cwd,
		shell:   shell,
	})
}

func rewriteCommandWithOptions(opts rewriteOptions) rewriteResult {
	useHookCache := shouldUseHookCache(opts.target, opts.cacheMode)
	command := opts.command
	project := detectProject(opts.cwd)
	parts := splitShell(command)
	var changed bool
	var reasons []string
	var rewritten strings.Builder
	for _, part := range parts {
		if part.kind == shellPartCommand {
			newText, reason := rewriteSimpleCommand(part.text, project, opts.shell, useHookCache)
			rewritten.WriteString(newText)
			if reason != "" {
				changed = true
				if !contains(reasons, reason) {
					reasons = append(reasons, reason)
				}
			}
		} else {
			rewritten.WriteString(part.text)
		}
	}
	var reason *string
	if len(reasons) > 0 {
		joined := strings.Join(reasons, "; ")
		reason = &joined
	}
	return rewriteResult{
		Original: command,
		Command:  rewritten.String(),
		Changed:  changed,
		Reason:   reason,
		Project:  project,
	}
}

func rewriteSimpleCommand(segment string, project projectDetection, shell string, useHookCache bool) (string, string) {
	leadingLen := len(segment) - len(strings.TrimLeftFunc(segment, unicode.IsSpace))
	trailingLen := len(segment) - len(strings.TrimRightFunc(segment, unicode.IsSpace))
	leading := segment[:leadingLen]
	trailing := segment[len(segment)-trailingLen:]
	body := strings.TrimSpace(segment)
	if body == "" {
		return segment, ""
	}
	commandBody := stripPowerShellCallOperator(body)
	if commandBody == "" {
		commandBody = body
	}
	first, rest := firstTokenAndRest(commandBody)
	if first == "" {
		return segment, ""
	}
	canonical := canonicalCommand(first)
	if canonical == "uv" || canonical == "uvx" {
		return segment, ""
	}
	if projectIsManagedByOtherTool(project) {
		return segment, ""
	}
	uv := uvShellPrefix(shell, useHookCache)
	if contains(interpreterCommands, canonical) {
		args := splitArgs(rest)
		if venvArgs, ok := interpreterVenvArgs(args); ok {
			return leading + uv + " venv " + commandToShellText(venvArgsWithDefault(venvArgs), shell) + trailing, first + " -m venv -> uv venv"
		}
		if pipArgs, ok := interpreterPipArgs(args); ok {
			if isRequirementsInstall(pipArgs) && project.Syncable {
				return leading + uv + " sync" + trailing, "python -m pip install -r -> uv sync because pyproject.toml is uv-syncable"
			}
			if isRequirementsInstall(pipArgs) && len(project.Issues) > 0 {
				return leading + uv + " pip " + commandToShellText(pipArgs, shell) + trailing, "python -m pip install -r -> uv pip because pyproject.toml [project] is incomplete; " + deref(project.Suggestion)
			}
			return leading + uv + " pip " + commandToShellText(pipArgs, shell) + trailing, first + " -m pip -> uv pip"
		}
		if scriptArgs, ok := interpreterScriptArgs(args); ok {
			return leading + uv + " run " + commandToShellText(scriptArgs, shell) + trailing, first + " script -> uv run script"
		}
		return leading + uv + " run python" + rest + trailing, first + " -> uv run python"
	}
	if contains(pipCommands, canonical) {
		args := splitArgs(rest)
		if isRequirementsInstall(args) && project.Syncable {
			return leading + uv + " sync" + trailing, "pip install -r -> uv sync because pyproject.toml is uv-syncable"
		}
		if isRequirementsInstall(args) && len(project.Issues) > 0 {
			return leading + uv + " pip" + rest + trailing, "pip install -r -> uv pip because pyproject.toml [project] is incomplete; " + deref(project.Suggestion)
		}
		return leading + uv + " pip" + rest + trailing, first + " -> uv pip"
	}
	if contains(pythonToolCommands, canonical) {
		if !projectUsesUV(project) {
			toolRunner := uvToolRunPrefix(shell, useHookCache)
			toolRunnerName := "uvx"
			if useHookCache {
				toolRunnerName = "uv tool run"
			}
			return leading + toolRunner + " " + canonical + rest + trailing, first + " -> " + toolRunnerName + " " + canonical
		}
		return leading + uv + " run " + canonical + rest + trailing, first + " -> uv run " + canonical
	}
	if contains(venvCommands, canonical) {
		args := splitArgs(rest)
		return leading + uv + " venv " + commandToShellText(venvArgsWithDefault(args), shell) + trailing, first + " -> uv venv"
	}
	return segment, ""
}

func uvShellPrefix(shell string, useHookCache bool) string {
	argv := []string{"uv"}
	if useHookCache {
		argv = append(argv, "--cache-dir", commandCacheDir())
	}
	return commandToShellText(argv, shell)
}

func uvToolRunPrefix(shell string, useHookCache bool) string {
	if !useHookCache {
		return "uvx"
	}
	return commandToShellText([]string{"uv", "--cache-dir", commandCacheDir(), "tool", "run"}, shell)
}

func commandToShellText(argv []string, shell string) string {
	if isWindowsShell(shell) {
		return list2cmdline(argv)
	}
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg != "" {
		allSafe := true
		for _, ch := range arg {
			if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || strings.ContainsRune("@%_+=:,./-", ch)) {
				allSafe = false
				break
			}
		}
		if allSafe {
			return arg
		}
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func list2cmdline(argv []string) string {
	var parts []string
	for _, arg := range argv {
		if arg == "" || strings.ContainsAny(arg, " \t\"") {
			var b strings.Builder
			b.WriteByte('"')
			backslashes := 0
			for _, ch := range arg {
				switch ch {
				case '\\':
					backslashes++
				case '"':
					b.WriteString(strings.Repeat("\\", backslashes*2+1))
					b.WriteRune('"')
					backslashes = 0
				default:
					b.WriteString(strings.Repeat("\\", backslashes))
					b.WriteRune(ch)
					backslashes = 0
				}
			}
			b.WriteString(strings.Repeat("\\", backslashes*2))
			b.WriteByte('"')
			parts = append(parts, b.String())
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func canonicalCommand(command string) string {
	name := strings.Trim(strings.TrimSpace(command), "\"'")
	name = strings.ReplaceAll(name, "\\", "/")
	lower := strings.ToLower(filepath.Base(name))
	for _, suffix := range windowsExecutableSuffixes {
		if strings.HasSuffix(lower, suffix) {
			lower = strings.TrimSuffix(lower, suffix)
			break
		}
	}
	if python3Pattern.MatchString(lower) {
		return "python3"
	}
	if pythonPattern.MatchString(lower) {
		if lower == "python3" {
			return "python3"
		}
		return "python"
	}
	return lower
}

func stripPowerShellCallOperator(command string) string {
	rest, ok := strings.CutPrefix(command, "&")
	if !ok || rest == "" {
		return ""
	}
	if len(rest) > 0 && unicode.IsSpace(rune(rest[0])) {
		return strings.TrimLeftFunc(rest, unicode.IsSpace)
	}
	return ""
}

func firstTokenAndRest(command string) (string, string) {
	var quote rune
	for i, ch := range command {
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if unicode.IsSpace(ch) {
			return command[:i], command[i:]
		}
	}
	return command, ""
}

func splitArgs(rest string) []string {
	text := strings.TrimSpace(rest)
	if text == "" {
		return nil
	}
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, ch := range text {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				current.WriteRune(ch)
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if unicode.IsSpace(ch) {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(ch)
	}
	if escaped {
		current.WriteByte('\\')
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	if len(args) == 0 {
		return strings.Fields(text)
	}
	return args
}

func isRequirementsInstall(args []string) bool {
	if len(args) == 0 || strings.ToLower(args[0]) != "install" {
		return false
	}
	for i, arg := range args[1:] {
		lowered := strings.Trim(strings.ToLower(arg), "\"'")
		if (lowered == "-r" || lowered == "--requirement") && i+1 < len(args[1:]) {
			return true
		}
		if strings.HasPrefix(lowered, "-r") && len(lowered) > 2 {
			return true
		}
		if strings.HasPrefix(lowered, "--requirement=") {
			return true
		}
	}
	return false
}

func interpreterPipArgs(args []string) ([]string, bool) {
	if len(args) >= 2 && args[0] == "-m" && (args[1] == "pip" || args[1] == "pip3") {
		return args[2:], true
	}
	return nil, false
}

func interpreterVenvArgs(args []string) ([]string, bool) {
	if len(args) >= 2 && args[0] == "-m" && (args[1] == "venv" || args[1] == "virtualenv") {
		return args[2:], true
	}
	return nil, false
}

func interpreterScriptArgs(args []string) ([]string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return nil, false
	}
	lowered := strings.ToLower(strings.Trim(args[0], "\"'"))
	if strings.HasSuffix(lowered, ".py") || strings.HasSuffix(lowered, ".pyw") {
		return args, true
	}
	return nil, false
}

func projectIsManagedByOtherTool(project projectDetection) bool {
	return project.Manager == "poetry" || project.Manager == "pdm"
}

func projectUsesUV(project projectDetection) bool {
	return project.Manager == "uv" || project.Pyproject != nil || project.UVLock != nil
}

func venvArgsWithDefault(args []string) []string {
	if hasVenvPathArg(args) {
		return args
	}
	out := append([]string{}, args...)
	return append(out, ".venv")
}

func hasVenvPathArg(args []string) bool {
	valueOptions := []string{"-p", "--python", "--prompt", "--index", "--default-index", "-i", "--index-url", "--find-links", "-f", "--cache-dir", "--config-file"}
	for i := 0; i < len(args); {
		arg := args[i]
		if arg == "--" {
			return i+1 < len(args)
		}
		if contains(valueOptions, arg) {
			i += 2
			continue
		}
		hasInlineValue := false
		for _, option := range valueOptions {
			if strings.HasPrefix(option, "--") && strings.HasPrefix(arg, option+"=") {
				hasInlineValue = true
				break
			}
		}
		if hasInlineValue || strings.HasPrefix(arg, "-") {
			i++
			continue
		}
		return true
	}
	return false
}

type shellPartKind int

const (
	shellPartCommand shellPartKind = iota
	shellPartOperator
)

type shellPart struct {
	kind shellPartKind
	text string
}

func splitShell(command string) []shellPart {
	var parts []shellPart
	start := 0
	var quote rune
	for i := 0; i < len(command); {
		ch := rune(command[i])
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			i++
			continue
		}
		op := ""
		if strings.HasPrefix(command[i:], "&&") || strings.HasPrefix(command[i:], "||") {
			op = command[i : i+2]
		} else if ch == ';' || ch == '|' || ch == '\n' {
			op = command[i : i+1]
		}
		if op != "" {
			if start < i {
				parts = append(parts, shellPart{kind: shellPartCommand, text: command[start:i]})
			}
			parts = append(parts, shellPart{kind: shellPartOperator, text: op})
			i += len(op)
			start = i
			continue
		}
		i++
	}
	if start < len(command) {
		parts = append(parts, shellPart{kind: shellPartCommand, text: command[start:]})
	}
	if len(parts) == 0 {
		parts = append(parts, shellPart{kind: shellPartCommand, text: command})
	}
	return parts
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
