// Package ipc speaks the sideline line protocol over a unix socket: one
// JSON Request per line in, one JSON Response per line back out.
package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/AlienResidents/sideline/internal/model"
)

// MaxLine caps a single protocol line at 1 MiB. A larger line is rejected
// by closing the connection rather than buffering unbounded input.
const MaxLine = 1 << 20

// Handler answers one request with one response.
type Handler func(model.Request) model.Response

// Server is a listening unix socket serving the line protocol.
type Server struct {
	ln      net.Listener
	path    string
	handler Handler

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
}

// Listen starts a server on path, replacing any stale socket file there.
// The socket is created 0600: state rows are the caller's own.
func Listen(path string, handler Handler) (*Server, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket %s: %w", path, err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}
	return &Server{
		ln:      ln,
		path:    path,
		handler: handler,
		conns:   map[net.Conn]struct{}{},
	}, nil
}

// Serve accepts connections until Close. It returns nil after Close.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveConn(conn)
		}()
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLine)
	for sc.Scan() {
		var req model.Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			if err := writeLine(conn, model.Response{OK: false, Error: fmt.Sprintf("bad request: %v", err)}); err != nil {
				return
			}
			continue
		}
		if err := writeLine(conn, s.handler(req)); err != nil {
			return
		}
	}
	// A too-long line surfaces here as sc.Err(); the connection closes,
	// which the client sees as a truncated read.
}

// Close stops the listener, closes live connections and removes the
// socket file. It is safe to call twice.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conns := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	err := s.ln.Close()
	for _, c := range conns {
		c.Close()
	}
	s.wg.Wait()
	if rerr := os.Remove(s.path); rerr != nil && !os.IsNotExist(rerr) {
		// Not fatal: a stale socket just fails the next Listen, which
		// removes it first.
		_ = rerr
	}
	return err
}

// Send connects to the socket at path, sends req and reads one response.
func Send(path string, req model.Request) (model.Response, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return model.Response{}, fmt.Errorf("dial %s: %w", path, err)
	}
	defer conn.Close()
	if err := writeLine(conn, req); err != nil {
		return model.Response{}, fmt.Errorf("write request: %w", err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLine)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			if err == bufio.ErrTooLong {
				return model.Response{}, fmt.Errorf("response line over %d bytes", MaxLine)
			}
			return model.Response{}, fmt.Errorf("read response: %w", err)
		}
		return model.Response{}, fmt.Errorf("server closed the connection: %w", io.ErrUnexpectedEOF)
	}
	var resp model.Response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return model.Response{}, fmt.Errorf("parse response: %w", err)
	}
	return resp, nil
}

// writeLine encodes v as one JSON line.
func writeLine(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
