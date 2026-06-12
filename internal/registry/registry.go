package registry

import (
	"context"
	"errors"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/bxnlabs/argus/internal/node/db"
)

// Source identifies how a node entered the registry.
type Source string

const (
	SourceLocal      Source = "local"
	SourceManual     Source = "manual"
	SourceDiscovered Source = "discovered"
)

// Node is a registry entry as returned to the client — a pure JSON DTO. Dedup
// keys are computed locally where needed (in List and the write path) rather
// than carried on the struct, so identity handling lives in one place.
type Node struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"` // empty == same-origin (the local node)
	Source Source `json:"source"`
	Self   bool   `json:"self"`
}

// ManualNode is db.ManualNode; aliased so callers keep using
// registry.ManualNode while the registry shares the db's row type directly.
type ManualNode = db.ManualNode

// DB is the persistence dependency (satisfied by *db.DB), covering the registry's
// reads and manual-node mutations.
type DB interface {
	ListManualNodes(ctx context.Context) ([]ManualNode, error)
	AddManualNode(ctx context.Context, id, name, url string) error
	UpdateManualNode(ctx context.Context, id, name, url string) error
	DeleteManualNode(ctx context.Context, id string) error
}

// DiscoverFunc returns discovered nodes (shaped as registry.Node). It is
// best-effort: an error degrades to local+manual.
type DiscoverFunc func(ctx context.Context) ([]Node, error)

// ValidationError is a client-facing input error: the handler maps it to HTTP
// 400 and surfaces its message verbatim.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ErrSelfOrigin is returned when a manual add/update points at this node's own
// origin. List gives the local node precedence and dedups on its key, so such a
// row would silently vanish from the listing; the write path rejects it up front
// (mapped to HTTP 409).
var ErrSelfOrigin = errors.New("that url is this node")

// Service merges the three node sources and owns manual-node mutations.
type Service struct {
	db       DB
	self     Node
	selfKey  string // normalized dedup key for the local node ("" when no tailnet identity)
	discover DiscoverFunc
}

// New builds a registry service. selfDedupURL is the local node's canonical
// (tailnet) URL used to collapse a discovered copy of self; "" means the node
// has no tailnet identity, so nothing can collide with self. discover may be nil
// (no Tailscale).
func New(db DB, self Node, selfDedupURL string, discover DiscoverFunc) *Service {
	s := &Service{db: db, self: self, discover: discover}
	if selfDedupURL != "" {
		s.selfKey = normalize(selfDedupURL)
	}
	return s
}

// normalize lowercases scheme+host, strips a trailing slash, and drops the
// scheme's default port (:80 for http, :443 for https) so that variants like
// "http://Gpu-Box:80/" and "http://gpu-box" collapse to the same key.
func normalize(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	// Trim the FQDN's trailing dot so a manually-added "gpu.ts.net." dedups
	// against the discovered "gpu.ts.net" (discovery trims it too, but this is
	// the single chokepoint every source's URL passes through).
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		// JoinHostPort re-brackets IPv6 literals (host "::1" → "[::1]:3000")
		// that Hostname() unbracketed; "host:port" alone would be invalid.
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	return u.String()
}

// isSelfOrigin reports whether raw normalizes to the local node's dedup key.
// False when self has no dedup key (no tailnet identity), since then nothing can
// collide with self.
func (s *Service) isSelfOrigin(raw string) bool {
	return s.selfKey != "" && normalize(raw) == s.selfKey
}

