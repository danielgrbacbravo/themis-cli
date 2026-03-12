package discovery

import "testing"

func TestNormalizeTestsBaseURL_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "directory encoded without slash",
			input: "https://themis.housing.rug.nl/file/course/%40tests",
			want:  "https://themis.housing.rug.nl/file/course/%40tests",
		},
		{
			name:  "directory encoded with trailing slash",
			input: "https://themis.housing.rug.nl/file/course/%40tests/",
			want:  "https://themis.housing.rug.nl/file/course/%40tests",
		},
		{
			name:  "file input",
			input: "https://themis.housing.rug.nl/file/course/%40tests/1.in",
			want:  "https://themis.housing.rug.nl/file/course/%40tests",
		},
		{
			name:  "file input with query",
			input: "https://themis.housing.rug.nl/file/course/%40tests/2.out?raw=true",
			want:  "https://themis.housing.rug.nl/file/course/%40tests",
		},
		{
			name:  "directory literal at sign",
			input: "https://themis.housing.rug.nl/file/course/@tests",
			want:  "https://themis.housing.rug.nl/file/course/%40tests",
		},
		{
			name:  "file literal at sign",
			input: "https://themis.housing.rug.nl/file/course/@tests/3.in",
			want:  "https://themis.housing.rug.nl/file/course/%40tests",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeTestsBaseURL(tc.input)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected normalized URL.\nwant: %s\ngot:  %s", tc.want, got)
			}
		})
	}
}

func TestNormalizeTestsBaseURL_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "missing scheme and host",
			input: "/file/course/%40tests/1.in",
		},
		{
			name:  "unsupported scheme",
			input: "ftp://themis.housing.rug.nl/file/course/%40tests/1.in",
		},
		{
			name:  "missing tests directory",
			input: "https://themis.housing.rug.nl/file/course/1.in",
		},
		{
			name:  "invalid file extension",
			input: "https://themis.housing.rug.nl/file/course/%40tests/1.txt",
		},
		{
			name:  "invalid file name",
			input: "https://themis.housing.rug.nl/file/course/%40tests/foo.in",
		},
		{
			name:  "extra path segments after file",
			input: "https://themis.housing.rug.nl/file/course/%40tests/1.in/more",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeTestsBaseURL(tc.input)
			if err == nil {
				t.Fatalf("expected error, got normalized URL: %s", got)
			}
		})
	}
}
