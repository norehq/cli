package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	Date      string `json:"date,omitempty"`
	Dirty     bool   `json:"dirty"`
	GoVersion string `json:"go,omitempty"`
}

func Read() Info {
	result := Info{
		Name:      "nore",
		Version:   normalizeVersion(Version),
		Commit:    strings.TrimSpace(Commit),
		Date:      strings.TrimSpace(Date),
		GoVersion: runtime.Version(),
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		result.Dirty = strings.HasSuffix(result.Version, "+dirty")
		return result
	}
	if build.GoVersion != "" {
		result.GoVersion = build.GoVersion
	}
	if result.Version == "dev" {
		result.Version = normalizeVersion(build.Main.Version)
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if result.Commit == "" {
				result.Commit = setting.Value
			}
		case "vcs.time":
			if result.Date == "" {
				result.Date = setting.Value
			}
		case "vcs.modified":
			result.Dirty = setting.Value == "true"
		}
	}
	if strings.HasSuffix(result.Version, "+dirty") {
		result.Dirty = true
	}
	return result
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "(devel)", "devel", "dev":
		return "dev"
	default:
		return strings.TrimPrefix(value, "v")
	}
}
