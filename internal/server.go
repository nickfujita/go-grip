package internal

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	chroma_html "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/nickfujita/go-grip/defaults"
	"github.com/nickfujita/go-grip/internal/reload"
)

type Server struct {
	parser        *Parser
	boundingBox   bool
	host          string
	port          int
	browser       bool
	enableReload  bool
	customCSS     []string
	theme         string
	ignoreDirs    []string
	resolvedTheme themeConfig
}

func NewServer(host string, port int, boundingBox bool, browser bool, enableReload bool, parser *Parser, customCSS []string, theme string, ignoreDirs []string) *Server {
	return &Server{
		host:         host,
		port:         port,
		boundingBox:  boundingBox,
		browser:      browser,
		enableReload: enableReload,
		parser:       parser,
		customCSS:    customCSS,
		theme:        theme,
		ignoreDirs:   ignoreDirs,
	}
}

// themeData returns the template fields describing the active theme: the render
// mode, the base for a custom theme, and the route the custom stylesheet is
// served at. It falls back to "auto" when the theme has not been resolved (e.g.
// a handler built directly in tests without calling Serve).
func (s *Server) themeData() (mode, base, customRoute string) {
	cfg := s.resolvedTheme
	switch cfg.mode {
	case "custom":
		return "custom", cfg.base, "/custom/theme.css"
	case "light", "dark":
		return cfg.mode, "", ""
	default:
		return "auto", "", ""
	}
}

// customCSSRoutes returns the stable in-app routes that each user stylesheet is
// served at, in the same order they were passed on the command line.
func (s *Server) customCSSRoutes() []string {
	routes := make([]string, len(s.customCSS))
	for i := range s.customCSS {
		routes[i] = fmt.Sprintf("/custom/css/%d.css", i)
	}
	return routes
}

// validateCustomCSS ensures every user stylesheet exists and is a regular file
// before the server starts, so a typo fails loudly instead of 404-ing at
// request time.
func validateCustomCSS(paths []string) error {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("custom CSS file not found: %s", p)
		}
		if info.IsDir() {
			return fmt.Errorf("custom CSS path is a directory, not a file: %s", p)
		}
	}
	return nil
}

func (s *Server) Serve(file string) error {
	if err := validateCustomCSS(s.customCSS); err != nil {
		return err
	}

	cfg, err := resolveTheme(s.theme)
	if err != nil {
		return err
	}
	s.resolvedTheme = cfg

	directory := path.Dir(file)
	filename := path.Base(file)

	var reloadMiddleware *reload.Reloader
	if s.enableReload {
		reloadMiddleware = reload.New(directory)
		reloadMiddleware.ShouldIgnore = NewIgnoreMatcher(directory, s.ignoreDirs)
		reloadMiddleware.DebugLog = log.New(io.Discard, "", 0)
		// Fix WebSocket CORS issues for development
		reloadMiddleware.Upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}

	dir := http.Dir(directory)
	handler := s.newHandler(dir)

	addr := fmt.Sprintf("http://%s:%d/", s.host, s.port)
	if file == "" {
		// If README.md exists then open README.md at beginning
		readme := "README.md"
		f, err := dir.Open(readme)
		if err == nil {
			//nolint:errcheck
			defer f.Close()
		}
		if err == nil {
			addr, _ = url.JoinPath(addr, readme)
		}
	} else {
		addr, _ = url.JoinPath(addr, filename)
	}

	fmt.Printf("🚀 Starting server: %s\n", addr)

	if s.browser {
		err := Open(addr)
		if err != nil {
			fmt.Println("❌ Error opening browser:", err)
		}
	}

	if s.enableReload {
		handler = reloadMiddleware.Handle(handler)
		fmt.Printf("📡 Auto-reload enabled. Files will trigger browser refresh.\n")
	} else {
		fmt.Printf("🔄 Auto-reload disabled. Use F5 to manually refresh.\n")
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), handler)
}

