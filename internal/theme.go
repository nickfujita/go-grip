package internal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// builtinThemes are the reserved theme names that do not resolve to a file on
// disk. Any other value is treated as the name of a custom theme.
var builtinThemes = map[string]bool{"auto": true, "light": true, "dark": true}

// themeConfig is the resolved theme for a server: either one of the built-in
// modes (auto/light/dark) or a custom stylesheet layered on a built-in base.
type themeConfig struct {
	mode       string // "auto" | "light" | "dark" | "custom"
	base       string // for custom mode: "light" | "dark" | "none"
	customPath string // filesystem path to the custom theme stylesheet
}

// themesDir returns the directory custom themes are loaded from:
// $XDG_CONFIG_HOME/go-grip/themes, falling back to ~/.config/go-grip/themes.
func themesDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "go-grip", "themes")
}

var baseDirectiveRegex = regexp.MustCompile(`(?i)^\s*/\*\s*go-grip-base:\s*(light|dark|none)\s*\*/`)

// parseBaseDirective reads the optional first-line directive
// `/* go-grip-base: light|dark|none */` from a custom theme, defaulting to
// "dark" when the directive is absent.
func parseBaseDirective(css []byte) string {
	firstLine := css
	if i := bytes.IndexByte(css, '\n'); i >= 0 {
		firstLine = css[:i]
	}
	if m := baseDirectiveRegex.FindSubmatch(firstLine); m != nil {
		return strings.ToLower(string(m[1]))
	}
	return "dark"
}

// availableThemes lists the custom theme names (files without the .css suffix)
// found in the themes directory, sorted for stable output.
func availableThemes(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".css") {
			names = append(names, strings.TrimSuffix(e.Name(), ".css"))
		}
	}
	sort.Strings(names)
	return names
}

// resolveTheme turns the raw --theme flag value into a themeConfig, reading the
// custom theme file (and its base directive) when the value is not a built-in
// name. A missing custom theme file is a clear error that lists the searched
// path and the theme names that are available.
func resolveTheme(theme string) (themeConfig, error) {
	if theme == "" {
		theme = "auto"
	}
	if builtinThemes[theme] {
		return themeConfig{mode: theme}, nil
	}

	dir := themesDir()
	path := filepath.Join(dir, theme+".css")
	css, err := os.ReadFile(path)
	if err != nil {
		hint := "none found"
		if available := availableThemes(dir); len(available) > 0 {
			hint = strings.Join(available, ", ")
		}
		return themeConfig{}, fmt.Errorf(
			"custom theme %q not found at %s (available themes: %s)",
			theme, path, hint,
		)
	}

	return themeConfig{
		mode:       "custom",
		base:       parseBaseDirective(css),
		customPath: path,
	}, nil
}
