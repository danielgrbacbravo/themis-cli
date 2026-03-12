package themis

import "testing"

func TestNormalizeBaseURL_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https no path",
			input: "https://themis.housing.rug.nl",
			want:  "https://themis.housing.rug.nl",
		},
		{
			name:  "https trailing slash",
			input: "https://themis.housing.rug.nl/",
			want:  "https://themis.housing.rug.nl",
		},
		{
			name:  "path trailing slash removed",
			input: "https://themis.housing.rug.nl/base/",
			want:  "https://themis.housing.rug.nl/base",
		},
		{
			name:  "query and fragment removed",
			input: "https://themis.housing.rug.nl/base/?x=1#frag",
			want:  "https://themis.housing.rug.nl/base",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeBaseURL(tc.input)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected normalized URL.\nwant: %s\ngot:  %s", tc.want, got)
			}
		})
	}
}

func TestNormalizeBaseURL_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "missing host",
			input: "https:///foo",
		},
		{
			name:  "unsupported scheme",
			input: "ftp://themis.housing.rug.nl",
		},
		{
			name:  "no scheme",
			input: "themis.housing.rug.nl",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeBaseURL(tc.input); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
