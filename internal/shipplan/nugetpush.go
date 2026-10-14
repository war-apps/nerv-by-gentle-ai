package shipplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The nuget preset pushes packages itself, over NuGet's push protocol
// (https://learn.microsoft.com/nuget/api/package-publish-resource), so the
// API key only ever travels in the X-NuGet-ApiKey request header: never on
// a command line. `dotnet nuget push --api-key` puts it on argv, and every
// way around that fails on Linux (observed with the .NET 10.0.112 SDK,
// NuGet 7.0.3): a clear-text <apikeys> entry in a NuGet.Config is refused
// ("encryption is not supported on non-Windows platforms": NuGet always
// DPAPI-decrypts it), a response file given to `dotnet nuget` is expanded
// and forwarded as argv to the child `dotnet exec NuGet.CommandLine.XPlat.dll
// ... --api-key <key>`, NuGet.CommandLine.XPlat.dll does not read response
// files itself, and NuGet reads no API key from the environment.

// nugetPushTimeout bounds one HTTP request of a push (an upload included).
const nugetPushTimeout = 10 * time.Minute

const (
	// errorBodyReadLimit bounds how much of a feed's error body is read:
	// enough to find the key anywhere a feed would echo it.
	errorBodyReadLimit = 64 << 10
	// errorBodyShowLimit bounds the part of it shown in an error: a feed's
	// message is short, an HTML error page is not worth more.
	errorBodyShowLimit = 512
)

// pushNuGet pushes every *.nupkg in dir, in name order, to source with key,
// each followed by its .snupkg when the source's service index offers a
// SymbolPackagePublish resource. source is a v3 service index (…/index.json,
// whose PackagePublish resource is used) or a push URL (a v2 feed). A 409
// answer means the version is already there and is skipped, like
// --skip-duplicate. Redirects are not followed (the key would go with
// them), and plain http is refused except on a loopback host. The key never
// appears in the summary or in errors. A failure part-way names what the
// feed already accepted, what it already had, the package that failed and
// those not attempted (a push cannot be undone). client nil means a
// default client.
func pushNuGet(ctx context.Context, client *http.Client, dir, source, key string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: nugetPushTimeout}
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	p := &nugetPush{client: &c, key: key}

	pkgs, err := filepath.Glob(filepath.Join(dir, "*.nupkg"))
	if err != nil || len(pkgs) == 0 {
		return "", fmt.Errorf("no *.nupkg in %s to push", dir)
	}
	slices.Sort(pkgs)
	publish, symbols, err := p.endpoints(ctx, source)
	if err != nil {
		return "", p.redact(err)
	}
	type upload struct{ path, url string }
	var uploads []upload
	for _, pkg := range pkgs {
		uploads = append(uploads, upload{pkg, publish})
		if snupkg := strings.TrimSuffix(pkg, ".nupkg") + ".snupkg"; symbols != "" && fileExists(snupkg) {
			uploads = append(uploads, upload{snupkg, symbols})
		}
	}
	var pushed, present []string
	report := func() []string {
		var parts []string
		if len(pushed) > 0 {
			parts = append(parts, "pushed: "+strings.Join(pushed, ", "))
		}
		if len(present) > 0 {
			parts = append(parts, "already on the feed: "+strings.Join(present, ", "))
		}
		return parts
	}
	for i, u := range uploads {
		dup, err := p.put(ctx, u.url, u.path)
		if err != nil {
			// Name what the feed already holds: a failed push is not undone.
			parts := report()
			if len(parts) == 0 {
				parts = []string{"nothing pushed"}
			}
			parts = append(parts, fmt.Sprintf("failed on %s: %v", filepath.Base(u.path), err))
			var rest []string
			for _, r := range uploads[i+1:] {
				rest = append(rest, filepath.Base(r.path))
			}
			if len(rest) > 0 {
				parts = append(parts, "not attempted: "+strings.Join(rest, ", "))
			}
			return "", p.redact(errors.New(strings.Join(parts, "; ")))
		}
		if dup {
			present = append(present, filepath.Base(u.path))
		} else {
			pushed = append(pushed, filepath.Base(u.path))
		}
	}
	return strings.Join(report(), "; "), nil
}

type nugetPush struct {
	client *http.Client
	key    string
}

// redact hides the key in an error (a feed may echo it).
func (p *nugetPush) redact(err error) error {
	if p.key == "" || !strings.Contains(err.Error(), p.key) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), p.key, "***"))
}

