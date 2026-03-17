package state

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestStateJSONRoundTrip(t *testing.T) {
	updatedAt := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 3, 17, 10, 3, 0, 0, time.UTC)
	lastFetchedAt := time.Date(2026, 3, 17, 10, 3, 0, 0, time.UTC)
	lastSuccessAt := time.Date(2026, 3, 17, 10, 3, 0, 0, time.UTC)

	in := State{
		SchemaVersion:  CurrentSchemaVersion,
		BaseURL:        "https://themis.housing.rug.nl",
		CatalogRootURL: "https://themis.housing.rug.nl/course",
		UpdatedAt:      updatedAt,
		Roots: []RootRef{
			{
				NodeID:       "url:e51a8c8fa915b6e24cc85d4b992b1962b81b6170c294bfae64e516e416eb3145",
				CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os",
				Title:        "Operating Systems",
				Kind:         "course",
				UpdatedAt:    updatedAt,
			},
		},
		Nodes: map[string]Node{
			"url:e51a8c8fa915b6e24cc85d4b992b1962b81b6170c294bfae64e516e416eb3145": {
				ID:               "url:e51a8c8fa915b6e24cc85d4b992b1962b81b6170c294bfae64e516e416eb3145",
				Kind:             "course",
				Title:            "Operating Systems",
				CanonicalURL:     "https://themis.housing.rug.nl/course/2025-2026/os",
				NavAPIURL:        "https://themis.housing.rug.nl/api/navigation/2025-2026/os",
				ParentIDs:        []string{"url:parent"},
				ChildIDs:         []string{"url:child1", "url:child2"},
				ChildrenHydrated: true,
				DepthHint:        3,
				Status:           StatusOK,
				LastFetchedAt:    &lastFetchedAt,
				LastSuccessAt:    &lastSuccessAt,
				ContentHash:      "sha256:3a9d",
				Details: map[string]any{
					"breadcrumb": []any{"Courses", "2025-2026", "Operating Systems"},
				},
				Assets: []AssetRef{
					{Name: "1.in", URL: "https://themis.housing.rug.nl/file/course/%40tests/1.in", Kind: "test-input"},
				},
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out State
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip mismatch\nwant: %#v\ngot:  %#v", in, out)
	}
}