func (s *Server) newHandler(dir http.Dir) http.Handler {
	fileServer := http.FileServer(dir)
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(defaults.StaticFiles)))
	mux.HandleFunc("/custom/css/", s.serveCustomCSS)
	mux.HandleFunc("/custom/theme.css", s.serveCustomTheme)

	regex := regexp.MustCompile(`(?i)\.md$`)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if regex.MatchString(r.URL.Path) {
			isFile, err := isRegularFile(dir, r.URL.Path)
			if err == nil && isFile {
				setNoCacheHeaders(w)

				bytes, err := readToString(dir, r.URL.Path)
				if err != nil {
					log.Fatal(err)
					return
				}
				htmlContent, err := s.parser.MdToHTML(bytes)
				if err != nil {
					log.Fatal(err)
					return
				}

				themeMode, themeBase, customTheme := s.themeData()
				err = serveTemplate(w, htmlStruct{
					Content:      string(htmlContent),
					BoundingBox:  s.boundingBox,
					CssCodeLight: getCssCode("github"),
					CssCodeDark:  getCssCode("github-dark"),
					CustomCSS:    s.customCSSRoutes(),
					ThemeMode:    themeMode,
					ThemeBase:    themeBase,
					CustomTheme:  customTheme,
				})
				if err != nil {
					log.Fatal(err)
					return
				}
				return
			}
		}

		isDirectory, err := isDirectory(dir, r.URL.Path)
		if err == nil && isDirectory {
			setNoCacheHeaders(w)
			stripCacheValidators(r)
		}

		fileServer.ServeHTTP(w, r)
	})

	return mux
}

// serveCustomCSS serves the user stylesheet registered at
// /custom/css/<index>.css, where <index> is the position of the --css flag on
// the command line.
func (s *Server) serveCustomCSS(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/custom/css/")
	name = strings.TrimSuffix(name, ".css")
	index, err := strconv.Atoi(name)
	if err != nil || index < 0 || index >= len(s.customCSS) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	setNoCacheHeaders(w)
	http.ServeFile(w, r, s.customCSS[index])
}

// serveCustomTheme serves the resolved custom theme stylesheet at
// /custom/theme.css. The stylesheet is sourced from the binary (a built-in
// theme like nightshade) or from disk. It 404s unless a custom theme is active.
func (s *Server) serveCustomTheme(w http.ResponseWriter, r *http.Request) {
	if s.resolvedTheme.mode != "custom" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	setNoCacheHeaders(w)

	if s.resolvedTheme.customContent != nil {
		//nolint:errcheck
		w.Write(s.resolvedTheme.customContent)
		return
	}
	if s.resolvedTheme.customPath == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, s.resolvedTheme.customPath)
}

func readToString(dir http.Dir, filename string) ([]byte, error) {
	f, err := dir.Open(filename)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer f.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(f)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type htmlStruct struct {
	Content      string
	BoundingBox  bool
	CssCodeLight string
	CssCodeDark  string
	CustomCSS    []string
	ThemeMode    string
	ThemeBase    string
	CustomTheme  string
}

func serveTemplate(w http.ResponseWriter, html htmlStruct) error {
	w.Header().Set("Content-Type", "text/html")
	tmpl, err := template.ParseFS(defaults.Templates, "templates/layout.html")
	if err != nil {
		return err
	}
	err = tmpl.Execute(w, html)
	return err
}

func getCssCode(style string) string {
	buf := new(strings.Builder)
	formatter := chroma_html.New(chroma_html.WithClasses(true))
	s := styles.Get(style)
	_ = formatter.WriteCSS(buf, s)
	return buf.String()
}

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func stripCacheValidators(r *http.Request) {
	r.Header.Del("If-Modified-Since")
	r.Header.Del("If-None-Match")
}

func isDirectory(dir http.Dir, name string) (bool, error) {
	file, err := dir.Open(name)
	if err != nil {
		return false, err
	}
	//nolint:errcheck
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return false, err
	}

	return info.IsDir(), nil
}

func isRegularFile(dir http.Dir, name string) (bool, error) {
	file, err := dir.Open(name)
	if err != nil {
		return false, err
	}
	//nolint:errcheck
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return false, err
	}

	return !info.IsDir(), nil
}
