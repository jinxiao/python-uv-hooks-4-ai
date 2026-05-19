package hook

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const pathProfileMarker = "uv-python-hook install-bin"

func installBinary(opts installBinaryOptions) map[string]any {
	source, err := currentExecutablePath()
	if err != nil {
		return map[string]any{
			"changed": false,
			"error":   err.Error(),
		}
	}

	installDir := opts.dir
	if installDir == "" {
		installDir = defaultBinaryInstallDir()
	}
	installDir = cleanPath(installDir)
	destination := filepath.Join(installDir, binaryInstallName())
	detectedBefore := detectInstalledBinaries(source, destination)
	destinationExisted := fileExists(destination)
	destinationVersionBefore := commandOutput(nil, destination, "--version")
	result := map[string]any{
		"source":                     source,
		"source_version":             VersionString(),
		"destination":                destination,
		"destination_exists_before":  destinationExisted,
		"destination_version_before": destinationVersionBefore,
		"install_dir":                installDir,
		"detected_binaries":          detectedBefore,
		"action":                     "pending",
		"changed":                    false,
	}

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		result["error"] = err.Error()
		return result
	}

	if samePath(source, destination) {
		result["already_installed"] = true
		result["action"] = "current-executable-is-destination"
	} else if sameFileContent(source, destination) {
		result["already_installed"] = true
		result["action"] = "unchanged"
	} else if err := copyExecutable(source, destination); err != nil {
		result["error"] = err.Error()
		return result
	} else {
		result["changed"] = true
		if destinationExisted {
			result["action"] = "updated"
		} else {
			result["action"] = "installed"
		}
	}
	result["destination_version_after"] = commandOutput(nil, destination, "--version")

	processPathHasDir := dirInPath(installDir)
	userPathHasDirBefore := userPathHasDir(installDir)
	pathResolvesToDestinationBefore := pathResolvesTo(destination)
	pathResult := map[string]any{
		"enabled":                         opts.updatePath,
		"process_path_has_dir":            processPathHasDir,
		"path_resolves_to_destination":    pathResolvesToDestinationBefore,
		"path_resolved_binary":            resolvedBinaryPath(),
		"user_path_has_dir":               userPathHasDirBefore,
		"persistent_path_changed":         false,
		"process_path_precedence_changed": false,
		"changed":                         false,
	}
	if opts.updatePath && (!pathResolvesToDestinationBefore || !userPathHasDirBefore) {
		persistentChanged, method, target, err := ensureInstallDirOnPath(installDir)
		pathResult["persistent_path_changed"] = persistentChanged
		pathResult["method"] = method
		if target != "" {
			pathResult["target"] = target
		}
		if err != nil {
			pathResult["error"] = err.Error()
		} else {
			if !pathResolvesToDestinationBefore {
				pathResult["process_path_precedence_changed"] = true
				ensureDirFirstInProcessPath(installDir)
			}
			pathResult["needs_new_shell"] = persistentChanged || !pathResolvesToDestinationBefore
			pathResult["changed"] = persistentChanged || !pathResolvesToDestinationBefore
		}
	}
	pathResult["process_path_has_dir_after"] = dirInPath(installDir)
	pathResult["path_resolves_to_destination_after"] = pathResolvesTo(destination)
	pathResult["path_resolved_binary_after"] = resolvedBinaryPath()
	pathResult["user_path_has_dir_after"] = userPathHasDir(installDir)
	result["path"] = pathResult
	result["warnings"] = binaryInstallWarnings(destination, installDir)
	return result
}

func binaryInstallState(dir string) map[string]any {
	source, err := currentExecutablePath()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	if dir == "" {
		dir = defaultBinaryInstallDir()
	}
	installDir := cleanPath(dir)
	destination := filepath.Join(installDir, binaryInstallName())
	return map[string]any{
		"source":            source,
		"default_dir":       defaultBinaryInstallDir(),
		"install_dir":       installDir,
		"destination":       destination,
		"detected_binaries": detectInstalledBinaries(source, destination),
		"path": map[string]any{
			"process_path_has_dir": dirInPath(installDir),
			"user_path_has_dir":    userPathHasDir(installDir),
		},
	}
}

func currentExecutablePath() (string, error) {
	source, err := os.Executable()
	if err != nil {
		return "", err
	}
	return cleanFilePath(source), nil
}

