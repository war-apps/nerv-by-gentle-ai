package ecosystem

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade"
	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade/selfupgradetest"
)

// stubTransport answers any request whose URL matches want with a 200 JSON
// body, and fails the test on any other URL, so a test proves Latest built
// exactly that URL without a real network call.
type stubTransport struct {
	t    *testing.T
	want string
	body string
}

func (s stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != s.want {
		s.t.Fatalf("unexpected request URL %s, want %s", req.URL.String(), s.want)
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

func TestLatest_ResolvesStableFromFakeServer(t *testing.T) {
	srv := selfupgradetest.New(t, "nerv-task",
		selfupgradetest.Release{Tag: "v1.1.0"},
		selfupgradetest.Release{Tag: "v1.0.1"},
	)
	tool := testTool()
	client := selfupgrade.Client{HTTP: srv.Client(), APIBase: srv.APIBase(), DownloadBase: srv.DownloadBase()}

	got, err := Latest(context.Background(), tool, client)
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if got != "v1.1.0" {
		t.Errorf("Latest() = %q, want v1.1.0", got)
	}
}

func TestLatest_DefaultsAPIBaseFromToolRepo(t *testing.T) {
	// No APIBase override: Latest must build it from t.Repo rather than
	// hitting a blank URL. The transport fails the test if Latest asks
	// any other URL, proving the default was derived, not left empty.
	tool := testTool()
	body, _ := json.Marshal(map[string]any{"tag_name": "v1.1.0"})
	client := selfupgrade.Client{HTTP: &http.Client{Transport: stubTransport{
		t:    t,
		want: "https://api.github.com/repos/war-apps/nerv-task-by-gentle-ai/releases/latest",
		body: string(body),
	}}}

	got, err := Latest(context.Background(), tool, client)
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if got != "v1.1.0" {
		t.Errorf("Latest() = %q, want v1.1.0", got)
	}
}
