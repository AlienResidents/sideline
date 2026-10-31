package ipc

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
)

func echoHandler(req model.Request) model.Response {
	return model.Response{OK: true, State: &model.State{Version: model.StateVersion, Tasks: []model.Task{{ID: req.Op}}}}
}

func startServer(t *testing.T) (*Server, string) {
	t.Helper()
	// macOS caps a unix socket path near 104 bytes; t.TempDir() alone
	// overruns it in longer test names.
	path := filepath.Join(mustTempDir(t), "s.sock")
	srv, err := Listen(path, echoHandler)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })
	return srv, path
}

func TestSocketRoundTrip(t *testing.T) {
	_, path := startServer(t)

	resp, err := Send(path, model.Request{Op: model.OpPing})
	if err != nil || !resp.OK {
		t.Fatalf("ping: err=%v resp=%+v", err, resp)
	}
	if resp.State == nil || len(resp.State.Tasks) != 1 || resp.State.Tasks[0].ID != model.OpPing {
		t.Fatalf("echo state lost: %+v", resp.State)
	}

	// A second request on a fresh connection reuses the same socket.
	if _, err := Send(path, model.Request{Op: model.OpList}); err != nil {
		t.Fatalf("second send: %v", err)
	}
}

func TestBadJSONLine(t *testing.T) {
	_, path := startServer(t)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("not json\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	sc := scanLine(t, conn)
	var resp model.Response
	if err := json.Unmarshal(sc, &resp); err != nil {
		t.Fatalf("bad-line response not JSON: %v", err)
	}
	if resp.OK {
		t.Fatalf("bad line accepted: %+v", resp)
	}
	if resp.Error == "" {
		t.Fatalf("bad line response missing error text")
	}
}

func TestOversizedLineRejected(t *testing.T) {
	_, path := startServer(t)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// One byte over the cap: the server must not buffer it.
	big := "\"" + strings.Repeat("a", MaxLine) + "\"\n"
	// The server closes the socket once the line overruns the cap, so
	// the write may fail partway: that is the rejection under test.
	_, _ = conn.Write([]byte(big))
	sc := scanLine(t, conn)
	if len(sc) > MaxLine {
		t.Fatalf("server echoed %d bytes, over the cap", len(sc))
	}
}

func TestConcurrentClients(t *testing.T) {
	_, path := startServer(t)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := Send(path, model.Request{Op: model.OpPing, ID: strings.Repeat("x", i)}); err != nil {
				t.Errorf("client %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestCloseRemovesSocketAndIsIdempotent(t *testing.T) {
	path := filepath.Join(mustTempDir(t), "s.sock")
	srv, err := Listen(path, echoHandler)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = srv.Serve() }()
	if _, err := Send(path, model.Request{Op: model.OpPing}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket file survived Close: err=%v", err)
	}
	if _, err := Send(path, model.Request{Op: model.OpPing}); err == nil {
		t.Fatalf("send to closed socket succeeded")
	}
}

// scanLine reads one newline-terminated line from conn, failing the test on
// error. The oversized-line test relies on the server closing the
// connection, so an EOF with no bytes is a failure handled by its caller.
func scanLine(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	buf := make([]byte, 1)
	var out []byte
	for {
		n, err := conn.Read(buf)
		out = append(out, buf[:n]...)
		if n > 0 && out[len(out)-1] == '\n' {
			return out[:len(out)-1]
		}
		if err != nil {
			return out
		}
	}
}

// mustTempDir makes a short-lived temp dir removed at test end.
func mustTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sl")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
