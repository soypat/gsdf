package textsdf

import (
	_ "embed"
)

// Embedded fonts.
var (
	//go:embed iso-3098.ttf
	_iso3098TTF []byte
)

// ISO3098TTF returns the ISO-3098 true type font file.
func ISO3098TTF() []byte {
	return append([]byte{}, _iso3098TTF...) // copy contents.
}
