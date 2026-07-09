package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDirectoryListingIgnoresCacheValidators(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	server := NewServer("localhost", 6419, false, false, false, NewParser(), nil)
	handler := server.newHandler(http.Dir(tmpDir))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-Modified-Since", time.Now().Add(24*time.Hour).UTC().Format(http.TimeFormat))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("expected Cache-Control to disable storage, got %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "README.md") {
		t.Fatalf("expected directory listing body to mention README.md, got %q", recorder.Body.String())
	}
}

func TestRegularFileStillSupportsConditionalRequests(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "plain.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write plain.txt: %v", err)
	}

	server := NewServer("localhost", 6419, false, false, false, NewParser(), nil)
	handler := server.newHandler(http.Dir(tmpDir))

	req := httptest.NewRequest(http.MethodGet, "/plain.txt", nil)
	req.Header.Set("If-Modified-Since", time.Now().Add(24*time.Hour).UTC().Format(http.TimeFormat))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("expected status %d, got %d", http.StatusNotModified, recorder.Code)
	}
}

func TestMarkdownResponsesDisableCaching(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	server := NewServer("localhost", 6419, false, false, false, NewParser(), nil)
	handler := server.newHandler(http.Dir(tmpDir))

	req := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	req.Header.Set("If-Modified-Since", time.Now().Add(24*time.Hour).UTC().Format(http.TimeFormat))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("expected Cache-Control to disable storage, got %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html" {
		t.Fatalf("expected text/html response, got %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "Hello") {
		t.Fatalf("expected rendered markdown response to contain document content, got %q", recorder.Body.String())
	}
}

func TestCustomCSSIsServedAtStableRoute(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cssPath := filepath.Join(tmpDir, "brand.css")
	if err := os.WriteFile(cssPath, []byte(".markdown-body{color:#907cff}\n"), 0o644); err != nil {
		t.Fatalf("write brand.css: %v", err)
	}

	server := NewServer("localhost", 6419, false, false, false, NewParser(), []string{cssPath})
	handler := server.newHandler(http.Dir(tmpDir))

	req := httptest.NewRequest(http.MethodGet, "/custom/css/0.css", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "text/css") {
		t.Fatalf("expected text/css content type, got %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "#907cff") {
		t.Fatalf("expected served body to contain stylesheet contents, got %q", recorder.Body.String())
	}
}

func TestCustomCSSUnknownIndexReturns404(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cssPath := filepath.Join(tmpDir, "brand.css")
	if err := os.WriteFile(cssPath, []byte(".markdown-body{}\n"), 0o644); err != nil {
		t.Fatalf("write brand.css: %v", err)
	}

	server := NewServer("localhost", 6419, false, false, false, NewParser(), []string{cssPath})
	handler := server.newHandler(http.Dir(tmpDir))

	for _, path := range []string{"/custom/css/5.css", "/custom/css/nope.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", path, recorder.Code)
		}
	}
}

func TestCustomCSSLinkTagsEmittedAfterTheme(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := serveTemplate(recorder, htmlStruct{
		Content:   "<p>hi</p>",
		CustomCSS: []string{"/custom/css/0.css", "/custom/css/1.css"},
	})
	if err != nil {
		t.Fatalf("serve template: %v", err)
	}

	body := recorder.Body.String()
	for _, route := range []string{"/custom/css/0.css", "/custom/css/1.css"} {
		if !strings.Contains(body, `<link rel="stylesheet" href="`+route+`" />`) {
			t.Fatalf("expected link tag for %s, got:\n%s", route, body)
		}
	}

	// Custom stylesheets must come after the built-in theme stylesheet so they win the cascade.
	themeIdx := strings.Index(body, "github-markdown-dark.css")
	customIdx := strings.Index(body, "/custom/css/0.css")
	if themeIdx == -1 || customIdx == -1 || customIdx < themeIdx {
		t.Fatalf("expected custom CSS to be emitted after the theme block (theme=%d, custom=%d)", themeIdx, customIdx)
	}
}

func TestValidateCustomCSS(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cssPath := filepath.Join(tmpDir, "brand.css")
	if err := os.WriteFile(cssPath, []byte(".markdown-body{}\n"), 0o644); err != nil {
		t.Fatalf("write brand.css: %v", err)
	}

	if err := validateCustomCSS([]string{cssPath}); err != nil {
		t.Fatalf("expected existing file to validate, got %v", err)
	}
	if err := validateCustomCSS(nil); err != nil {
		t.Fatalf("expected empty list to validate, got %v", err)
	}

	missing := filepath.Join(tmpDir, "does-not-exist.css")
	if err := validateCustomCSS([]string{missing}); err == nil {
		t.Fatalf("expected error for missing file %s", missing)
	}
	if err := validateCustomCSS([]string{tmpDir}); err == nil {
		t.Fatalf("expected error when path is a directory")
	}
}
