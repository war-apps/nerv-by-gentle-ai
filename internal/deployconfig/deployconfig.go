// Package deployconfig reads the deploy: section of a repository's
// .ai-workflow/config.yaml, the configuration of `nerv deploy`:
//
//	deploy:
//	  host: github                 # the git host provider (release-<name>)
//	  publish: true                # push the artifact's images (default true)
//	  targets:                     # optional; deployed in this order
//	    - name: staging
//	      provider: ssh-compose    # a target provider (release-<name>)
//	      config: { environment: staging }
//	  git:
//	    commit_message: "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}"
//	    remote: origin
//
// Like internal/releaseconfig it cuts the top-level block out of the file and
// parses it strictly: unknown keys are refused. Which config keys a target
// accepts is its provider manifest's business (internal/deployrun).
package deployconfig

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
)

// ErrNotConfigured is returned by Load when the repository has no
// .ai-workflow/config.yaml or the file has no deploy: key.
var ErrNotConfigured = errors.New("deploy: not configured in .ai-workflow/config.yaml")

// DefaultCommitMessage is @semantic-release/git's default release commit
// message.
const DefaultCommitMessage = "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}"

// DefaultRemote is the remote deploy clones and pushes to.
const DefaultRemote = "origin"

// Deploy is the deploy: section.
type Deploy struct {
	// Host is the git host provider's name; empty creates no release.
	Host string `yaml:"host"`
	// Publish makes nerv deploy push the artifact's images itself, in its
	// publish step; nil means true (Publishes).
	Publish *bool `yaml:"publish"`
	// Targets are deployed in order; none releases without deploying.
	Targets []Target `yaml:"targets"`
	Git     Git      `yaml:"git"`
}

// Publishes reports whether nerv deploy pushes the artifact's images in its
// own publish step (deploy.publish, default true).
func (d *Deploy) Publishes() bool { return d.Publish == nil || *d.Publish }

// Target is one deploy target.
type Target struct {
	// Name identifies the target (--target). Required, unique.
	Name string `yaml:"name"`
	// Provider is the target provider's name. Required.
	Provider string `yaml:"provider"`
	// Config are the provider's config keys for this target.
	Config map[string]string `yaml:"config"`
}

// Git configures the release commit and the push.
type Git struct {
	// CommitMessage is the release commit's message, with the placeholders
	// of Placeholders. Default: DefaultCommitMessage.
	CommitMessage string `yaml:"commit_message"`
	// Remote is the remote cloned and pushed to. Default: DefaultRemote.
	Remote string `yaml:"remote"`
}

// Release is what a commit message is rendered with.
type Release struct {
	Version, Tag, Channel, Notes string
}

// Placeholders are the commit message placeholders: @semantic-release/git's
// lodash ones and their short forms.
var Placeholders = []string{
	"${nextRelease.version}", "${nextRelease.gitTag}", "${nextRelease.channel}", "${nextRelease.notes}",
	"${version}", "${tag}", "${channel}", "${notes}",
}

var (
	nameRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	providerRe    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	keyRe         = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	remoteRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	placeholderRe = regexp.MustCompile(`\$\{[^}]*\}`)
)

// CommitMessage renders message for r in one pass, so the notes are never
// expanded themselves.
func CommitMessage(message string, r Release) string {
	values := map[string]string{
		"${nextRelease.version}": r.Version, "${nextRelease.gitTag}": r.Tag, "${nextRelease.channel}": r.Channel, "${nextRelease.notes}": r.Notes,
		"${version}": r.Version, "${tag}": r.Tag, "${channel}": r.Channel, "${notes}": r.Notes,
	}
	return placeholderRe.ReplaceAllStringFunc(message, func(p string) string {
		if v, ok := values[p]; ok {
			return v
		}
		return p
	})
}

// Load reads the deploy: section of repoRoot's .ai-workflow/config.yaml.
func Load(repoRoot string) (*Deploy, error) {
	if err := paths.RefusePreviousProject(repoRoot); err != nil {
		return nil, err
	}
	path := paths.ProjectConfig(repoRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}
	d, err := parse(data)
	if err != nil && !errors.Is(err, ErrNotConfigured) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, err
}

func parse(data []byte) (*Deploy, error) {
	if msg := releaseconfig.RemovedPipeline(data); msg != "" {
		return nil, errors.New(msg)
	}
	node, err := yamlsection.Locate(data, "deploy")
	var problem *yamlsection.ProblemError
	switch {
	case errors.Is(err, yamlsection.ErrAbsent):
		return nil, ErrNotConfigured
	case errors.As(err, &problem):
		return nil, errors.New(problem.Problem)
	case err != nil:
		return nil, err
	}
	var d Deploy
	if node.ShortTag() != "!!null" {
		unknown, err := yamlsection.UnknownFields(node, reflect.TypeFor[Deploy](), "deploy")
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			return nil, errors.New(strings.Join(unknown, "; "))
		}
		if err := node.Decode(&d); err != nil {
			return nil, errors.New(strings.Join(yamlsection.DecodeProblems("deploy", err), "; "))
		}
	}
	if d.Git.CommitMessage == "" {
		d.Git.CommitMessage = DefaultCommitMessage
	}
	if d.Git.Remote == "" {
		d.Git.Remote = DefaultRemote
	}
	if problems := d.validate(); len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "; "))
	}
	return &d, nil
}

func (d *Deploy) validate() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	if d.Host != "" && !providerRe.MatchString(d.Host) {
		add("deploy.host %q must be a provider name (lower-case letters, digits and dashes)", d.Host)
	}
	seen := map[string]int{}
	for i, t := range d.Targets {
		at := fmt.Sprintf("deploy.targets[%d]", i)
		if !nameRe.MatchString(t.Name) {
			add("%s.name %q must be lower-case letters, digits, ., _ and -", at, t.Name)
		} else if j, dup := seen[t.Name]; dup {
			add("%s.name: %q is also targets[%d]", at, t.Name, j)
		} else {
			seen[t.Name] = i
		}
		switch {
		case t.Provider == "":
			add("%s.provider: required", at)
		case !providerRe.MatchString(t.Provider):
			add("%s.provider %q must be a provider name", at, t.Provider)
		}
		for key := range t.Config {
			if !keyRe.MatchString(key) {
				add("%s.config.%s: a key is lower-case letters, digits and underscores", at, key)
			}
		}
	}
	for _, ph := range placeholderRe.FindAllString(d.Git.CommitMessage, -1) {
		known := false
		for _, k := range Placeholders {
			known = known || k == ph
		}
		if !known {
			add("deploy.git.commit_message: unknown placeholder %s (known: %s)", ph, strings.Join(Placeholders, ", "))
		}
	}
	if !remoteRe.MatchString(d.Git.Remote) {
		add("deploy.git.remote %q must be a remote name", d.Git.Remote)
	}
	return p
}
