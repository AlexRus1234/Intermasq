// Mock plugin for Intermasq's plugin contract (tests/fixtures/plugins/hello).
//
// Honours the existing runtime contract defined in main.go::loadPlugins():
// the host process exports PLUGIN_SOCKET=<unix socket path> and then
// reverse-proxies requests onto that socket. A real plugin therefore just
// has to bind the socket it is given and answer requests — no TCP, no
// network exposure. This binary does exactly that and nothing more.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sockPath := os.Getenv("PLUGIN_SOCKET")
	if sockPath == "" {
		fmt.Fprintln(os.Stderr, "[hello] PLUGIN_SOCKET not set")
		os.Exit(1)
	}

	// Stale socket from a previous run would block Listen.
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[hello] listen %s: %v\n", sockPath, err)
		os.Exit(1)
	}
	defer listener.Close()

	// 0660 matches SocketsDir's 0770 mode: owner+group read/write. Under
	// CI both processes run as root; in a rootless deployment the systemd
	// RuntimeDirectory owns the folder for the service user.
	os.Chmod(sockPath, 0660)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"plugin":"hello","path":%q,"method":%q}`, r.URL.Path, r.Method)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","plugin":"hello"}`)
	})

	// Die cleanly when intermasq tears the job down so we don't leak the
	// socket file in non-ephemeral environments.
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		<-sigc
		listener.Close()
		os.Exit(0)
	}()

	fmt.Printf("[hello] listening on %s\n", sockPath)
	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintf(os.Stderr, "[hello] serve: %v\n", err)
		os.Exit(1)
	}
}
