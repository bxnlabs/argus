package registry

import (
	"context"
	"log"
	"net"
	"net/url"
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

// Node is a registry entry as returned to the client.
type Node struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"` // empty == same-origin (the local node)
	Source Source `json:"source"`
	Self   bool   `json:"self"`

	// dedupKey is the normalized URL used to collapse duplicates across
	// sources. For the local node it is its own tailnet URL (so a discovered
	// copy of self is dropped); for others it is normalize(URL). Not serialized.
	dedupKey string `json:"-"`
}

// ManualNode is db.ManualNode; aliased so callers keep using
// registry.ManualNode while the registry shares the db's row type directly.
type ManualNode = db.ManualNode

// DB is the persistence dependency (satisfied by *db.DB).
type DB interface {
	ListManualNodes(ctx context.Context) ([]ManualNode, error)
}

// DiscoverFunc returns discovered nodes (already shaped as registry.Node with a
// dedupKey set). It is best-effort: an error degrades to local+manual.
type DiscoverFunc func(ctx context.Context) ([]Node, error)

// Service merges the three node sources.
type Service struct {
	db       DB
	self     Node
	discover DiscoverFunc
}

// New builds a registry service. discover may be nil (no Tailscale).
func New(db DB, self Node, discover DiscoverFunc) *Service {
	return &Service{db: db, self: self, discover: discover}
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

// IsSelfOrigin reports whether raw normalizes to the local node's dedup key,
// i.e. a manual add/update pointing at this same node. List gives the local
// node precedence and dedups on this key, so such a row would be silently
// dropped from the listing ("I added it and it vanished"); the handler rejects
// it up front instead. False when self has no dedup key (no tailnet identity),
// since then nothing can collide with self.
func (s *Service) IsSelfOrigin(raw string) bool {
	return s.self.dedupKey != "" && normalize(raw) == s.self.dedupKey
}

// SetDedupKey sets the local node's dedup key from its canonical (tailnet) URL.
func (n *Node) SetDedupKey(canonicalURL string) {
	if canonicalURL != "" {
		n.dedupKey = normalize(canonicalURL)
	}
}

// NodeFromDiscovery builds a discovered node with its dedup key set.
func NodeFromDiscovery(name, rawURL string) Node {
	return Node{
		ID:       "discovered:" + normalize(rawURL),
		Name:     name,
		URL:      rawURL,
		Source:   SourceDiscovered,
		dedupKey: normalize(rawURL),
	}
}

// List returns local + manual + discovered, deduped by normalized URL with
// precedence local > manual > discovered.
func (s *Service) List(ctx context.Context) ([]Node, error) {
	seen := map[string]bool{}
	var out []Node

	// 1. Local (self) always first.
	self := s.self
	if self.dedupKey != "" {
		seen[self.dedupKey] = true
	}
	out = append(out, self)

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
		out = append(out, Node{ID: m.ID, Name: m.Name, URL: m.URL, Source: SourceManual, dedupKey: key})
	}

	// 3. Discovered (best-effort).
	if s.discover != nil {
		discovered, derr := s.discover(ctx)
		if derr != nil {
			log.Printf("registry: discovery degraded: %v", derr)
		} else {
			for _, d := range discovered {
				key := d.dedupKey
				if key == "" {
					key = normalize(d.URL)
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				d.Source = SourceDiscovered
				d.dedupKey = key
				out = append(out, d)
			}
		}
	}

	return out, nil
}
