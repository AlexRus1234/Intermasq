// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Mock plugin for Intermasq's plugin contract (tests/fixtures/plugins/hello).
//
// Honours the existing runtime contract defined in internal/plugins.Load():
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
