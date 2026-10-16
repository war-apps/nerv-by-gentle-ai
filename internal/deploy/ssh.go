package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

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

// Run runs the remote command on t's host and returns its standard output;
// with stream set, its output goes there live instead.
func (s SSH) Run(ctx context.Context, t Target, remote string, stream io.Writer) (string, error) {
	args := []string{"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + s.KnownHosts,
		"-o", "ConnectTimeout=" + strconv.Itoa(int(math.Ceil(s.ConnectTimeout.Seconds()))), "-o", "LogLevel=ERROR",
		"-p", strconv.Itoa(s.Port)}
	c := proc.Command{Name: "ssh", Env: append([]string(nil), s.Env...)}
	if s.Auth == "key" {
		args = append(args, "-i", s.KeyFile, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes")
	} else {
		args = append(args, "-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no", "-o", "NumberOfPasswordPrompts=1")
		args = append([]string{"-e", "ssh"}, args...)
		c.Name = "sshpass"
		c.Env = append(c.Env, "SSHPASS="+t.Password)
	}
	c.Args = append(args, t.User+"@"+t.Host, remote)
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
