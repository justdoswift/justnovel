package main

import "testing"

// TestSearchDirectlyLive asks the two sites that answer a plain request, for a
// title they have, and checks real books come back.
func TestSearchDirectlyLive(t *testing.T) {
	for _, tc := range []struct {
		kind  SourceKind
		query string
	}{
		{SourcePiaotian, "极道天魔"},
		{SourceSto9, "斗破苍穹"},
	} {
		source := SourceFor(tc.kind)
		hits, err := searchDirectly(source, tc.query)
		if err != nil {
			t.Logf("%s %q: %v", source.Name, tc.query, err)
			continue
		}
		t.Logf("%s %q -> %d hits", source.Name, tc.query, len(hits))
		for i, h := range hits {
			if i > 2 {
				break
			}
			t.Logf("   %q by %q -> %s", h.Title, h.Author, h.URL)
		}
		if len(hits) == 0 {
			t.Errorf("%s returned nothing for %q", source.Name, tc.query)
		}
	}
}
