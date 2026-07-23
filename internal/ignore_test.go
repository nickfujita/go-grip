package internal

import "testing"

func TestIgnoreMatcherNamePatterns(t *testing.T) {
	match := NewIgnoreMatcher("/srv/docs", DefaultIgnores)

	cases := []struct {
		path string
		want bool
	}{
		{"/srv/docs", false},                        // serve root itself
		{"/srv/docs/guide", false},                  // regular directory
		{"/srv/docs/node_modules", true},            // dependency tree at top level
		{"/srv/docs/app/node_modules/lodash", true}, // nested, matched via any element
		{"/srv/docs/.git", true},                    // hidden directory
		{"/srv/docs/app/.cache/tmp", true},          // nested hidden directory
		{"/srv/docs/app/vendor", true},              // vendor tree
		{"/srv/other/node_modules", false},          // outside the serve root
		{"/srv/docs/notes.md", false},               // regular file event
	}
	for _, c := range cases {
		if got := match(c.path); got != c.want {
			t.Errorf("match(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIgnoreMatcherPathPatterns(t *testing.T) {
	match := NewIgnoreMatcher("/srv/docs", []string{"big/data", "logs/*"})

	cases := []struct {
		path string
		want bool
	}{
		{"/srv/docs/big/data", true},          // exact relative path
		{"/srv/docs/big/data/sub/deep", true}, // everything below a matched directory
		{"/srv/docs/big", false},              // parent of a matched path
		{"/srv/docs/logs/2026", true},         // glob on relative path
		{"/srv/docs/logs", false},             // glob does not match the bare parent
		{"/srv/docs/node_modules", false},     // defaults replaced, not appended
	}
	for _, c := range cases {
		if got := match(c.path); got != c.want {
			t.Errorf("match(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIgnoreMatcherRelativeRoot(t *testing.T) {
	// Serve() derives the directory from the file argument, so relative
	// roots like "." must anchor to the working directory.
	match := NewIgnoreMatcher(".", DefaultIgnores)
	if match(".") {
		t.Error("relative serve root must never be ignored")
	}
	if !match("node_modules") {
		t.Error("relative paths under the root must match name patterns")
	}
}
