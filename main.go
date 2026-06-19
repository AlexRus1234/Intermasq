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

package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"
	_ "intermask/docs"
)

//go:embed frontend/dist/*
var staticFiles embed.FS

var (
	Port       = flag.String("port", "8080", "Port to listen on")
	DBPath     = flag.String("db", "/etc/intermasq/users.json", "Path to user database")
	ConfigDir  = flag.String("conf-dir", "/etc/dnsmasq.d", "Directory with dnsmasq configs")
	LeasesPath = flag.String("leases", "/var/lib/misc/dnsmasq.leases", "Path to dnsmasq.leases")
	PluginsDir = "/etc/intermasq/plugins"
	SocketsDir = "/run/intermasq/sockets" // Папка для сокетов (в оперативной памяти)
	SecretKey  = []byte(os.Getenv("INTERMASQ_SECRET"))
)

var (
	macRegex      = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9-.]+$`)
	mu            sync.Mutex
	loadedPlugins []PluginManifest
)

type PluginManifest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bin  string `json:"bin"`
	// Port убрали, он больше не нужен
}

func init() {
	if len(SecretKey) == 0 { SecretKey = []byte("default-secret") }
}

func loadPlugins(r *gin.Engine) {
	// Создаем папку для сокетов (tmpfs)
	os.MkdirAll(SocketsDir, 0770)

	entries, err := os.ReadDir(PluginsDir)
	if err != nil { return }

	for _, entry := range entries {
		if !entry.IsDir() { continue }
		
		path := filepath.Join(PluginsDir, entry.Name())
		manifestPath := filepath.Join(path, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil { continue }

		var p PluginManifest
		if err := json.Unmarshal(data, &p); err != nil { continue }

		binPath := filepath.Join(path, p.Bin)
		sockPath := filepath.Join(SocketsDir, p.ID+".sock")

		if _, err := os.Stat(binPath); err == nil {
			cmd := exec.Command(binPath)
			cmd.Dir = path
			// Передаем путь к сокету через ENV
			cmd.Env = append(os.Environ(), 
				fmt.Sprintf("INTERMASQ_KEY=%s", os.Getenv("INTERMASQ_SECRET")),
				fmt.Sprintf("PLUGIN_SOCKET=%s", sockPath),
			)
			
			if err := cmd.Start(); err != nil {
				fmt.Printf("[PLUGINS] Ошибка запуска %s: %v\n", p.Name, err)
				continue
			}
			fmt.Printf("[PLUGINS] Запущен %s на сокете %s\n", p.Name, sockPath)
		}

		// Настраиваем Прокси через Unix Socket
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = "dummy" // Для unix сокетов хост не важен, но поле должно быть
				// Обрезаем префикс пути
				// (gin делает это сам через r.Any, но на всякий случай можно тут)
			},
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", sockPath)
				},
			},
		}

		r.Any("/plugins/"+p.ID+"/*any", func(c *gin.Context) {
			c.Request.URL.Path = c.Param("any") // Отрезаем /plugins/{id}
			proxy.ServeHTTP(c.Writer, c.Request)
		})

		loadedPlugins = append(loadedPlugins, p)
	}
}

func main() {
	flag.Parse()
	loadUsers()
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	loadPlugins(r)

	api := r.Group("/api")
	{
		api.GET("/status", statusHandler)
		api.POST("/setup", setupHandler)
		api.POST("/login", loginHandler)
		
		auth := api.Group("/")
		auth.Use(authMiddleware)
		{
			auth.GET("/plugins", func(c *gin.Context) { c.JSON(200, loadedPlugins) })
			auth.POST("/restart-self", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "restarting"})
				go func() { exec.Command("/usr/bin/systemctl", "restart", "intermasq").Run() }()
			})
			auth.GET("/hosts", getHostsHandler)
			auth.GET("/leases", getLeasesHandler)
			auth.GET("/arp", getArpHandler)
			auth.POST("/hosts", addHostHandler)
			auth.POST("/hosts/bulk", bulkAddHostsHandler)
			auth.DELETE("/hosts/:mac", deleteHostHandler)
			auth.POST("/rollback", rollbackHandler)
			auth.POST("/reload", reloadHandler)
			auth.GET("/backup", backupHandler)
		}
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	staticFS, _ := fs.Sub(staticFiles, "frontend/dist")
	r.NoRoute(gin.WrapH(http.FileServer(http.FS(staticFS))))

	fmt.Printf("Intermasq v3.0 (Socket Mode) Started on :%s\n", *Port)
	r.Run(":" + *Port)
}
