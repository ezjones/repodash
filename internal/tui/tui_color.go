package tui

import (
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"
)

// Ports of the page's cover-art colour code (internal/server/web/index.html: hash, rng, PAL, ART.name,
// LANG_COLOR, mixHex) so the terminal shows the same colours for the same repo name.

// nameHash is the page's hash(): FNV-1a over the first UTF-16 unit of each code point
// (JS `for (const c of str)` with `c.charCodeAt(0)`).
func nameHash(s string) uint32 {
	h := uint32(2166136261)
	for _, r := range s {
		h ^= uint32(utf16.Encode([]rune{r})[0])
		h *= 16777619
	}
	return h
}

// rng is the page's mulberry32, with JS 32-bit integer semantics.
func rng(seed uint32) func() float64 {
	return func() float64 {
		seed += 0x6D2B79F5
		t := (seed ^ seed>>15) * (1 | seed)
		t = (t + (t^t>>7)*(61|t)) ^ t
		return float64(t^t>>14) / 4294967296
	}
}

var bandPalette = [][5]string{
	{"#EEE6D8", "#D9573B", "#2F5D62", "#E8B04A", "#243035"}, {"#E6EDF3", "#274C77", "#6096BA", "#E3A812", "#1B2A3A"},
	{"#EFEAE0", "#7A9E7E", "#C7603B", "#3B4B59", "#E2C28F"}, {"#ECE9F5", "#5B4B8A", "#F2A65A", "#2A9D8F", "#2B2B3A"},
	{"#F3E7E2", "#B5446E", "#2D6A6A", "#F0B67F", "#2E2A33"}, {"#E7EFE4", "#2E6F40", "#E9A23B", "#C84B31", "#1F2D26"},
	{"#F1ECDB", "#1D4E89", "#E4572E", "#F2C14E", "#222222"}, {"#E4EBEE", "#0B6E6E", "#F26B5B", "#F6D55C", "#173F4F"},
	{"#F0E4EA", "#3C3F8F", "#E26D5A", "#7BC8A4", "#25243A"}, {"#E8EDE6", "#C2452D", "#3F7F6E", "#EAB308", "#2A2F2B"},
}

// bandColors is the page's ART.name: background and text colour for a repo's name band.
func bandColors(name string) (bg, ink string) {
	h := nameHash(name)
	P := bandPalette[int(h)%len(bandPalette)]
	bg = [4]string{P[4], P[1], P[2], P[0]}[int(math.Floor(rng(h)()*4))]
	ink = "#ffffff"
	if bg == P[0] || bg == P[3] {
		ink = P[4]
	}
	return
}

var langColor = map[string]string{"Go": "#2A9CB8", "Python": "#3E6FA3", "TypeScript": "#3178C6", "JavaScript": "#B59B0C", "Rust": "#B5532A",
	"Swift": "#E0533D", "Shell": "#4E9A3C", "C++": "#C2417A", "C": "#5C6B7A", "Ruby": "#B8322A", "Java": "#B0722A", "Kotlin": "#7F52FF",
	"C#": "#2F8F3A", "PHP": "#6C78AF", "Lua": "#2C2D72", "Dart": "#0B7FA8", "Zig": "#C27A1A", "Elixir": "#6E4A7E", "Haskell": "#5E5086",
	"Scala": "#C22D40", "Vue": "#3B9E6E", "Svelte": "#D9481C", "QML": "#41A15C"}

// mixHex blends hex colour a toward b by t, as the page's mixHex does.
func mixHex(a, b string, t float64) string {
	p := func(h string, i int) float64 { v, _ := strconv.ParseUint(h[i:i+2], 16, 8); return float64(v) }
	out := "#"
	for _, i := range []int{1, 3, 5} {
		x, y := p(a, i), p(b, i)
		out += fmt.Sprintf("%02x", int(math.Floor(x+(y-x)*t+0.5)))
	}
	return out
}

// langBadgeColor is the badge background the page uses for a language.
func langBadgeColor(lang string) string {
	c, ok := langColor[lang]
	if !ok {
		c = "#667788"
	}
	return mixHex(c, "#000000", .38)
}
