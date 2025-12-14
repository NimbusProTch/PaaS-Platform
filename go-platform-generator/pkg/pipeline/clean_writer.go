package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanWriter writes to both Kratix default path and clean structure
type CleanWriter struct {
	kratixDir string
	cleanBase string
	tenant    string
	env       string
}

func NewCleanWriter(kratixDir, tenant, env string) *CleanWriter {
	return &CleanWriter{
		kratixDir: kratixDir,
		cleanBase: "/kratix/output/../../../../../manifests/voltron",
		tenant:    tenant,
		env:       env,
	}
}

func (w *CleanWriter) WriteFile(relativePath string, content []byte) error {
	// 1. Write to Kratix default location
	kratixPath := filepath.Join(w.kratixDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(kratixPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(kratixPath, content, 0644); err != nil {
		return err
	}

	// 2. Also write to clean structure
	cleanPath := filepath.Join(w.cleanBase, fmt.Sprintf("%s-%s", w.tenant, w.env), relativePath)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
		// Log but don't fail - Kratix might block this
		fmt.Printf("Warning: Could not create clean path: %v\n", err)
		return nil
	}
	
	if err := os.WriteFile(cleanPath, content, 0644); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: Could not write to clean path: %v\n", err)
	}

	return nil
}

func (w *CleanWriter) CopyDirectory(srcDir, destRelative string) error {
	// Copy to both locations
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(srcDir, path)
		if relPath == "." {
			return nil
		}

		targetRelative := filepath.Join(destRelative, relPath)

		if info.IsDir() {
			// Create directories
			kratixPath := filepath.Join(w.kratixDir, targetRelative)
			os.MkdirAll(kratixPath, 0755)

			cleanPath := filepath.Join(w.cleanBase, fmt.Sprintf("%s-%s", w.tenant, w.env), targetRelative)
			os.MkdirAll(cleanPath, 0755)
		} else {
			// Copy file
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return w.WriteFile(targetRelative, content)
		}
		
		return nil
	})
}