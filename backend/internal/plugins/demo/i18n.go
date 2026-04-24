package demo

import (
	"embed"
)

// localeFS contains the plugin's i18n bundles. Every file under
// locales/*.json is embedded and exposed through LocaleFS so the host's
// admin UI can load labels on demand.
//
//go:embed locales/*.json
var localeFS embed.FS

// LocaleFS returns the embedded filesystem containing the plugin's
// locale bundles. It is exported so the host (and tests) can enumerate
// the files without depending on the internal embed variable name.
func LocaleFS() embed.FS { return localeFS }
