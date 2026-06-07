package registry

import (
	"context"
	"log"
	"net/url"
	"strings"
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

// ManualNode mirrors db.ManualNode without importing the db package.
type ManualNode struct {
	ID   string
	Name string
	URL  string
}

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

// normalize lowercases the host and strips a trailing slash so that
// "http://Gpu-Box:80/" and "http://gpu-box:80" collapse.
func normalize(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String()
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
