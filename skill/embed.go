package skill

import (
	"embed"
	"io/fs"
)

const (
	Name    = "nore"
	Version = "1"
)

//go:embed nore
var embedded embed.FS

func Bundle() (fs.FS, error) {
	return fs.Sub(embedded, Name)
}

func Text() (string, error) {
	payload, err := embedded.ReadFile(Name + "/SKILL.md")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
