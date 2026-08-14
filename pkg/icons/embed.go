// Package icons provides embedded cloud provider icon assets.
package icons

import (
	"embed"
	"io/fs"
)

//go:embed all:icons
var embeddedIcons embed.FS

// GetIconBytes returns the raw bytes of an icon by its mapping path (e.g., "aws/compute/ec2.svg").
func GetIconBytes(iconPath string) ([]byte, error) {
	return embeddedIcons.ReadFile("icons/" + iconPath)
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
