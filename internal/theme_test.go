package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveThemeBuiltins(t *testing.T) {
	for _, name := range []string{"", "auto", "light", "dark"} {
		cfg, err := resolveTheme(name)
		if err != nil {
			t.Fatalf("resolveTheme(%q): unexpected error %v", name, err)
		}
		want := name
		if want == "" {
			want = "auto"
		}
		if cfg.mode != want {
			t.Fatalf("resolveTheme(%q).mode = %q, want %q", name, cfg.mode, want)
		}
		if cfg.customPath != "" {
			t.Fatalf("resolveTheme(%q) should have no custom path, got %q", name, cfg.customPath)
		}
	}
}

func TestResolveThemeCustom(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	themes := filepath.Join(dir, "go-grip", "themes")
	if err := os.MkdirAll(themes, 0o755); err != nil {
		t.Fatalf("mkdir themes: %v", err)
	}
	themeFile := filepath.Join(themes, "midnight.css")
	if err := os.WriteFile(themeFile, []byte("/* go-grip-base: dark */\n.markdown-body{}\n"), 0o644); err != nil {
		t.Fatalf("write theme: %v", err)
	}

	cfg, err := resolveTheme("midnight")
	if err != nil {
		t.Fatalf("resolveTheme(midnight): %v", err)
	}
	if cfg.mode != "custom" {
		t.Fatalf("mode = %q, want custom", cfg.mode)
	}
	if cfg.base != "dark" {
		t.Fatalf("base = %q, want dark", cfg.base)
	}
	if cfg.customPath != themeFile {
		t.Fatalf("customPath = %q, want %q", cfg.customPath, themeFile)
	}
}

func TestResolveThemeBuiltinEmbedded(t *testing.T) {
	// A theme compiled into the binary resolves with no external file, even
	// when the on-disk themes directory is empty.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := resolveTheme("nightshade")
	if err != nil {
		t.Fatalf("resolveTheme(nightshade): %v", err)
	}
	if cfg.mode != "custom" {
		t.Fatalf("mode = %q, want custom", cfg.mode)
	}
	if cfg.base != "dark" {
		t.Fatalf("base = %q, want dark", cfg.base)
	}
	if cfg.customPath != "" {
		t.Fatalf("embedded theme should have no customPath, got %q", cfg.customPath)
	}
	if len(cfg.customContent) == 0 {
		t.Fatal("embedded theme should carry its stylesheet bytes")
	}
	if !strings.Contains(string(cfg.customContent), "markdown-body") {
		t.Fatalf("embedded nightshade content looks wrong: %q", cfg.customContent)
	}
}

func TestResolveThemeMissingListsEmbedded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := resolveTheme("does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing theme")
	}
	if !strings.Contains(err.Error(), "nightshade") {
		t.Fatalf("error should list embedded theme names, got: %s", err.Error())
	}
}

func TestServeCustomThemeEmbedded(t *testing.T) {
	server := NewServer("localhost", 6419, false, false, false, NewParser(), nil, "nightshade")
	cfg, err := resolveTheme("nightshade")
	if err != nil {
		t.Fatalf("resolveTheme(nightshade): %v", err)
	}
	server.resolvedTheme = cfg
	handler := server.newHandler(http.Dir(t.TempDir()))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/custom/theme.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/css") {
		t.Fatalf("expected text/css, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "markdown-body") {
		t.Fatalf("expected served embedded theme body, got %q", rec.Body.String())
	}
}

