package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTestCasesWithOptions_RequiresBothFilesAndSupportsMissWindow(t *testing.T) {
	server, client := newTestServer(map[string]string{
		"/file/course/@tests/1.in":  "in1",
		"/file/course/@tests/1.out": "out1",
		"/file/course/@tests/2.in":  "in2",
		"/file/course/@tests/2.out": "out2",
		"/file/course/@tests/3.in":  "in3-only",
		"/file/course/@tests/4.in":  "in4",
		"/file/course/@tests/4.out": "out4",
	})
	defer server.Close()

	baseTestsURL, testCases, err := ListTestCasesWithOptions(client, server.URL+"/file/course/%40tests", ListOptions{
		Start:     1,
		Max:       10,
		MaxMisses: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if baseTestsURL != server.URL+"/file/course/%40tests" {
		t.Fatalf("unexpected normalized tests URL: %s", baseTestsURL)
	}

	numbers := ListTestNumbers(testCases)
	want := []int{1, 2, 4}
	if len(numbers) != len(want) {
		t.Fatalf("unexpected count: got=%d want=%d", len(numbers), len(want))
	}
	for i := range want {
		if numbers[i] != want[i] {
			t.Fatalf("unexpected test number at %d: got=%d want=%d", i, numbers[i], want[i])
		}
	}
}

func TestListTestCasesWithOptions_StartAndMaxBoundProbing(t *testing.T) {
	server, client := newTestServer(map[string]string{
		"/file/course/@tests/5.in":  "in5",
		"/file/course/@tests/5.out": "out5",
		"/file/course/@tests/6.in":  "in6",
		"/file/course/@tests/6.out": "out6",
		"/file/course/@tests/7.in":  "in7",
		"/file/course/@tests/7.out": "out7",
	})
	defer server.Close()

	_, testCases, err := ListTestCasesWithOptions(client, server.URL+"/file/course/%40tests/6.in", ListOptions{
		Start:     6,
		Max:       2,
		MaxMisses: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	numbers := ListTestNumbers(testCases)
	want := []int{6, 7}
	if len(numbers) != len(want) {
		t.Fatalf("unexpected count: got=%d want=%d", len(numbers), len(want))
	}
	for i := range want {
		if numbers[i] != want[i] {
			t.Fatalf("unexpected test number at %d: got=%d want=%d", i, numbers[i], want[i])
		}
	}
}

func TestListTestCasesWithOptions_InvalidOptions(t *testing.T) {
	server, client := newTestServer(map[string]string{})
	defer server.Close()

	_, _, err := ListTestCasesWithOptions(client, server.URL+"/file/course/%40tests", ListOptions{
		Start:     0,
		Max:       10,
		MaxMisses: 2,
	})
	if err == nil {
		t.Fatal("expected error for invalid start")
	}
}

func TestListTestCasesWithOptions_Treats403AsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file/course/@tests/1.in", "/file/course/@tests/1.out":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		case "/file/course/@tests/2.in", "/file/course/@tests/2.out":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("forbidden"))
			return
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<!doctype html><html><body>missing</body></html>"))
			return
		}
	}))
	defer server.Close()

	_, testCases, err := ListTestCasesWithOptions(server.Client(), server.URL+"/file/course/%40tests", ListOptions{
		Start:     1,
		Max:       5,
		MaxMisses: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	numbers := ListTestNumbers(testCases)
	if len(numbers) != 1 || numbers[0] != 1 {
		t.Fatalf("unexpected test numbers: %#v", numbers)
	}
}

func newTestServer(files map[string]string) (*httptest.Server, *http.Client) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if content, ok := files[r.URL.Path]; ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(content))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<!doctype html><html><body>missing</body></html>"))
	}))
	return server, server.Client()
}
