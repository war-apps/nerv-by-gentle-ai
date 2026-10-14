package shipplan

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKey = "oy2-super-secret-key"

// received is one request a fake feed got.
type received struct {
	method, path, key, part, file string
	body                          []byte
}

// fakeFeed is a NuGet feed: GET /v3/index.json answers a service index
// pointing at /api/v2/package (and /api/v2/symbolpackage when symbols is
// set); PUTs are recorded and answered with status.
type fakeFeed struct {
	mu      sync.Mutex
	got     []received
	status  int
	body    string
	symbols bool
	// statuses answer the PUTs in order; later PUTs get status.
	statuses []int
	puts     int
}

func (f *fakeFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := received{method: r.Method, path: r.URL.Path, key: r.Header.Get("X-NuGet-ApiKey")}
	if r.Method == http.MethodGet && r.URL.Path == "/v3/index.json" {
		base := "http://" + r.Host
		res := `{"@type":"PackagePublish/2.0.0","@id":"` + base + `/api/v2/package"}`
		if f.symbols {
			res += `,{"@type":"SymbolPackagePublish/4.9.0","@id":"` + base + `/api/v2/symbolpackage"}`
		}
		f.record(rec)
		io.WriteString(w, `{"version":"3.0.0","resources":[{"@type":"SearchQueryService","@id":"`+base+`/q"},`+res+`]}`)
		return
	}
	if r.Method == http.MethodGet {
		// Not part of the protocol: another local process probing the new
		// loopback port (observed on a shared host). Not recorded.
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && strings.HasPrefix(mt, "multipart/") {
		mr := multipart.NewReader(r.Body, params["boundary"])
		if p, err := mr.NextPart(); err == nil {
			rec.part, rec.file = p.FormName(), p.FileName()
			rec.body, _ = io.ReadAll(p)
		}
	}
	f.record(rec)
	status := f.status
	f.mu.Lock()
	if f.puts < len(f.statuses) {
		status = f.statuses[f.puts]
	}
	f.puts++
	f.mu.Unlock()
	if status == 0 {
		status = http.StatusCreated
	}
	w.WriteHeader(status)
	io.WriteString(w, f.body)
}

func (f *fakeFeed) record(r received) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, r)
}

// packages writes the named files (content = name) to a temp dir.
func packages(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("zip of "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPushNuGet_V3IndexAndSymbols(t *testing.T) {
	feed := &fakeFeed{symbols: true}
	srv := httptest.NewServer(feed)
	defer srv.Close()
	dir := packages(t, "Sdk.1.0.0.nupkg", "Sdk.1.0.0.snupkg", "Other.2.0.0.nupkg", "notes.txt")

	summary, err := pushNuGet(context.Background(), nil, dir, srv.URL+"/v3/index.json", testKey)
	if err != nil {
		t.Fatalf("pushNuGet = %v", err)
	}
	want := []received{
		{method: "GET", path: "/v3/index.json"},
		{method: "PUT", path: "/api/v2/package/", key: testKey, part: "package", file: "package.nupkg", body: []byte("zip of Other.2.0.0.nupkg")},
		{method: "PUT", path: "/api/v2/package/", key: testKey, part: "package", file: "package.nupkg", body: []byte("zip of Sdk.1.0.0.nupkg")},
		{method: "PUT", path: "/api/v2/symbolpackage/", key: testKey, part: "package", file: "package.nupkg", body: []byte("zip of Sdk.1.0.0.snupkg")},
	}
	if len(feed.got) != len(want) {
		t.Fatalf("requests = %+v", feed.got)
	}
	for i, w := range want {
		g := feed.got[i]
		if g.method != w.method || g.path != w.path || g.key != w.key || g.part != w.part || g.file != w.file || string(g.body) != string(w.body) {
			t.Errorf("request %d = %+v\nwant %+v", i, g, w)
		}
	}
	if !strings.Contains(summary, "pushed: Other.2.0.0.nupkg, Sdk.1.0.0.nupkg, Sdk.1.0.0.snupkg") || strings.Contains(summary, testKey) {
		t.Errorf("summary = %q", summary)
	}
}

func TestPushNuGet_V2URLDuplicatesAndFailures(t *testing.T) {
	feed := &fakeFeed{status: http.StatusConflict}
	srv := httptest.NewServer(feed)
	defer srv.Close()
	dir := packages(t, "Sdk.1.0.0.nupkg", "Sdk.1.0.0.snupkg")
	summary, err := pushNuGet(context.Background(), nil, dir, srv.URL+"/api/v2/package", testKey)
	if err != nil {
		t.Fatalf("a duplicate is skipped, got %v", err)
	}
	if len(feed.got) != 1 || feed.got[0].path != "/api/v2/package/" {
		t.Errorf("requests = %+v (a v2 source gets no symbol push)", feed.got)
	}
	if !strings.Contains(summary, "already on the feed: Sdk.1.0.0.nupkg") {
		t.Errorf("summary = %q", summary)
	}

	// The feed echoes the key in its error: the error hides it.
	feed.status, feed.body = http.StatusForbidden, "The API key '"+testKey+"' is invalid"
	_, err = pushNuGet(context.Background(), nil, dir, srv.URL+"/api/v2/package", testKey)
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "Sdk.1.0.0.nupkg") || strings.Contains(err.Error(), testKey) {
		t.Errorf("pushNuGet = %v, want a 403 error without the key", err)
	}
}

func TestPushNuGet_NeverSendsTheKeyElsewhere(t *testing.T) {
	other := &fakeFeed{}
	elsewhere := httptest.NewServer(other)
	defer elsewhere.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	dir := packages(t, "Sdk.1.0.0.nupkg")

	_, err := pushNuGet(context.Background(), nil, dir, redirect.URL+"/api/v2/package", testKey)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("redirected push = %v, want an error", err)
	}
	if len(other.got) != 0 {
		t.Errorf("the redirect was followed: %+v", other.got)
	}

	for _, source := range []string{"http://nuget.example.com/api/v2/package", "ftp://x/y", "not a url"} {
		_, err := pushNuGet(context.Background(), nil, dir, source, testKey)
		if err == nil || strings.Contains(err.Error(), testKey) {
			t.Errorf("source %q: pushNuGet = %v, want a refusal", source, err)
		}
	}
}

