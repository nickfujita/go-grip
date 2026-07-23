package internal

import (
	"path/filepath"
	"strings"
)

// DefaultIgnores is the watcher exclusion list applied when --ignore is not
// given: hidden directories plus common dependency and cache trees. Ignored
// directories are still served; they are only excluded from the live-reload
// file watcher, which otherwise registers an inotify watch per directory and
// can exhaust the kernel limit when serving a large tree.
var DefaultIgnores = []string{".*", "node_modules", "vendor", "venv", "__pycache__"}

// NewIgnoreMatcher returns a predicate telling the file watcher which paths to
// skip, given the directory being served and a list of patterns. A pattern
// without a path separator is matched (filepath.Match) against every element
// of the path relative to root, so "node_modules" excludes such directories at
// any depth. A pattern containing a separator is matched against the whole
// root-relative path and also excludes everything below a matched directory.
// The serve root itself and paths outside it are never ignored.
func NewIgnoreMatcher(root string, patterns []string) func(string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	var names, paths []string
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if strings.ContainsRune(p, filepath.Separator) {
			paths = append(paths, strings.TrimSuffix(p, string(filepath.Separator)))
		} else {
			names = append(names, p)
		}
	}
	return func(dir string) bool {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(absRoot, abs)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return false
		}
		for _, elem := range strings.Split(rel, string(filepath.Separator)) {
			for _, pat := range names {
				if ok, _ := filepath.Match(pat, elem); ok {
					return true
				}
			}
		}
		for _, pat := range paths {
			for r := rel; r != "." && r != string(filepath.Separator); r = filepath.Dir(r) {
				if ok, _ := filepath.Match(pat, r); ok {
					return true
				}
			}
		}
		return false
	}
}
