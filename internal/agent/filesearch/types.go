package filesearch

// FileSearchResult represents a single file search match.
type FileSearchResult struct {
	Name string `json:"name"` // basename: "argus"
	Path string `json:"path"` // absolute: "/home/user/Workspace/repos/bxnlabs/argus"
	Type string `json:"type"` // "file" or "directory"
}

// FileSearchResponse contains all matches for a query.
type FileSearchResponse struct {
	Results  []FileSearchResult `json:"results"`
	Query    string             `json:"query"`
	Count    int                `json:"count"`
	Partial  bool               `json:"partial,omitempty"`
	TimedOut bool               `json:"timedOut,omitempty"`
	Scanned  int                `json:"scanned,omitempty"`
}
