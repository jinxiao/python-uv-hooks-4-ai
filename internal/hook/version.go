package hook

import "strings"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func VersionString() string {
	text := "uv-python-hook " + Version
	var meta []string
	if Commit != "" && Commit != "none" {
		meta = append(meta, "commit="+Commit)
	}
	if Date != "" && Date != "unknown" {
		meta = append(meta, "date="+Date)
	}
	if len(meta) == 0 {
		return text
	}
	return text + " (" + strings.Join(meta, ", ") + ")"
}
