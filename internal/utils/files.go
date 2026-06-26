package utils

import (
	"io"
	"os"
	"path/filepath"
)

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func BuildWorkDirs(base string) (map[string]string, error) {
	dirs := map[string]string{
		"base":          base,
		"master_data":   filepath.Join(base, "master", "db_master"),
		"master_config": filepath.Join(base, "master", "config"),
		"slave_data":    filepath.Join(base, "slave", "db_slave"),
		"slave_config":  filepath.Join(base, "slave", "config"),
	}

	for _, dir := range dirs {
		if err := EnsureDir(dir); err != nil {
			return nil, err
		}
	}

	return dirs, nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}
