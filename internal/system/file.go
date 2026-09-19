package system

import (
	"os"
	"path/filepath"
	"strings"
)

// FileExists determines if the path given by `filename` exists.
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// FileNameWithoutExt returns the filename without its extension.
func FileNameWithoutExt(fileName string) string {
	base := filepath.Base(fileName)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ReplaceExt replaces the extension of `fp` with `ext` if the extension of
// `fp` is in `formats`.
//
// This is used in places where we need to normalize file extensions (e.g.,
// `foo.mdx` -> `foo.md`) in order to respect format associations.
