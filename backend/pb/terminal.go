package pb

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/websocket"
)

type terminalFrame struct {
	Type     string `json:"type"`
	Token    string `json:"token,omitempty"`
	Device   string `json:"device,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Data     string `json:"data,omitempty"`
	Cols     int    `json:"cols,omitempty"`
	Rows     int    `json:"rows,omitempty"`
}

var terminalSockets = make(chan struct{}, 32)

type terminalStream struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// Write serializes SSH output as binary WebSocket frames, preserving terminal bytes.
func (s *terminalStream) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := websocket.Message.Send(s.conn, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// event sends a control message separately from raw terminal output.
func (s *terminalStream) event(kind, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = websocket.JSON.Send(s.conn, map[string]string{"type": kind, "message": message})
}

var terminalSessions = struct {
	sync.Mutex
	active   map[string]int
	attempts map[string][]time.Time
}{active: map[string]int{}, attempts: map[string][]time.Time{}}

// reserveTerminal bounds concurrent sessions and SSH login attempts per authenticated account.
func reserveTerminal(user string) bool {
	terminalSessions.Lock()
	defer terminalSessions.Unlock()
	now := time.Now()
	recent := []time.Time{}
	for _, t := range terminalSessions.attempts[user] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	total := 0
	for _, n := range terminalSessions.active {
		total += n
	}
	if len(recent) >= 6 || terminalSessions.active[user] >= 2 || total >= 16 {
		return false
	}
	terminalSessions.attempts[user] = append(recent, now)
	terminalSessions.active[user]++
	return true
}

// releaseTerminal returns a session slot after every successful reservation.
func releaseTerminal(user string) {
	terminalSessions.Lock()
	defer terminalSessions.Unlock()
	terminalSessions.active[user]--
	if terminalSessions.active[user] == 0 {
		delete(terminalSessions.active, user)
	}
}

// terminalTarget resolves only configured literal host IPs; clients cannot choose ports or commands.
func terminalTarget(ip string) (string, error) {
	if net.ParseIP(ip) == nil {
		return "", errors.New("invalid terminal host")
	}
	for _, allowed := range strings.Split(os.Getenv("UPSNAP_TERMINAL_HOSTS"), ",") {
		if strings.TrimSpace(allowed) == ip {
			return net.JoinHostPort(ip, "22"), nil
		}
	}
	return "", errors.New("terminal is not configured for this device")
}

// terminalSize rejects malformed resize frames before forwarding a PTY window change.
func terminalSize(cols, rows int) bool { return cols >= 2 && cols <= 500 && rows >= 2 && rows <= 250 }

// terminalDevice resolves configured router/computer IDs and readiness for any signed-in account; no power grants are required.
func terminalDevice(app core.App, id string) (*core.Record, string, error) {
	if d, target, err := routerTerminalRecord(app, id); err != nil || d != nil {
		return d, target, err
	}
	d, err := app.FindRecordById("devices", id)
	if err != nil {
		return nil, "", errors.New("device unavailable")
	}
	target, err := terminalTarget(d.GetString("ip"))
	if err != nil {
		return nil, "", err
	}
	if d.GetString("status") != "online" {
		return nil, "", errors.New("device SSH is not ready")
	}
	return d, target, nil
}

// HandlerTerminalDevice returns metadata and the configured SSH username for signed-in users.
// Returns JSON or an API error; no SSH credentials or power commands are sent.
func HandlerTerminalDevice(e *core.RequestEvent) error {
	d, target, err := terminalDevice(e.App, e.Request.PathValue("id"))
	if err != nil {
		return apis.NewApiError(http.StatusConflict, err.Error(), nil)
	}
	e.Response.Header().Set("Cache-Control", "no-store")
	username := "balint"
	if router := routerByTarget(target); router != nil {
		username = router.Username
	}
	return e.JSON(http.StatusOK, map[string]string{"id": d.Id, "name": d.GetString("name"), "ip": d.GetString("ip"), "username": username})
}

// passwordSSH connects with the supplied password and pinned host key; router-only legacy algorithms are explicitly allowlisted.
// target/user/password are connection inputs; hosts is the pinned known-hosts file. Returns an owned client or an error; no keys/agent are used.
func passwordSSH(target, user, password, hosts string) (*ssh.Client, error) {
	callback, err := knownhosts.New(hosts)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{User: user, Auth: []ssh.AuthMethod{ssh.Password(password)}, HostKeyCallback: callback, HostKeyAlgorithms: []string{ssh.KeyAlgoED25519}, Timeout: 8 * time.Second}
	if router := routerByTarget(target); router != nil {
		config.HostKeyAlgorithms = router.Algorithms
		config.Timeout = 15 * time.Second
		config.Config.KeyExchanges = append(ssh.SupportedAlgorithms().KeyExchanges, "diffie-hellman-group14-sha1")
	}
	connection, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Now().Add(config.Timeout))
	c, channels, requests, err := ssh.NewClientConn(connection, target, config)
	if err != nil {
		connection.Close()
		return nil, err
	}
	_ = connection.SetDeadline(time.Time{})
	return ssh.NewClient(c, channels, requests), nil
}

// HandlerTerminal upgrades same-origin sockets; the first bounded frame must authenticate before SSH starts.
func HandlerTerminal(e *core.RequestEvent) error {
	origin := os.Getenv("UPSNAP_WEBAUTHN_ORIGIN")
	if origin == "" || os.Getenv("UPSNAP_TERMINAL_HOSTS") == "" {
		return apis.NewApiError(http.StatusServiceUnavailable, "Terminal is not configured.", nil)
	}
	if e.Request.Header.Get("Origin") != origin {
		return apis.NewApiError(http.StatusForbidden, "Invalid terminal origin.", nil)
	}
	select {
	case terminalSockets <- struct{}{}:
		defer func() { <-terminalSockets }()
	default:
		return apis.NewApiError(http.StatusTooManyRequests, "Terminal capacity reached.", nil)
	}
	server := websocket.Server{Handshake: func(c *websocket.Config, r *http.Request) error {
		if r.Header.Get("Origin") != origin {
			return errors.New("invalid origin")
		}
		return nil
	}, Handler: func(ws *websocket.Conn) { serveTerminal(e.App, ws) }}
	server.ServeHTTP(e.Response, e.Request)
	return nil
}

// serveTerminal owns one independent SSH PTY, validates monitor auth, and closes all resources on exit.
func serveTerminal(app core.App, ws *websocket.Conn) {
	defer ws.Close()
	stream := &terminalStream{conn: ws}
	ws.MaxPayloadBytes = 8192
	_ = ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	var first terminalFrame
	if err := websocket.JSON.Receive(ws, &first); err != nil {
		return
	}
	auth, err := app.FindAuthRecordByToken(first.Token, core.TokenTypeAuth)
	if err != nil || first.Type != "auth" {
		stream.event("error", "Sign in to Server Monitor first.")
		return
	}
	if auth.Collection().Name != "users" && auth.Collection().Name != "_superusers" {
		stream.event("error", "Invalid monitor account.")
		return
	}
	_, target, err := terminalDevice(app, first.Device)
	if err != nil {
		stream.event("error", err.Error())
		return
	}
	if len(first.Username) == 0 || len(first.Username) > 128 || strings.ContainsAny(first.Username, "\x00\r\n") || len(first.Password) == 0 || len(first.Password) > 4096 || !terminalSize(first.Cols, first.Rows) {
		stream.event("error", "Enter an SSH username and password.")
		return
	}
	user := auth.Collection().Id + "/" + auth.Id
	if !reserveTerminal(user) {
		stream.event("error", "Too many terminal sessions or login attempts. Try again later.")
		return
	}
	defer releaseTerminal(user)
	hosts := os.Getenv("UPSNAP_TERMINAL_KNOWN_HOSTS")
	if hosts == "" {
		hosts = "/app/ssh/known_hosts"
	}
	client, err := passwordSSH(target, first.Username, first.Password, hosts)
	first.Password = ""
	if err != nil {
		stream.event("error", "SSH sign-in failed. Check the username/password, host availability and pinned host key.")
		return
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		stream.event("error", "Cannot open SSH session.")
		return
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return
	}
	session.Stdout = stream
	session.Stderr = stream
	if err = session.RequestPty("xterm-256color", first.Rows, first.Cols, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}); err != nil {
		stream.event("error", "SSH host refused a terminal.")
		return
	}
	if err = session.Shell(); err != nil {
		stream.event("error", "SSH host refused a login shell.")
		return
	}
	app.Logger().Info("Terminal connected", "user", auth.Id, "device", first.Device)
	defer app.Logger().Info("Terminal closed", "user", auth.Id, "device", first.Device)
	stream.event("ready", "Connected")
	done := make(chan struct{})
	defer close(done)
	go func() {
		err := session.Wait()
		message := "Session ended"
		if err != nil {
			message = "SSH session closed"
		}
		stream.event("exit", message)
		ws.Close()
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		limit := time.NewTimer(8 * time.Hour)
		defer limit.Stop()
		for {
			select {
			case <-done:
				return
			case <-limit.C:
				ws.Close()
				client.Close()
				return
			case <-ticker.C:
				if _, err := app.FindAuthRecordByToken(first.Token, core.TokenTypeAuth); err != nil {
					ws.Close()
					client.Close()
					return
				}
			}
		}
	}()
	ws.MaxPayloadBytes = 65536
	for {
		_ = ws.SetReadDeadline(time.Now().Add(15 * time.Minute))
		var frame terminalFrame
		if err := websocket.JSON.Receive(ws, &frame); err != nil {
			return
		}
		switch frame.Type {
		case "input":
			if len(frame.Data) > 32768 {
				return
			}
			if _, err = fmt.Fprint(stdin, frame.Data); err != nil {
				return
			}
		case "resize":
			if !terminalSize(frame.Cols, frame.Rows) {
				return
			}
			if err = session.WindowChange(frame.Rows, frame.Cols); err != nil {
				return
			}
		default:
			return
		}
	}
}
