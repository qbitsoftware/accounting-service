package accounting

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

// ebWire is one request as Excellent Books received it. The query and the form
// body are both built by url.Values.Encode on our side, which sorts the keys,
// so they are kept raw: byte for byte, including how a space is encoded (%20,
// never + — Standard Books stores a literal +).
type ebWire struct {
	Method string
	Path   string // after /api/<company>/, e.g. "IPVc" or "IPVc/300001"
	Query  string
	Body   string
}

func (w ebWire) String() string {
	s := w.Method + " " + w.Path
	if w.Query != "" {
		s += "?" + w.Query
	}
	if w.Body != "" {
		s += " " + w.Body
	}
	return s
}

// ebFake is a scripted Excellent Books server. respond answers each request; a
// nil respond, or an empty body from it, answers 200 with an empty receipt
// envelope.
type ebFake struct {
	mu      sync.Mutex
	reqs    []ebWire
	respond func(n int, w ebWire) (status int, body string)
}

func (f *ebFake) wire() []ebWire {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ebWire(nil), f.reqs...)
}

const ebEmptyReceipts = `{"data":{"@register":"IPVc","@sequence":"1","IPVc":[]}}`

// newEBFake returns a provider talking to a local server that records every
// request and answers it with respond.
func newEBFake(t *testing.T, respond func(n int, w ebWire) (int, string)) (*excellentProvider, *ebFake) {
	t.Helper()
	f := &ebFake{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w := ebWire{
			Method: r.Method,
			Path:   strings.TrimPrefix(r.URL.EscapedPath(), "/api/1/"),
			Query:  r.URL.RawQuery,
			Body:   string(body),
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
			t.Errorf("request %s without the configured basic auth", w)
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded; charset=UTF-8" {
				t.Errorf("request %s Content-Type = %q", w, ct)
			}
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
			out = ebEmptyReceipts
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	return &excellentProvider{client: excellentbooks.New(excellentbooks.Config{
		BaseURL:     srv.URL,
		CompanyCode: "1",
		Username:    "u",
		Password:    "p",
		HTTPClient:  srv.Client(),
	})}, f
}

// assertEBWire compares every recorded request with the expected wire, one
// line per request, and prints both in full on a mismatch so a deliberate
// change can be read off the failure.
func assertEBWire(t *testing.T, f *ebFake, want ...string) {
	t.Helper()
	got := f.wire()
	lines := make([]string, len(got))
	for i, w := range got {
		lines[i] = w.String()
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("wire changed\n got:\n  %s\n want:\n  %s", strings.Join(lines, "\n  "), strings.Join(want, "\n  "))
	}
}

func ebDay(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// ebCreated is what EB answers a POST to IPVc with: the saved record.
func ebCreated(serNr string) string {
	return `{"data":{"@register":"IPVc","@sequence":"9001","IPVc":[{"SerNr":"` + serNr + `","@sequence":"9001","OKFlag":"1"}]}}`
}
