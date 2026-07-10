package defaults

import "embed"

//go:embed templates
var Templates embed.FS

//go:embed static
var StaticFiles embed.FS

// Themes holds the built-in custom themes compiled into the binary (e.g.
// nightshade). These are resolvable by name via --theme with no external file.
//
//go:embed themes
var Themes embed.FS
