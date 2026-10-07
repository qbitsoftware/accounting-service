package accounting

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
)

// saWire is one request as SmartAccounts received it, normalised so it can be
// compared byte for byte across runs: the timestamp and the signature change on
// every request and are dropped, everything else — parameter order included —
// is kept exactly as sent.
type saWire struct {
	Method string
	Path   string // endpoint after /en/api/, e.g. "purchasesales/payments:add"
	Query  string // raw query minus timestamp= and signature=
	Body   string
}

func (w saWire) String() string {
	s := w.Method + " " + w.Path + "?" + w.Query
	if w.Body != "" {
		s += " " + w.Body
	}
	return s
}

// saFake is a scripted SmartAccounts server. respond answers each request; a
// nil respond, or an empty body from it, answers 200 with an empty object.
type saFake struct {
	mu      sync.Mutex
	reqs    []saWire
	respond func(n int, w saWire) (status int, body string)
}

func (f *saFake) wire() []saWire {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]saWire(nil), f.reqs...)
}

// newSAFake returns a provider talking to a local TLS server. Each test gets
// its own API key, so the process-wide limiter and 24h meter (both keyed per
// company) never carry state from one test into the next.
func newSAFake(t *testing.T, respond func(n int, w saWire) (int, string)) (*smartProvider, *saFake) {
	t.Helper()
	f := &saFake{respond: respond}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w := saWire{
			Method: r.Method,
			Path:   strings.TrimPrefix(r.URL.Path, "/en/api/"),
			Query:  normaliseSAQuery(r.URL.RawQuery),
			Body:   string(body),
		}
		f.mu.Lock()
		n := len(f.reqs)
		f.reqs = append(f.reqs, w)
		f.mu.Unlock()

		status, out := http.StatusOK, ""
		if f.respond != nil {
			status, out = f.respond(n, w)
		}
		if out == "" {
			out = "{}"
		}
		if status == http.StatusServiceUnavailable {
			rw.Header().Set("Retry-After", "1")
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	return &smartProvider{client: smartaccounts.New(smartaccounts.Config{
		Host:          strings.TrimPrefix(srv.URL, "https://"),
		APIKey:        "key-" + strings.ReplaceAll(t.Name(), "/", "-"),
		SecretKey:     "secret",
		HTTPClient:    srv.Client(),
		RatePerSecond: -1,
		NettingBank:   "Tasaarveldus",
	})}, f
}

func normaliseSAQuery(raw string) string {
	parts := strings.Split(raw, "&")
	kept := parts[:0]
	for _, p := range parts {
		if strings.HasPrefix(p, "timestamp=") || strings.HasPrefix(p, "signature=") {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "&")
}

// assertSAWire compares every recorded request with the expected wire, one
// line per request, and prints both in full on a mismatch so a deliberate change
// can be read off the failure.
func assertSAWire(t *testing.T, f *saFake, key string, want ...string) {
	t.Helper()
	got := f.wire()
	lines := make([]string, len(got))
	for i, w := range got {
		lines[i] = strings.ReplaceAll(w.String(), "apikey="+key, "apikey=KEY")
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("wire changed\n got:\n  %s\n want:\n  %s", strings.Join(lines, "\n  "), strings.Join(want, "\n  "))
	}
}

func saTestKey(t *testing.T) string { return "key-" + strings.ReplaceAll(t.Name(), "/", "-") }
