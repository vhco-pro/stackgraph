// Package icons provides embedded cloud provider icon assets.
package icons

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:icons
var embeddedIcons embed.FS

// GetIconBytes returns the raw bytes of an icon by its mapping path (e.g., "aws/compute/ec2.svg").
func GetIconBytes(iconPath string) ([]byte, error) {
	return embeddedIcons.ReadFile("icons/" + iconPath)
}

// WriteIconToTemp writes an embedded icon to a temp file and returns the path.
// Caller is responsible for cleanup.
func WriteIconToTemp(iconPath string) (string, error) {
	data, err := GetIconBytes(iconPath)
	if err != nil {
		return "", err
	}

	ext := filepath.Ext(iconPath)
	f, err := os.CreateTemp("", "stackgraph-icon-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

// ListIcons returns all icon paths in the embedded filesystem.
func ListIcons() ([]string, error) {
	var paths []string
	err := fs.WalkDir(embeddedIcons, "icons", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			// Strip "icons/" prefix
			paths = append(paths, path[6:])
		}
		return nil
	})
	return paths, err
}