func detectInstalledBinaries(source, destination string) []map[string]any {
	var candidates []map[string]any
	addCandidate := func(kind, path string, onPath bool) {
		if path == "" {
			return
		}
		path = cleanFilePath(path)
		for _, candidate := range candidates {
			if existing, _ := candidate["path"].(string); samePath(existing, path) {
				candidate["kind"] = candidate["kind"].(string) + "," + kind
				if onPath {
					candidate["on_path"] = true
				}
				return
			}
		}
		exists := fileExists(path)
		candidate := map[string]any{
			"kind":                 kind,
			"path":                 path,
			"exists":               exists,
			"on_path":              onPath,
			"same_as_source":       samePath(source, path),
			"same_content":         exists && sameFileContent(source, path),
			"selected_destination": samePath(destination, path),
		}
		if exists {
			candidate["version"] = commandOutput(nil, path, "--version")
		}
		candidates = append(candidates, candidate)
	}

	addCandidate("current-executable", source, false)
	if path, err := exec.LookPath("uv-python-hook"); err == nil {
		addCandidate("path", path, true)
	}
	addCandidate("default-install", filepath.Join(defaultBinaryInstallDir(), binaryInstallName()), false)
	addCandidate("selected-destination", destination, false)
	return candidates
}

func binaryInstallWarnings(destination, installDir string) []string {
	var warnings []string
	if path, err := exec.LookPath("uv-python-hook"); err == nil && !samePath(path, destination) {
		warnings = append(warnings, "uv-python-hook on PATH resolves to "+path+" instead of installed binary "+destination+"; open a new shell or move "+installDir+" earlier in PATH.")
	}
	return warnings
}

func defaultBinaryInstallDir() string {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "Programs", "uv-python-hook")
		}
		return filepath.Join(homeDir(), "AppData", "Local", "Programs", "uv-python-hook")
	}
	return filepath.Join(homeDir(), ".local", "bin")
}

func binaryInstallName() string {
	if runtime.GOOS == "windows" {
		return "uv-python-hook.exe"
	}
	return "uv-python-hook"
}

func copyExecutable(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o755)
}

func sameFileContent(left, right string) bool {
	leftData, leftErr := os.ReadFile(left)
	if leftErr != nil {
		return false
	}
	rightData, rightErr := os.ReadFile(right)
	if rightErr != nil {
		return false
	}
	return bytes.Equal(leftData, rightData)
}

func dirInPath(dir string) bool {
	cleanDir := normalizeComparablePath(dir)
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if normalizeComparablePath(item) == cleanDir {
			return true
		}
	}
	return false
}

func userPathHasDir(dir string) bool {
	if runtime.GOOS == "windows" {
		userPath := windowsUserPath()
		return pathListHasDir(userPath, dir)
	}
	return shellProfileHasDir(dir)
}

func pathListHasDir(list, dir string) bool {
	cleanDir := normalizeComparablePath(dir)
	for _, item := range filepath.SplitList(list) {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if normalizeComparablePath(expandUserPath(item)) == cleanDir {
			return true
		}
	}
	return false
}

func resolvedBinaryPath() *string {
	path, err := exec.LookPath("uv-python-hook")
	if err != nil {
		return nil
	}
	path = cleanFilePath(path)
	return &path
}

func pathResolvesTo(path string) bool {
	resolved := resolvedBinaryPath()
	return resolved != nil && samePath(*resolved, path)
}

func ensureDirFirstInProcessPath(dir string) {
	current := os.Getenv("PATH")
	if current == "" {
		_ = os.Setenv("PATH", dir)
		return
	}
	cleanDir := normalizeComparablePath(dir)
	parts := []string{dir}
	for _, item := range filepath.SplitList(current) {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if normalizeComparablePath(item) == cleanDir {
			continue
		}
		parts = append(parts, item)
	}
	_ = os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}

