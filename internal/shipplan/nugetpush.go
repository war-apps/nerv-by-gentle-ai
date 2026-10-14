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

// pushNuGet pushes every *.nupkg in dir, in name order, to source with key,
// each followed by its .snupkg when the source's service index offers a
// SymbolPackagePublish resource. source is a v3 service index (…/index.json,
// whose PackagePublish resource is used) or a push URL (a v2 feed). A 409
// answer means the version is already there and is skipped, like
// --skip-duplicate. Redirects are not followed (the key would go with
// them), and plain http is refused except on a loopback host. The key never
// appears in the summary or in errors. client nil means a default client.
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
	var pushed, present []string
	for _, pkg := range pkgs {
		files := []struct{ path, url string }{{pkg, publish}}
		if snupkg := strings.TrimSuffix(pkg, ".nupkg") + ".snupkg"; symbols != "" && fileExists(snupkg) {
			files = append(files, struct{ path, url string }{snupkg, symbols})
		}
		for _, f := range files {
			dup, err := p.put(ctx, f.url, f.path)
			if err != nil {
				return "", p.redact(fmt.Errorf("pushing %s: %w", filepath.Base(f.path), err))
			}
			if dup {
				present = append(present, filepath.Base(f.path))
			} else {
				pushed = append(pushed, filepath.Base(f.path))
			}
		}
	}
	var parts []string
	if len(pushed) > 0 {
		parts = append(parts, "pushed "+strings.Join(pushed, ", "))
	}
	if len(present) > 0 {
		parts = append(parts, "already on the feed: "+strings.Join(present, ", "))
	}
	return strings.Join(parts, "; "), nil
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

// put uploads one package as NuGet does: a multipart/form-data PUT whose
// one file part is named "package". It reports a 409 (already there).
func (p *nugetPush) put(ctx context.Context, target, path string) (duplicate bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="package"; filename="package.nupkg"`)
	h.Set("Content-Type", "application/octet-stream")
	if _, err := mw.CreatePart(h); err != nil {
		return false, err
	}
	tail := "\r\n--" + mw.Boundary() + "--\r\n"
	body := io.MultiReader(bytes.NewReader(head.Bytes()), f, strings.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, body)
	if err != nil {
		return false, err
	}
	req.ContentLength = int64(head.Len()) + info.Size() + int64(len(tail))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-NuGet-ApiKey", p.key)
	req.Header.Set("X-NuGet-Protocol-Version", "4.1.0")
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
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return false, fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(msg)))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
