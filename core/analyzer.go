package core

// FileContext is what an Analyzer needs to inspect a single file.
type FileContext struct {
	Path    string // relative path from repo root
	Content []byte
}

// Analyzer is the plugin boundary. The scan engine never hardcodes a
// detection rule directly — every rule lives behind this interface.
type Analyzer interface {
	Name() string
	Run(file FileContext) []Finding
}
