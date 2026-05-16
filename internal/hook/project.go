package hook

import (
	"os"
	"path/filepath"
	"strings"
)

type pyprojectInfo struct {
	Sections             map[string]bool
	ProjectName          string
	ProjectVersion       string
	ProjectDynamic       []string
	ProjectDeps          bool
	ProjectOptionalDeps  bool
	DependencyGroups     bool
	ToolUVDevDeps        bool
	ToolUVSources        bool
	ToolPoetry           bool
	ToolPDM              bool
	ProjectSectionBroken bool
}

func detectProject(cwd string) projectDetection {
	start := cleanPath(cwd)
	for dir := start; ; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, "poetry.lock")) {
			return externalProject(start, dir, "poetry")
		}
		if fileExists(filepath.Join(dir, "pdm.lock")) {
			return externalProject(start, dir, "pdm")
		}
		pyproject := filepath.Join(dir, "pyproject.toml")
		if fileExists(pyproject) {
			if manager := externalManagerFromPyproject(pyproject); manager != "" {
				return externalProjectWithPyproject(start, dir, stringPtr(pyproject), manager)
			}
			reasons := uvSyncableReasons(pyproject)
			issues := projectIssues(pyproject)
			if len(issues) > 0 {
				reasons = nil
			}
			var suggestion *string
			if len(issues) > 0 {
				suggestion = stringPtr("Complete pyproject.toml [project] metadata, for example add project.name and project.version, or use [project].dynamic = ['version']; until then, requirements installs fall back to uv pip.")
			}
			return projectDetection{
				CWD:        start,
				Pyproject:  stringPtr(pyproject),
				UVLock:     optionalFile(filepath.Join(dir, "uv.lock")),
				Root:       stringPtr(dir),
				Manager:    "uv",
				Syncable:   len(reasons) > 0,
				Reasons:    reasons,
				Issues:     issues,
				Suggestion: suggestion,
			}
		}
		uvLock := filepath.Join(dir, "uv.lock")
		if fileExists(uvLock) {
			return projectDetection{
				CWD:      start,
				UVLock:   stringPtr(uvLock),
				Root:     stringPtr(dir),
				Manager:  "uv",
				Syncable: true,
				Reasons:  []string{"uv.lock"},
				Issues:   []string{},
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return projectDetection{
		CWD:     start,
		Reasons: []string{},
		Issues:  []string{},
	}
}

func externalProject(start, dir, manager string) projectDetection {
	return externalProjectWithPyproject(start, dir, optionalFile(filepath.Join(dir, "pyproject.toml")), manager)
}

func externalProjectWithPyproject(start, dir string, pyproject *string, manager string) projectDetection {
	suggestion := "This project appears to be managed by " + manager + "; uv command rewriting is disabled for this project."
	return projectDetection{
		CWD:        start,
		Pyproject:  pyproject,
		Root:       stringPtr(dir),
		Manager:    manager,
		Syncable:   false,
		Reasons:    []string{manager},
		Issues:     []string{},
		Suggestion: stringPtr(suggestion),
	}
}

func externalManagerFromPyproject(pyproject string) string {
	info, err := parsePyproject(pyproject)
	if err != nil {
		return ""
	}
	if info.ToolPoetry {
		return "poetry"
	}
	if info.ToolPDM {
		return "pdm"
	}
	return ""
}

func optionalFile(path string) *string {
	if fileExists(path) {
		return stringPtr(path)
	}
	return nil
}

func uvSyncableReasons(pyproject string) []string {
	info, err := parsePyproject(pyproject)
	if err != nil {
		return nil
	}
	var reasons []string
	if info.ProjectDeps {
		reasons = append(reasons, "project.dependencies")
	}
	if info.ProjectOptionalDeps {
		reasons = append(reasons, "project.optional-dependencies")
	}
	if info.DependencyGroups {
		reasons = append(reasons, "dependency-groups")
	}
	if info.ToolUVDevDeps {
		reasons = append(reasons, "tool.uv.dev-dependencies")
	}
	if info.ToolUVSources && len(reasons) > 0 {
		reasons = append(reasons, "tool.uv.sources")
	}
	if fileExists(filepath.Join(filepath.Dir(pyproject), "uv.lock")) && hasProjectIdentity(info) {
		reasons = append(reasons, "uv.lock")
	}
	return reasons
}

func projectIssues(pyproject string) []string {
	info, err := parsePyproject(pyproject)
	if err != nil {
		if strings.Contains(err.Error(), "parse") {
			return []string{"cannot parse pyproject.toml: " + err.Error()}
		}
		return []string{"cannot read pyproject.toml: " + err.Error()}
	}
	if !info.Sections["project"] {
		return nil
	}
	if info.ProjectSectionBroken {
		return []string{"[project] must be a TOML table"}
	}
	var issues []string
	if strings.TrimSpace(info.ProjectName) == "" {
		issues = append(issues, "[project].name is required for uv sync")
	}
	if strings.TrimSpace(info.ProjectVersion) == "" && !contains(info.ProjectDynamic, "version") {
		issues = append(issues, "[project].version is required unless [project].dynamic includes 'version'")
	}
	return issues
}

func hasProjectIdentity(info pyprojectInfo) bool {
	return strings.TrimSpace(info.ProjectName) != "" || info.Sections["tool.uv"]
}

func parsePyproject(path string) (pyprojectInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pyprojectInfo{}, err
	}
	info := pyprojectInfo{Sections: map[string]bool{}}
	section := ""
	lines := strings.Split(string(data), "\n")
	for _, rawLine := range lines {
		line := stripTomlComment(rawLine)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			info.Sections[section] = true
			if section == "tool.poetry" || strings.HasPrefix(section, "tool.poetry.") {
				info.ToolPoetry = true
			}
			if section == "tool.pdm" || strings.HasPrefix(section, "tool.pdm.") {
				info.ToolPDM = true
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch section {
		case "project":
			switch key {
			case "name":
				info.ProjectName = parseTomlString(value)
			case "version":
				info.ProjectVersion = parseTomlString(value)
			case "dynamic":
				info.ProjectDynamic = parseTomlStringArray(value)
			case "dependencies":
				info.ProjectDeps = tomlValueNonEmpty(value)
			case "optional-dependencies":
				info.ProjectOptionalDeps = tomlValueNonEmpty(value)
			}
		case "tool.uv":
			switch key {
			case "dev-dependencies":
				info.ToolUVDevDeps = tomlValueNonEmpty(value)
			case "sources":
				info.ToolUVSources = tomlValueNonEmpty(value)
			}
		default:
			if section == "tool.poetry" || strings.HasPrefix(section, "tool.poetry.") {
				info.ToolPoetry = true
			}
			if section == "tool.pdm" || strings.HasPrefix(section, "tool.pdm.") {
				info.ToolPDM = true
			}
			if section == "dependency-groups" || strings.HasPrefix(section, "dependency-groups.") {
				if tomlValueNonEmpty(value) {
					info.DependencyGroups = true
				}
			}
			if strings.HasPrefix(section, "project.optional-dependencies") && tomlValueNonEmpty(value) {
				info.ProjectOptionalDeps = true
			}
		}
	}
	return info, nil
}

func stripTomlComment(line string) string {
	var quote rune
	for i, ch := range line {
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
		if ch == '#' {
			return line[:i]
		}
	}
	return line
}

func parseTomlString(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	return value
}

func parseTomlStringArray(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if body == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(body, ",") {
		item := strings.Trim(strings.TrimSpace(part), "\"'")
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func tomlValueNonEmpty(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" || value == "{}" {
		return false
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
		return body != ""
	}
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}"))
		return body != ""
	}
	return true
}
