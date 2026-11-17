package ecosystem

import (
	"context"

	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade"
)

// Latest resolves t's newest stable release tag through a selfupgrade
// client: APIBase/DownloadBase/BinaryName default to t's own repository
// and binary name when client leaves them empty (tests override them with
// a fake server).
func Latest(ctx context.Context, t Tool, client selfupgrade.Client) (string, error) {
	if client.APIBase == "" {
		client.APIBase = "https://api.github.com/repos/" + t.Repo
	}
	if client.DownloadBase == "" {
		client.DownloadBase = "https://github.com/" + t.Repo + "/releases/download"
	}
	if client.BinaryName == "" {
		client.BinaryName = t.Binary
	}
	return client.Resolve(ctx, selfupgrade.Stable)
}
