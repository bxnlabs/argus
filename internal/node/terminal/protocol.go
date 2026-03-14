package terminal

// controlMessage is the JSON protocol envelope for WebSocket text frames.
// Binary frames carry raw PTY data (no JSON wrapping).
type controlMessage struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	Session string `json:"session,omitempty"`
	Message string `json:"message,omitempty"`
	Text    string `json:"text,omitempty"`   // compose: text to inject
	Submit  *bool  `json:"submit,omitempty"` // compose: append \r after text (default true for backwards compat)
}
