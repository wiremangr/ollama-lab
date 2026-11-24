package tools

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

type netSession struct {
	id         string
	kind       string
	conn       net.Conn
	remoteAddr string
	localAddr  string
	createdAt  time.Time
	lastUsedAt time.Time
	mu         sync.Mutex
}

type netSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*netSession
	nextID   int
}

var globalNetSessions = &netSessionManager{sessions: map[string]*netSession{}}

const (
	netSessionIdleTTL     = 5 * time.Minute
	netSessionMaxLifetime = 30 * time.Minute
	netSessionMaxCount    = 32
	netMaxReadBytes       = 64 * 1024
	netMaxSendBytes       = 64 * 1024
)

func (m *netSessionManager) cleanupLocked(now time.Time) {
	for id, s := range m.sessions {
		if now.Sub(s.lastUsedAt) > netSessionIdleTTL || now.Sub(s.createdAt) > netSessionMaxLifetime {
			_ = s.conn.Close()
			delete(m.sessions, id)
		}
	}
}

func (m *netSessionManager) open(kind string, conn net.Conn) (*netSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(time.Now())
	if len(m.sessions) >= netSessionMaxCount {
		return nil, fmt.Errorf("too many active network sessions (max %d)", netSessionMaxCount)
	}
	m.nextID++
	id := fmt.Sprintf("%s_%d_%d", kind, time.Now().UnixNano(), m.nextID)
	s := &netSession{
		id:         id,
		kind:       kind,
		conn:       conn,
		remoteAddr: conn.RemoteAddr().String(),
		localAddr:  conn.LocalAddr().String(),
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
	}
	m.sessions[id] = s
	return s, nil
}

func (m *netSessionManager) get(id string) (*netSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(time.Now())
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found or expired: %s", id)
	}
	s.lastUsedAt = time.Now()
	return s, nil
}

func (m *netSessionManager) close(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("session not found or expired: %s", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Close()
}

func dialTCP(ctx context.Context, host string, port int, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}

func dialTLS(ctx context.Context, host string, port int, serverName string, timeout time.Duration, insecure bool) (net.Conn, error) {
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		Config: &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: insecure, //nolint:gosec // explicitly requested by tool caller with confirmation
		},
	}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}

func encodeNetData(data []byte, encoding string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "utf8", "utf-8", "text":
		return string(data), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(data), nil
	default:
		return "", fmt.Errorf("unsupported encoding: %s (use utf8 or base64)", encoding)
	}
}

func decodeNetData(data string, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "utf8", "utf-8", "text":
		return []byte(data), nil
	case "base64":
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 data: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s (use utf8 or base64)", encoding)
	}
}

func (s *netSession) send(ctx context.Context, payload []byte, timeout time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(payload) > netMaxSendBytes {
		return 0, fmt.Errorf("data exceeds max send size (%d bytes)", netMaxSendBytes)
	}
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		return 0, err
	}
	n, err := s.conn.Write(payload)
	s.lastUsedAt = time.Now()
	return n, err
}

func (s *netSession) recv(ctx context.Context, maxBytes int, timeout time.Duration, readUntil string) ([]byte, bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxBytes <= 0 {
		maxBytes = 4096
	}
	if maxBytes > netMaxReadBytes {
		maxBytes = netMaxReadBytes
	}
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := s.conn.SetReadDeadline(deadline); err != nil {
		return nil, false, false, err
	}

	var buf []byte
	eof := false
	timedOut := false
	reader := bufio.NewReader(s.conn)

	if readUntil != "" {
		delim := []byte(readUntil)
		if len(delim) == 0 {
			return nil, false, false, fmt.Errorf("read_until cannot be empty")
		}
		for len(buf) < maxBytes {
			b, err := reader.ReadByte()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					timedOut = true
					break
				}
				if err == io.EOF {
					eof = true
					break
				}
				s.lastUsedAt = time.Now()
				return buf, false, false, err
			}
			buf = append(buf, b)
			if len(buf) >= len(delim) && strings.HasSuffix(string(buf), readUntil) {
				break
			}
		}
	} else {
		tmp := make([]byte, maxBytes)
		n, err := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				timedOut = true
			} else if err == io.EOF {
				eof = true
			} else if n == 0 {
				s.lastUsedAt = time.Now()
				return buf, false, false, err
			}
		}
	}

	s.lastUsedAt = time.Now()
	return buf, eof, timedOut, nil
}
