package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// SSH runs commands on a target's Docker host, as update-deploy's remote():
// host keys checked strictly against KnownHosts; auth password through
// sshpass -e with the password in SSHPASS only (never on a command line),
// auth key with KeyFile in batch mode.
type SSH struct {
	Proc           proc.Runner
	Auth           string // password or key
	KeyFile        string
	KnownHosts     string
	Port           int
	ConnectTimeout time.Duration
	// Env is the base environment of ssh (without the deploy secrets);
	// auth password adds SSHPASS to it.
	Env []string
}

// Keepalives: a dead link fails the session after about 45s instead of
// hanging (the per-call deadlines bound the rest).
const (
	serverAliveInterval = 15
	serverAliveCountMax = 3
)

// Run runs the remote command on t's host and returns its standard output;
// with stream set, its output goes there live instead. The user, host and
// file values are checked first (see checkDestination): ssh would read a
// value starting with - as an option.
func (s SSH) Run(ctx context.Context, t Target, remote string, stream io.Writer) (string, error) {
	if err := checkDestination(t.User, t.Host); err != nil {
		return "", err
	}
	for _, f := range []struct{ what, path string }{{"ssh.known_hosts", s.KnownHosts}, {"ssh.key_file", s.KeyFile}} {
		if err := checkFile(f.what, f.path); err != nil {
			return "", err
		}
	}
	if s.Port < 1 || s.Port > 65535 {
		return "", fmt.Errorf("ssh.port %d is not a port", s.Port)
	}
	// Every value is an option argument (-o key="value", -p, -l, -i) and
	// the host comes after --, so none can be read as an option.
	args := []string{"-o", "StrictHostKeyChecking=yes", "-o", `UserKnownHostsFile="` + s.KnownHosts + `"`,
		"-o", "ConnectTimeout=" + strconv.Itoa(int(math.Ceil(s.ConnectTimeout.Seconds()))), "-o", "LogLevel=ERROR",
		"-o", "ServerAliveInterval=" + strconv.Itoa(serverAliveInterval), "-o", "ServerAliveCountMax=" + strconv.Itoa(serverAliveCountMax),
		"-p", strconv.Itoa(s.Port), "-l", t.User}
	c := proc.Command{Name: "ssh", Env: append([]string(nil), s.Env...)}
	if s.Auth == "key" {
		args = append(args, "-i", s.KeyFile, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes")
	} else {
		args = append(args, "-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no", "-o", "NumberOfPasswordPrompts=1")
		args = append([]string{"-e", "ssh"}, args...)
		c.Name = "sshpass"
		c.Env = append(c.Env, "SSHPASS="+t.Password)
	}
	c.Args = append(args, "--", t.Host, remote)
	if stream != nil {
		c.Stdout, c.Stderr = stream, stream
	}
	res, err := s.Proc.Run(ctx, c)
	if err != nil {
		return string(res.Stdout), s.explain(t, err, res)
	}
	return string(res.Stdout), nil
}

// explain turns an ssh failure into an error naming the host, never the
// password.
func (s SSH) explain(t Target, err error, res proc.Result) error {
	var coder interface{ ExitCode() int }
	code := -1
	if errors.As(err, &coder) {
		code = coder.ExitCode()
	}
	where := t.User + "@" + t.Host
	if errors.Is(err, exec.ErrNotFound) {
		tool := "ssh"
		if s.Auth == "password" {
			tool = "sshpass (auth password) or ssh"
		}
		return fmt.Errorf("%s: %s is not installed: %w", where, tool, err)
	}
	// 5 and 6 are sshpass's own exit codes (wrong password, unknown host
	// key); 255 is ssh's for a connection or authentication failure.
	switch {
	case s.Auth == "password" && code == 6:
		return fmt.Errorf("%s: the host key is not in the known_hosts file %s (host keys are checked strictly): pin it after checking its fingerprint (ssh-keyscan -t ed25519 %s)", where, s.KnownHosts, t.Host)
	case s.Auth == "password" && code == 5:
		return fmt.Errorf("%s: the SSH password was refused (DOCKER_SSH_PASS)", where)
	case code == 255:
		return fmt.Errorf("%s: ssh could not connect or authenticate (check %s and the credentials): %s", where, s.KnownHosts, tail(res))
	}
	if detail := tail(res); detail != "" {
		return fmt.Errorf("%s: %v: %s", where, err, detail)
	}
	return fmt.Errorf("%s: %w", where, err)
}

func tail(res proc.Result) string {
	for _, out := range [][]byte{res.Stderr, res.Stdout} {
		if s := strings.TrimSpace(string(out)); s != "" {
			return lastLine(s)
		}
	}
	return ""
}

// shQuote quotes s for the remote POSIX shell.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

var (
	userRe     = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]*$`)
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)
)

// checkDestination accepts a user of letters, digits, ., _ and - (not
// first) and a host that is a hostname or an IP address: nothing ssh could
// read as an option, a second destination or a shell word.
func checkDestination(user, host string) error {
	if !userRe.MatchString(user) {
		return fmt.Errorf("DOCKER_SSH_USER %q is not a valid user (letters, digits, ., _ and -, not starting with -)", user)
	}
	if net.ParseIP(host) == nil && !hostnameRe.MatchString(host) {
		return fmt.Errorf("DOCKER_SSH_HOST %q is not a hostname or an IP address", host)
	}
	return nil
}

// checkFile accepts an empty path or one without a leading -, quotes or
// control characters (it goes in a quoted -o value or after -i).
func checkFile(what, path string) error {
	if path == "" {
		return nil
	}
	if strings.HasPrefix(path, "-") || strings.ContainsAny(path, "\"") || strings.ContainsFunc(path, unicode.IsControl) {
		return fmt.Errorf("%s %q must be a file path without a leading -, quotes or control characters", what, path)
	}
	return nil
}
