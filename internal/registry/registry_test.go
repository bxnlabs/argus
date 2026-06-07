package registry

import (
	"context"
	"testing"
)

type fakeDB struct{ nodes []ManualNode }

func (f *fakeDB) ListManualNodes(context.Context) ([]ManualNode, error) { return f.nodes, nil }

func TestList_MergesSourcesWithPrecedence(t *testing.T) {
	db := &fakeDB{nodes: []ManualNode{{ID: "m1", Name: "manual-gpu", URL: "http://gpu-box:80"}}}
	self := Node{ID: "self", Name: "this", URL: "", Source: SourceLocal, Self: true, dedupKey: "http://laptop:80"}
	discover := func(context.Context) ([]Node, error) {
		return []Node{
			{ID: "d1", Name: "gpu-box", URL: "http://gpu-box:80", Source: SourceDiscovered, dedupKey: "http://gpu-box:80"},
			{ID: "d2", Name: "laptop", URL: "http://laptop:80", Source: SourceDiscovered, dedupKey: "http://laptop:80"},
			{ID: "d3", Name: "ci", URL: "http://ci:80", Source: SourceDiscovered, dedupKey: "http://ci:80"},
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
