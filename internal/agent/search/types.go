package search

// SearchMatch represents a single match from ripgrep.
type SearchMatch struct {
	File      string `json:"file"`      // relative path from search dir
	Line      int    `json:"line"`      // 1-indexed line number
	Column    int    `json:"column"`    // byte offset in line (0-indexed)
	MatchText string `json:"matchText"` // the matched substring
	LineText  string `json:"lineText"`  // full line content (newline stripped)
}

// SearchResult contains all matches for a query.
type SearchResult struct {
	Results []SearchMatch `json:"results"`
	Query   string        `json:"query"`
	Path    string        `json:"path"` // search directory (absolute)
	Count   int           `json:"count"`
}
