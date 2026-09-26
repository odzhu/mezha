package app

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
)

//go:embed templates/mezha.toml
var defaultMezhaToml string

//go:embed templates/provision
var provisionTemplatesFS embed.FS

type provisionFileEntry struct {
	relPath string
	content string
}

func defaultProvisionFS() fs.FS {
	sub, err := fs.Sub(provisionTemplatesFS, "templates/provision")
	if err != nil {
		panic(err)
	}
	return sub
}

func defaultProvisionFiles() ([]provisionFileEntry, error) {
	var files []provisionFileEntry
	sub := defaultProvisionFS()
	err := fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		files = append(files, provisionFileEntry{
			relPath: filepath.FromSlash(p),
			content: string(content),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load provision templates: %w", err)
	}
	return files, nil
}