// validNodeOrigin trims, validates, and normalizes a node origin URL. On success
// it returns the normalized URL and an empty message; on failure it returns ""
// and a client-facing error message. The URL is used as a node origin/base, so a
// path, query, fragment, or userinfo would corrupt later requests (e.g.
// http://gpu/api/api/node/summary); only a bare scheme://host[:port] (optionally
// a lone "/") is accepted.
//
// Origin *reachability* is intentionally not validated here. The supported
// trust boundary is loopback/tailnet (see the multi-node design spec), but that
// is enforced at request time by the target node's own CORS policy, and a node
// the browser can't reach simply reads as offline in the rail. Allow-listing
// origins at add time would add no real safety (the runtime CORS is the gate)
// while falsely rejecting legitimate tailnet short-names (e.g. "http://gpu",
// which resolves via the tailnet search domain but has no ".ts.net" suffix to
// match against).
func validNodeOrigin(raw string) (normalized, errMsg string) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "url must be an absolute http(s) URL"
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", "url must be a node origin, e.g. http://host:port"
	}
	// url.Parse already rejects a non-numeric or negative port ("http://h:abc",
	// "http://h:-1"), but it accepts out-of-range numbers ("http://h:0",
	// ":99999", ":65536"); reject those so a node can't be saved with a port it
	// could never listen on.
	if p := u.Port(); p != "" {
		n, perr := strconv.Atoi(p)
		if perr != nil || n < 1 || n > 65535 {
			return "", "url port must be between 1 and 65535"
		}
	}
	return normalize(raw), ""
}

// AddManualNode validates name/url, rejects this node's own origin, and persists
// a new manual node under id. The URL is stored normalized so the DB UNIQUE
// constraint and the dedup key agree (case/port variants collapse to one value).
// Returns *ValidationError (bad input), ErrSelfOrigin, db.ErrDuplicateURL, or a
// wrapped internal error.
func (s *Service) AddManualNode(ctx context.Context, id, name, rawURL string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &ValidationError{Msg: "name is required"}
	}
	normalizedURL, errMsg := validNodeOrigin(rawURL)
	if errMsg != "" {
		return &ValidationError{Msg: errMsg}
	}
	if s.isSelfOrigin(normalizedURL) {
		return ErrSelfOrigin
	}
	return s.db.AddManualNode(ctx, id, name, normalizedURL)
}

// UpdateManualNode edits a manual node's name and origin in place. Both are
// required (the only client, the Configure Node dialog, always sends both).
// Returns *ValidationError, ErrSelfOrigin, db.ErrNotFound, db.ErrDuplicateURL,
// or a wrapped internal error.
func (s *Service) UpdateManualNode(ctx context.Context, id, name, rawURL string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &ValidationError{Msg: "name is required"}
	}
	if strings.TrimSpace(rawURL) == "" {
		return &ValidationError{Msg: "url is required"}
	}
	normalizedURL, errMsg := validNodeOrigin(rawURL)
	if errMsg != "" {
		return &ValidationError{Msg: errMsg}
	}
	if s.isSelfOrigin(normalizedURL) {
		return ErrSelfOrigin
	}
	return s.db.UpdateManualNode(ctx, id, name, normalizedURL)
}

// DeleteManualNode removes a manual node by id. Returns db.ErrNotFound when no
// such node exists.
func (s *Service) DeleteManualNode(ctx context.Context, id string) error {
	return s.db.DeleteManualNode(ctx, id)
}

// NodeFromDiscovery builds a discovered node. Its dedup key is computed from URL
// in List, so none is stored here.
func NodeFromDiscovery(name, rawURL string) Node {
	return Node{
		ID:     "discovered:" + normalize(rawURL),
		Name:   name,
		URL:    rawURL,
		Source: SourceDiscovered,
	}
}

// List returns local + manual + discovered, deduped by normalized URL with
// precedence local > manual > discovered.
func (s *Service) List(ctx context.Context) ([]Node, error) {
	seen := map[string]bool{}
	var out []Node

	// 1. Local (self) always first.
	if s.selfKey != "" {
		seen[s.selfKey] = true
	}
	out = append(out, s.self)

	// 2. Manual.
	manual, err := s.db.ListManualNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range manual {
		key := normalize(m.URL)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Node{ID: m.ID, Name: m.Name, URL: m.URL, Source: SourceManual})
	}

	// 3. Discovered (best-effort).
	if s.discover != nil {
		discovered, derr := s.discover(ctx)
		if derr != nil {
			log.Printf("registry: discovery degraded: %v", derr)
		} else {
			for _, d := range discovered {
				key := normalize(d.URL)
				if seen[key] {
					continue
				}
				seen[key] = true
				d.Source = SourceDiscovered
				out = append(out, d)
			}
		}
	}

	return out, nil
}
