package pb

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// sshPasswordFixture starts an isolated real SSH server accepting only a test username/password.
func sshPasswordFixture(t *testing.T) (string, string, *atomic.Int32) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		calls.Add(1)
		if meta.User() == "tester" && string(password) == "test-password" {
			return nil, nil
		}
		return nil, errors.New("bad credentials")
	}}
	config.AddHostKey(signer)
	// A second offered key must not override the pinned Ed25519 identity.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, err := ssh.NewSignerFromKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(rsaSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer raw.Close()
				_ = raw.SetDeadline(time.Now().Add(3 * time.Second))
				conn, channels, requests, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "session" {
						_ = channel.Reject(ssh.UnknownChannelType, "test server")
						continue
					}
					ch, reqs, err := channel.Accept()
					if err != nil {
						return
					}
					go func() {
						defer ch.Close()
						for request := range reqs {
							switch request.Type {
							case "pty-req", "window-change":
								_ = request.Reply(true, nil)
							case "shell":
								_ = request.Reply(true, nil)
								_, _ = io.Copy(ch, ch)
								return
							default:
								_ = request.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); workers.Wait() })
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{listener.Addr().String()}, signer.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return listener.Addr().String(), path, calls
}

// TestPasswordSSH authenticates over the real SSH protocol without a key or agent.
func TestPasswordSSH(t *testing.T) {
	target, hosts, calls := sshPasswordFixture(t)
	client, err := passwordSSH(target, "tester", "test-password", hosts)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if calls.Load() != 1 {
		t.Fatal("password callback was not used")
	}
	if c, err := passwordSSH(target, "tester", "wrong", hosts); err == nil {
		c.Close()
		t.Fatal("wrong password accepted")
	}
}

// TestPasswordSSHRejectsUnknownHost prevents password transmission when the host key is not pinned.
func TestPasswordSSHRejectsUnknownHost(t *testing.T) {
	target, _, calls := sshPasswordFixture(t)
	file := filepath.Join(t.TempDir(), "empty_hosts")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := passwordSSH(target, "tester", "test-password", file); err == nil {
		c.Close()
		t.Fatal("unknown host accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("password sent to an untrusted host")
	}
}

// TestTerminalTargetAndSizes restricts destinations and rejects invalid PTY dimensions.
func TestTerminalTargetAndSizes(t *testing.T) {
	t.Setenv("UPSNAP_TERMINAL_HOSTS", "192.168.50.148,192.168.50.150,192.168.50.152")
	for _, ip := range []string{"192.168.50.148", "192.168.50.150", "192.168.50.152"} {
		target, err := terminalTarget(ip)
		if err != nil || target != ip+":22" {
			t.Fatalf("bad target %s: %v", target, err)
		}
	}
	for _, ip := range []string{"127.0.0.1", "example.com", "192.168.50.148:2222", "-oProxyCommand=id"} {
		if _, err := terminalTarget(ip); err == nil {
			t.Fatal("unconfigured target accepted")
		}
	}
	if !terminalSize(80, 24) || terminalSize(0, 24) || terminalSize(80, 9999) {
		t.Fatal("bad resize validation")
	}
}

// TestTerminalOrigin rejects cross-origin or missing-origin connections before upgrading.
func TestTerminalOrigin(t *testing.T) {
	t.Setenv("UPSNAP_WEBAUTHN_ORIGIN", "https://monitor.example")
	t.Setenv("UPSNAP_TERMINAL_HOSTS", "192.168.50.148")
	for _, origin := range []string{"", "https://other.example"} {
		request := httptest.NewRequest("GET", "https://monitor.example/api/upsnap/terminal/connect", nil)
		request.Header.Set("Origin", origin)
		event := &core.RequestEvent{Event: router.Event{Request: request, Response: httptest.NewRecorder()}}
		if err := HandlerTerminal(event); err == nil {
			t.Fatal("untrusted origin accepted")
		}
	}
}

// TestTerminalLimits bounds concurrent shells and repeated login attempts for each monitor account.
func TestTerminalLimits(t *testing.T) {
	const user = "terminal-limit-test"
	t.Cleanup(func() {
		terminalSessions.Lock()
		defer terminalSessions.Unlock()
		delete(terminalSessions.active, user)
		delete(terminalSessions.attempts, user)
	})
	if !reserveTerminal(user) || !reserveTerminal(user) || reserveTerminal(user) {
		t.Fatal("session limit not enforced")
	}
	releaseTerminal(user)
	releaseTerminal(user)
	for i := 0; i < 4; i++ {
		if !reserveTerminal(user) {
			t.Fatal("slot was not released")
		}
		releaseTerminal(user)
	}
	if reserveTerminal(user) {
		releaseTerminal(user)
		t.Fatal("login attempt limit not enforced")
	}
}

// TestPasswordSSHPTY checks shell input/output and resizing over an authenticated SSH PTY.
func TestPasswordSSHPTY(t *testing.T) {
	target, hosts, _ := sshPasswordFixture(t)
	client, err := passwordSSH(target, "tester", "test-password", hosts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	input, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := session.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := session.WindowChange(40, 120); err != nil {
		t.Fatal(err)
	}
	const text = "terminal-echo-check\n"
	if _, err := io.WriteString(input, text); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len(text))
	if _, err := io.ReadFull(output, data); err != nil {
		t.Fatal(err)
	}
	if string(data) != text {
		t.Fatalf("unexpected PTY output %q", data)
	}
}
