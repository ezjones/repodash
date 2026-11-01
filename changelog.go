package main

import _ "embed"

// changelog is CHANGELOG.md, shown as "What's new" in the TUI and on the web page.
//
//go:embed CHANGELOG.md
var changelog string