func TestResolveThemeMissingListsSearchPathAndAvailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	themes := filepath.Join(dir, "go-grip", "themes")
	if err := os.MkdirAll(themes, 0o755); err != nil {
		t.Fatalf("mkdir themes: %v", err)
	}
	for _, name := range []string{"aurora.css", "nightshade.css"} {
		if err := os.WriteFile(filepath.Join(themes, name), []byte(".x{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	_, err := resolveTheme("does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing theme")
	}
	msg := err.Error()
	if !strings.Contains(msg, filepath.Join(themes, "does-not-exist.css")) {
		t.Fatalf("error should list the searched path, got: %s", msg)
	}
	if !strings.Contains(msg, "aurora") || !strings.Contains(msg, "nightshade") {
		t.Fatalf("error should list available theme names, got: %s", msg)
	}
}

func TestParseBaseDirective(t *testing.T) {
	tests := []struct {
		name string
		css  string
		want string
	}{
		{"explicit dark", "/* go-grip-base: dark */\nbody{}", "dark"},
		{"explicit light", "/* go-grip-base: light */\nbody{}", "light"},
		{"explicit none", "/* go-grip-base: none */\nbody{}", "none"},
		{"case insensitive", "/* GO-GRIP-BASE: Light */\nbody{}", "light"},
		{"no directive defaults dark", "body{color:red}\n", "dark"},
		{"directive not on first line ignored", "body{}\n/* go-grip-base: light */\n", "dark"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseBaseDirective([]byte(tt.css)); got != tt.want {
				t.Fatalf("parseBaseDirective() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServeTemplateThemeModes(t *testing.T) {
	render := func(t *testing.T, h htmlStruct) string {
		t.Helper()
		rec := httptest.NewRecorder()
		if err := serveTemplate(rec, h); err != nil {
			t.Fatalf("serve template: %v", err)
		}
		return rec.Body.String()
	}

	t.Run("auto keeps the dual media block and toggle", func(t *testing.T) {
		body := render(t, htmlStruct{ThemeMode: "auto"})
		for _, want := range []string{`id="theme-light"`, `id="theme-dark"`, `id="theme-toggle"`, "theme-switch.js"} {
			if !strings.Contains(body, want) {
				t.Fatalf("auto mode missing %q\n%s", want, body)
			}
		}
	})

	t.Run("dark pins the dark stylesheet and drops the toggle", func(t *testing.T) {
		body := render(t, htmlStruct{ThemeMode: "dark"})
		if !strings.Contains(body, "github-markdown-dark.css") {
			t.Fatalf("dark mode missing dark stylesheet\n%s", body)
		}
		if strings.Contains(body, `id="theme-toggle"`) || strings.Contains(body, "theme-switch.js") {
			t.Fatalf("dark mode should not render the auto toggle\n%s", body)
		}
	})

	t.Run("custom layers the theme after its base and drops the toggle", func(t *testing.T) {
		body := render(t, htmlStruct{ThemeMode: "custom", ThemeBase: "dark", CustomTheme: "/custom/theme.css"})
		baseIdx := strings.Index(body, "github-markdown-dark.css")
		themeIdx := strings.Index(body, "/custom/theme.css")
		if baseIdx == -1 || themeIdx == -1 || themeIdx < baseIdx {
			t.Fatalf("custom theme must be emitted after its base (base=%d, theme=%d)\n%s", baseIdx, themeIdx, body)
		}
		if strings.Contains(body, `id="theme-toggle"`) {
			t.Fatalf("custom mode should not render the auto toggle\n%s", body)
		}
	})

	t.Run("custom base none omits the built-in markdown stylesheet", func(t *testing.T) {
		body := render(t, htmlStruct{ThemeMode: "custom", ThemeBase: "none", CustomTheme: "/custom/theme.css"})
		if strings.Contains(body, "github-markdown-dark.css") || strings.Contains(body, "github-markdown-light.css") {
			t.Fatalf("base none should not emit a built-in markdown stylesheet\n%s", body)
		}
		if !strings.Contains(body, "/custom/theme.css") {
			t.Fatalf("base none should still emit the custom theme\n%s", body)
		}
	})
}

func TestServeCustomThemeRoute(t *testing.T) {
	tmpDir := t.TempDir()
	themeFile := filepath.Join(tmpDir, "midnight.css")
	if err := os.WriteFile(themeFile, []byte("/* go-grip-base: dark */\n.markdown-body{color:#907cff}\n"), 0o644); err != nil {
		t.Fatalf("write theme: %v", err)
	}

	// Custom theme active: route serves the stylesheet.
	server := NewServer("localhost", 6419, false, false, false, NewParser(), nil, "midnight")
	server.resolvedTheme = themeConfig{mode: "custom", base: "dark", customPath: themeFile}
	handler := server.newHandler(http.Dir(tmpDir))

	req := httptest.NewRequest(http.MethodGet, "/custom/theme.css", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/css") {
		t.Fatalf("expected text/css, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "#907cff") {
		t.Fatalf("expected served theme body, got %q", rec.Body.String())
	}

	// No custom theme active: route 404s.
	plain := NewServer("localhost", 6419, false, false, false, NewParser(), nil, "auto")
	plainHandler := plain.newHandler(http.Dir(tmpDir))
	rec2 := httptest.NewRecorder()
	plainHandler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/custom/theme.css", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no custom theme active, got %d", rec2.Code)
	}
}
