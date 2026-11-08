package source_test

import (
	"context"
	"testing"

	"ai-flight-dashboard/internal/source"
)

type fakeSource struct {
	desc source.Descriptor
	dirs []string
}

func (f fakeSource) Descriptor() source.Descriptor { return f.desc }
func (f fakeSource) WatchDirs() []string           { return f.dirs }
func (f fakeSource) Scan(context.Context, source.Sink) error {
	return nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := source.NewRegistry()
	a := fakeSource{desc: source.Descriptor{ID: "a", Label: "A"}, dirs: []string{"/logs/a"}}
	r.Register(a)
	if got := r.Get("a"); got == nil || got.Descriptor().ID != "a" {
		t.Fatalf("expected source a, got %+v", got)
	}
	r.Unregister("a")
	if got := r.Get("a"); got != nil {
		t.Fatalf("expected source a to be removed")
	}
}

func TestRegistryDescriptorsHidesAndPreservesOrder(t *testing.T) {
	r := source.NewRegistry()
	r.Register(fakeSource{desc: source.Descriptor{ID: "b", Label: "BETA"}})
	r.Register(fakeSource{desc: source.Descriptor{ID: "a", Label: "ALPHA"}})
	r.Register(fakeSource{desc: source.Descriptor{ID: "hidden", Label: "HIDDEN", Hidden: true}})

	descriptors := r.Descriptors()
	if len(descriptors) != 2 {
		t.Fatalf("expected 2 visible descriptors, got %d", len(descriptors))
	}
	if descriptors[0].ID != "b" || descriptors[1].ID != "a" {
		t.Fatalf("expected registration order, got %+v", descriptors)
	}
}

func TestRegistrySourceForPathUsesLongestPrefix(t *testing.T) {
	r := source.NewRegistry()
	r.Register(fakeSource{desc: source.Descriptor{ID: "root", Label: "ROOT"}, dirs: []string{"/logs"}})
	r.Register(fakeSource{desc: source.Descriptor{ID: "nested", Label: "NESTED"}, dirs: []string{"/logs/nested"}})

	if got := r.SourceForPath("/logs/nested/session.jsonl"); got == nil || got.Descriptor().ID != "nested" {
		t.Fatalf("expected nested source, got %+v", got)
	}
	if got := r.SourceForPath("/logs/other/session.jsonl"); got == nil || got.Descriptor().ID != "root" {
		t.Fatalf("expected root source, got %+v", got)
	}
	if got := r.SourceForPath("/elsewhere/session.jsonl"); got != nil {
		t.Fatalf("expected no source, got %+v", got)
	}
}
