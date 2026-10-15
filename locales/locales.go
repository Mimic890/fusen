// Package locales embeds translation files, one flat JSON object per language.
package locales

import "embed"

// FS holds <lang>.json files.
//
//go:embed *.json
var FS embed.FS
