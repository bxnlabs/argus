package registry

import (
	"context"
	"testing"
)

type fakeDB struct{ nodes []ManualNode }

func (f *fakeDB) ListManualNodes(context.Context) ([]ManualNode, error) { return f.nodes, nil }

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		wantEq  bool
		comment string
	}{
		{
			name:    "empty string",
			a:       "",
			b:       "",
			wantEq:  true,
			comment: "empty -> empty",
		},
		{
			name:    "trailing slash stripped",
			a:       "http://host/",
			b:       "http://host",
			wantEq:  true,
			comment: "trailing slash should be stripped",
		},
		{
			name:    "uppercase host collapses",
			a:       "http://GPU-BOX:80",
			b:       "http://gpu-box:80",
			wantEq:  true,
			comment: "host should be lowercased",
		},
		{
			name:    "uppercase scheme collapses",
			a:       "HTTP://gpu-box:80",
			b:       "http://gpu-box:80",
			wantEq:  true,
			comment: "scheme should be lowercased (Fix A)",
		},
		{
			name:    "explicit default port collapses (http:80)",
			a:       "http://host:80",
			b:       "http://host",
			wantEq:  true,
			comment: "http:80 is the default and must be stripped so variants collapse",
		},
		{
			name:    "explicit default port collapses (https:443)",
			a:       "https://host:443",
			b:       "https://host",
			wantEq:  true,
			comment: "https:443 is the default and must be stripped so variants collapse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			na, nb := normalize(tt.a), normalize(tt.b)
			eq := na == nb
			if eq != tt.wantEq {
				t.Errorf("normalize(%q)=%q, normalize(%q)=%q, equal=%v, want equal=%v (%s)",
					tt.a, na, tt.b, nb, eq, tt.wantEq, tt.comment)
			}
		})
	}

	// Unparseable input must not panic and must return a lowercased string.
	t.Run("unparseable falls back to lowercased raw", func(t *testing.T) {
		raw := "://INVALID HOST"
		got := normalize(raw)
		// Just assert it doesn't panic and returns something lowercase-safe.
		if got == "" && raw != "" {
			// Empty is fine only if input was empty; otherwise just verify no panic occurred.
		}
		// The result should be lowercase (the fallback path calls strings.ToLower).
		for _, r := range got {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("normalize(%q) = %q contains uppercase character %q", raw, got, r)
			}
		}
	})
}

func TestList_NilDiscoverOmitsDiscoveredNodes(t *testing.T) {
	db := &fakeDB{nodes: []ManualNode{{ID: "m1", Name: "manual-a", URL: "http://a:80"}}}
	self := Node{ID: "self", Name: "this", URL: "", Source: SourceLocal, Self: true}
	svc := New(db, self, nil) // nil discover

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Expect self + one manual node = 2, no panic.
	if len(got) != 2 {
		t.Errorf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].Source != SourceLocal {
		t.Errorf("first node source = %s, want %s", got[0].Source, SourceLocal)
	}
	if got[1].Source != SourceManual {
		t.Errorf("second node source = %s, want %s", got[1].Source, SourceManual)
	}
}

func TestList_MergesSourcesWithPrecedence(t *testing.T) {
	db := &fakeDB{nodes: []ManualNode{{ID: "m1", Name: "manual-gpu", URL: "http://gpu-box:80"}}}
	self := Node{ID: "self", Name: "this", URL: "", Source: SourceLocal, Self: true, dedupKey: normalize("http://laptop:80")}
	discover := func(context.Context) ([]Node, error) {
		return []Node{
			{ID: "d1", Name: "gpu-box", URL: "http://gpu-box:80", Source: SourceDiscovered, dedupKey: normalize("http://gpu-box:80")},
			{ID: "d2", Name: "laptop", URL: "http://laptop:80", Source: SourceDiscovered, dedupKey: normalize("http://laptop:80")},
			{ID: "d3", Name: "ci", URL: "http://ci:80", Source: SourceDiscovered, dedupKey: normalize("http://ci:80")},
		}, nil
	}
	svc := New(db, self, discover)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Expect: self (local) + manual-gpu (manual wins over discovered d1) + ci.
	// laptop (d2) is dropped as a duplicate of self.
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	bySource := map[Source]int{}
	for _, n := range got {
		bySource[n.Source]++
	}
	if bySource[SourceLocal] != 1 || bySource[SourceManual] != 1 || bySource[SourceDiscovered] != 1 {
		t.Errorf("source counts = %v, want 1 each", bySource)
	}
	for _, n := range got {
		if n.URL == "http://gpu-box:80" && n.Source != SourceManual {
			t.Errorf("gpu-box should be manual, got %s", n.Source)
		}
	}
}

func TestList_DiscoveryErrorDegrades(t *testing.T) {
	db := &fakeDB{nodes: []ManualNode{{ID: "m1", Name: "a", URL: "http://a:80"}}}
	self := Node{ID: "self", Source: SourceLocal, Self: true}
	discover := func(context.Context) ([]Node, error) { return nil, context.DeadlineExceeded }
	svc := New(db, self, discover)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List should not error when discovery fails: %v", err)
	}
	if len(got) != 2 { // self + manual
		t.Errorf("len = %d, want 2", len(got))
	}
}
