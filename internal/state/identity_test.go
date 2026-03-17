package state

import "testing"

func TestCanonicalizeURL_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trim whitespace and strip query fragment",
			input: " https://themis.housing.rug.nl/course/2025-2026/os/?x=1#frag ",
			want:  "https://themis.housing.rug.nl/course/2025-2026/os",
		},
		{
			name:  "keep root slash",
			input: "https://themis.housing.rug.nl/",
			want:  "https://themis.housing.rug.nl/",
		},
		{
			name:  "already canonical",
			input: "https://themis.housing.rug.nl/course",
			want:  "https://themis.housing.rug.nl/course",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := CanonicalizeURL(tc.input)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected canonical URL. want=%q got=%q", tc.want, got)
			}
		})
	}
}

func TestCanonicalizeURL_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "relative", input: "/course/2025-2026/os"},
		{name: "unsupported scheme", input: "ftp://themis.housing.rug.nl/course"},
		{name: "invalid parse", input: "https://%zz"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, err := CanonicalizeURL(tc.input); err == nil {
				t.Fatalf("expected error, got canonical URL: %q", got)
			}
		})
	}
}

func TestNodeIDFromURL_Deterministic(t *testing.T) {
	id, canonical, err := NodeIDFromURL("https://themis.housing.rug.nl/course/2025-2026/os/")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if canonical != "https://themis.housing.rug.nl/course/2025-2026/os" {
		t.Fatalf("unexpected canonical URL: %s", canonical)
	}

	const wantID = "url:e51a8c8fa915b6e24cc85d4b992b1962b81b6170c294bfae64e516e416eb3145"
	if id != wantID {
		t.Fatalf("unexpected node id. want=%s got=%s", wantID, id)
	}
}