func normalizeComparablePath(path string) string {
	path = expandUserPath(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func cleanFilePath(path string) string {
	path = expandUserPath(strings.TrimSpace(path))
	if path == "" {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return path
}

func samePath(left, right string) bool {
	return normalizeComparablePath(left) == normalizeComparablePath(right)
}

func ensureInstallDirOnPath(dir string) (bool, string, string, error) {
	if runtime.GOOS == "windows" {
		return ensureWindowsUserPath(dir)
	}
	return ensureShellProfilePath(dir)
}

func windowsUserPath() string {
	script := `[Environment]::GetEnvironmentVariable('Path', 'User')`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func ensureWindowsUserPath(dir string) (bool, string, string, error) {
	script := `$dir = $args[0]
$current = [Environment]::GetEnvironmentVariable('Path', 'User')
$normalizedDir = [System.IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables($dir)).TrimEnd('\')
$kept = New-Object System.Collections.Generic.List[string]
$seen = New-Object System.Collections.Generic.HashSet[string]([StringComparer]::OrdinalIgnoreCase)
[void]$seen.Add($normalizedDir)
foreach ($part in ($current -split ';' | Where-Object { $_ -ne '' })) {
  $expanded = [Environment]::ExpandEnvironmentVariables($part)
  try {
    $normalizedPart = [System.IO.Path]::GetFullPath($expanded).TrimEnd('\')
  } catch {
    $normalizedPart = $expanded.TrimEnd('\')
  }
  if ([String]::Equals($normalizedPart, $normalizedDir, [StringComparison]::OrdinalIgnoreCase)) {
    continue
  }
  if ($seen.Add($normalizedPart)) {
    $kept.Add($part)
  }
}
$items = @($dir) + $kept.ToArray()
$new = [String]::Join(';', $items)
if ([String]::Equals($current, $new, [StringComparison]::Ordinal)) {
  Write-Output 'false'
} else {
  [Environment]::SetEnvironmentVariable('Path', $new, 'User')
  Write-Output 'true'
}`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script, dir).Output()
	changed := strings.TrimSpace(string(output)) == "true"
	return changed, "windows-user-environment", "User PATH", err
}

func ensureShellProfilePath(dir string) (bool, string, string, error) {
	profile := shellProfilePath()
	if profile == "" {
		return false, "shell-profile", "", fmt.Errorf("unable to locate shell profile")
	}
	if shellProfileHasDir(dir) {
		return false, "shell-profile", profile, nil
	}
	if err := os.MkdirAll(filepath.Dir(profile), 0o755); err != nil {
		return false, "shell-profile", profile, err
	}
	entry := "\n# " + pathProfileMarker + "\n" +
		"case \":$PATH:\" in\n" +
		"  *" + shellSingleQuote(":"+dir+":") + "*) ;;\n" +
		"  *) export PATH=" + shellSingleQuote(dir) + ":\"$PATH\" ;;\n" +
		"esac\n"
	file, err := os.OpenFile(profile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, "shell-profile", profile, err
	}
	defer file.Close()
	if _, err := file.WriteString(entry); err != nil {
		return false, "shell-profile", profile, err
	}
	return true, "shell-profile", profile, nil
}

func shellProfileHasDir(dir string) bool {
	profile := shellProfilePath()
	data, err := os.ReadFile(profile)
	if err != nil {
		return false
	}
	text := string(data)
	if strings.Contains(text, dir) {
		return true
	}
	home := homeDir()
	if strings.HasPrefix(dir, home) {
		relative := strings.TrimPrefix(dir, home)
		relative = strings.TrimPrefix(filepath.ToSlash(relative), "/")
		patterns := []string{
			"$HOME/" + relative,
			"${HOME}/" + relative,
			"~/" + relative,
		}
		for _, pattern := range patterns {
			if strings.Contains(text, pattern) {
				return true
			}
		}
	}
	return false
}

func shellProfilePath() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir(), ".zprofile")
	default:
		return filepath.Join(homeDir(), ".profile")
	}
}

func shellSingleQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "'\"'\"'") + "'"
}

func expandUserPath(path string) string {
	if path == "" {
		return path
	}
	if runtime.GOOS == "windows" {
		path = expandWindowsEnv(path)
	}
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		return filepath.Join(homeDir(), path[2:])
	}
	if strings.HasPrefix(path, "$HOME/") {
		return filepath.Join(homeDir(), path[len("$HOME/"):])
	}
	if strings.HasPrefix(path, "${HOME}/") {
		return filepath.Join(homeDir(), path[len("${HOME}/"):])
	}
	return path
}

func expandWindowsEnv(path string) string {
	path = os.ExpandEnv(path)
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '%' {
			out.WriteByte(path[i])
			continue
		}
		end := strings.IndexByte(path[i+1:], '%')
		if end < 0 {
			out.WriteByte(path[i])
			continue
		}
		name := path[i+1 : i+1+end]
		value := os.Getenv(name)
		if value == "" {
			out.WriteString("%" + name + "%")
		} else {
			out.WriteString(value)
		}
		i += end + 1
	}
	return out.String()
}