// endpoints returns the package and symbol push URLs of source.
func (p *nugetPush) endpoints(ctx context.Context, source string) (publish, symbols string, err error) {
	if err := checkPushURL(source); err != nil {
		return "", "", err
	}
	u, _ := url.Parse(source)
	if !strings.HasSuffix(strings.ToLower(u.Path), "index.json") {
		return withSlash(source), "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("reading the service index %s: %w", source, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("reading the service index %s: HTTP %s (a feed whose index needs credentials is not supported by the nuget preset; use a publish stage override)", source, resp.Status)
	}
	var index struct {
		Resources []struct {
			ID   string `json:"@id"`
			Type string `json:"@type"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&index); err != nil {
		return "", "", fmt.Errorf("reading the service index %s: %w", source, err)
	}
	for _, r := range index.Resources {
		switch {
		case publish == "" && strings.HasPrefix(r.Type, "PackagePublish/"):
			publish = r.ID
		case symbols == "" && strings.HasPrefix(r.Type, "SymbolPackagePublish/"):
			symbols = r.ID
		}
	}
	if publish == "" {
		return "", "", fmt.Errorf("the service index %s has no PackagePublish resource", source)
	}
	for _, e := range []string{publish, symbols} {
		if e == "" {
			continue
		}
		if err := checkPushURL(e); err != nil {
			return "", "", fmt.Errorf("the service index %s: %w", source, err)
		}
	}
	if symbols != "" {
		symbols = withSlash(symbols)
	}
	return withSlash(publish), symbols, nil
}

// checkPushURL refuses a URL the key must not be sent to: not http(s), or
// plain http to a host other than loopback.
func checkPushURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%q is not an http(s) NuGet source", raw)
	}
	if u.Scheme == "http" && !loopback(u.Hostname()) {
		return fmt.Errorf("%s is plain http: the API key would travel unencrypted (only loopback hosts may use http)", raw)
	}
	return nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// withSlash ends a push URL with "/", as the NuGet client does.
func withSlash(u string) string {
	if strings.HasSuffix(u, "/") {
		return u
	}
	return u + "/"
}

// put uploads one package (uploadRequest) and reports a 409 (the version
// is already there).
func (p *nugetPush) put(ctx context.Context, target, path string) (duplicate bool, err error) {
	req, err := uploadRequest(ctx, target, path, p.key)
	if err != nil {
		return false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusConflict:
		return true, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return false, fmt.Errorf("HTTP %s: the feed answered with a redirect to %q, which is not followed so the API key goes nowhere else", resp.Status, resp.Header.Get("Location"))
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyReadLimit))
	return false, fmt.Errorf("HTTP %s: %s", resp.Status, p.errorText(string(msg)))
}

// errorText is a feed's error body as shown in an error: the key redacted
// in the whole body first, then cut to errorBodyShowLimit bytes, and a
// trailing start of the key (4 characters or more) the cut left behind
// redacted too, so no part of the key is shown.
func (p *nugetPush) errorText(body string) string {
	if p.key != "" {
		body = strings.ReplaceAll(body, p.key, "***")
	}
	if len(body) > errorBodyShowLimit {
		body = body[:errorBodyShowLimit]
		for n := min(len(p.key)-1, len(body)); n >= 4; n-- {
			if strings.HasSuffix(body, p.key[:n]) {
				body = body[:len(body)-n] + "***"
				break
			}
		}
	}
	return strings.TrimSpace(body)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// uploadRequest builds the upload of one package as the NuGet client sends
// it: a PUT of a multipart/form-data body whose one file part is named
// "package", with the key in the X-NuGet-ApiKey header.
//
// The body is streamed from the file rather than built in memory
// (packages can be large), and it has a known ContentLength (some feeds
// refuse chunked uploads). multipart.Writer only writes its closing
// boundary when the whole part went through it, so the part's header comes
// from the Writer and the closing boundary ("\r\n--<boundary>--\r\n") is
// written by hand around the file. GetBody reopens the file and rebuilds
// the same bytes, so net/http can resend the request when a reused
// keep-alive connection turns out to be closed.
func uploadRequest(ctx context.Context, target, path, key string) (*http.Request, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="package"; filename="package.nupkg"`)
	h.Set("Content-Type", "application/octet-stream")
	if _, err := mw.CreatePart(h); err != nil {
		return nil, err
	}
	tail := "\r\n--" + mw.Boundary() + "--\r\n"
	body := func() (io.ReadCloser, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		r := io.MultiReader(bytes.NewReader(head.Bytes()), f, strings.NewReader(tail))
		return struct {
			io.Reader
			io.Closer
		}{r, f}, nil
	}
	first, err := body()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, first)
	if err != nil {
		first.Close()
		return nil, err
	}
	req.GetBody = body
	req.ContentLength = int64(head.Len()) + info.Size() + int64(len(tail))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-NuGet-ApiKey", key)
	req.Header.Set("X-NuGet-Protocol-Version", "4.1.0")
	return req, nil
}