func TestPushNuGet_Errors(t *testing.T) {
	srv := httptest.NewServer(&fakeFeed{})
	defer srv.Close()
	if _, err := pushNuGet(context.Background(), nil, packages(t), srv.URL+"/api/v2/package", testKey); err == nil || !strings.Contains(err.Error(), "no *.nupkg") {
		t.Errorf("no packages: %v", err)
	}
	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"version":"3.0.0","resources":[]}`)
	}))
	defer index.Close()
	if _, err := pushNuGet(context.Background(), nil, packages(t, "A.1.0.0.nupkg"), index.URL+"/v3/index.json", testKey); err == nil || !strings.Contains(err.Error(), "PackagePublish") {
		t.Errorf("index without PackagePublish: %v", err)
	}
	locked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer locked.Close()
	if _, err := pushNuGet(context.Background(), nil, packages(t, "A.1.0.0.nupkg"), locked.URL+"/v3/index.json", testKey); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("index needing credentials: %v", err)
	}
}

func TestPushNuGet_FailurePartWayNamesWhatTheFeedAccepted(t *testing.T) {
	feed := &fakeFeed{symbols: true, statuses: []int{201, 201, 409, 500}, body: "boom"}
	srv := httptest.NewServer(feed)
	defer srv.Close()
	dir := packages(t, "A.1.0.0.nupkg", "A.1.0.0.snupkg", "B.1.0.0.nupkg", "C.1.0.0.nupkg", "D.1.0.0.nupkg")
	_, err := pushNuGet(context.Background(), nil, dir, srv.URL+"/v3/index.json", testKey)
	want := "pushed: A.1.0.0.nupkg, A.1.0.0.snupkg; already on the feed: B.1.0.0.nupkg; failed on C.1.0.0.nupkg: HTTP 500 Internal Server Error: boom; not attempted: D.1.0.0.nupkg"
	if err == nil || err.Error() != want {
		t.Fatalf("pushNuGet error =\n%v\nwant\n%s", err, want)
	}

	feed2 := &fakeFeed{statuses: []int{500}, body: "down"}
	srv2 := httptest.NewServer(feed2)
	defer srv2.Close()
	_, err = pushNuGet(context.Background(), nil, packages(t, "A.1.0.0.nupkg"), srv2.URL+"/api/v2/package", testKey)
	if want := "nothing pushed; failed on A.1.0.0.nupkg: HTTP 500 Internal Server Error: down"; err == nil || err.Error() != want {
		t.Errorf("pushNuGet error = %v, want %q", err, want)
	}
}

func TestPushNuGet_KeyStraddlingTheErrorCutIsHidden(t *testing.T) {
	for _, before := range []int{500, 508, 511, 0} {
		feed := &fakeFeed{status: http.StatusForbidden, body: strings.Repeat("x", before) + testKey + strings.Repeat("y", 600)}
		srv := httptest.NewServer(feed)
		_, err := pushNuGet(context.Background(), nil, packages(t, "A.1.0.0.nupkg"), srv.URL+"/api/v2/package", testKey)
		srv.Close()
		if err == nil {
			t.Fatalf("key at %d: want an error", before)
		}
		for n := 4; n <= len(testKey); n++ {
			if strings.Contains(err.Error(), testKey[:n]) {
				t.Errorf("key at %d: the error shows the key's first %d characters: %v", before, n, err)
				break
			}
		}
	}
}

func TestUploadRequest_BodyParsesAndRewinds(t *testing.T) {
	dir := packages(t, "Sdk.1.0.0.nupkg")
	pkg := filepath.Join(dir, "Sdk.1.0.0.nupkg")
	req, err := uploadRequest(context.Background(), "https://feed.example/api/v2/package/", pkg, testKey)
	if err != nil {
		t.Fatal(err)
	}
	first, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	req.Body.Close()
	if int64(len(first)) != req.ContentLength {
		t.Errorf("ContentLength %d, body %d bytes", req.ContentLength, len(first))
	}
	if req.GetBody == nil {
		t.Fatal("no GetBody: net/http cannot retry the upload on a closed keep-alive connection")
	}
	again, err := req.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := io.ReadAll(again)
	again.Close()
	if string(second) != string(first) {
		t.Error("GetBody gave different bytes")
	}

	// The hand-built closing boundary makes a well-formed multipart body.
	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(strings.NewReader(string(first)), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(part)
	if part.FormName() != "package" || part.FileName() != "package.nupkg" || string(content) != "zip of Sdk.1.0.0.nupkg" {
		t.Errorf("part %q %q %q", part.FormName(), part.FileName(), content)
	}
	if _, err := mr.NextPart(); err != io.EOF {
		t.Errorf("after the one part: %v, want io.EOF (a closed body)", err)
	}
	if req.Header.Get("X-NuGet-ApiKey") != testKey || req.Method != http.MethodPut {
		t.Error("method or key header missing")
	}
}

func TestPushNuGet_DirectoryNameWithGlobCharacters(t *testing.T) {
	feed := &fakeFeed{}
	srv := httptest.NewServer(feed)
	defer srv.Close()
	dir := filepath.Join(t.TempDir(), "art[1]", "nuget")
	if err := os.MkdirAll(filepath.Join(dir, "Dir.1.0.0.nupkg"), 0o755); err != nil { // a directory, not a package
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Sdk.1.0.0.nupkg"), []byte("zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := pushNuGet(context.Background(), nil, dir, srv.URL+"/api/v2/package", testKey)
	if err != nil || summary != "pushed: Sdk.1.0.0.nupkg" {
		t.Errorf("pushNuGet = %q, %v; want the one package pushed", summary, err)
	}
}

func TestPushNuGet_InjectedClientBoundsAHangingFeed(t *testing.T) {
	release := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer hang.Close()
	defer close(release)
	start := time.Now()
	_, err := pushNuGet(context.Background(), &http.Client{Timeout: 200 * time.Millisecond}, packages(t, "A.1.0.0.nupkg"), hang.URL+"/api/v2/package", testKey)
	if err == nil || !strings.Contains(err.Error(), "failed on A.1.0.0.nupkg") {
		t.Errorf("pushNuGet = %v, want the timeout as the failure", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s: the client's timeout was not used", d)
	}
	if defaultPushClient().Timeout != nugetPushTimeout || nugetPushTimeout <= 0 {
		t.Errorf("the default client must bound each request")
	}
}

func TestUploadRequest_SendsOnlyTheHeadersTheNuGetClientSends(t *testing.T) {
	req, err := uploadRequest(context.Background(), "https://feed.example/x/", filepath.Join(packages(t, "A.1.0.0.nupkg"), "A.1.0.0.nupkg"), testKey)
	if err != nil {
		t.Fatal(err)
	}
	req.Body.Close()
	if req.Header.Get("X-NuGet-Protocol-Version") != "" {
		t.Error("X-NuGet-Protocol-Version is sent; the NuGet 7.0.3 client does not send it on push (captured)")
	}
}
