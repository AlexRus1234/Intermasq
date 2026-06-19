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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func statusHandler(c *gin.Context) {
	isActive := checkDnsmasqStatus()
	mu.Lock()
	defer mu.Unlock()
	c.JSON(200, gin.H{
		"setup_required": len(users) == 0,
		"version":        "2.0",
		"dnsmasq_active": isActive,
	})
}

func setupHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()
	if len(users) > 0 { c.JSON(403, gin.H{"error": "Уже настроено"}); return }
	var req AuthReq
	if err := c.BindJSON(&req); err != nil { return }
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	users[req.Username] = string(hash)
	saveUsers()
	c.JSON(200, gin.H{"token": makeToken(req.Username)})
}

func loginHandler(c *gin.Context) {
	var req AuthReq
	if err := c.BindJSON(&req); err != nil { return }
	mu.Lock()
	hash, ok := users[req.Username]
	mu.Unlock()
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(401, gin.H{"error": "Неверный логин или пароль"}); return
	}
	c.JSON(200, gin.H{"token": makeToken(req.Username)})
}

// НОВОЕ: Возвращает список активных MAC из ARP
func getArpHandler(c *gin.Context) {
	c.JSON(200, getArpTable())
}

func getHostsHandler(c *gin.Context) {
	hosts := []HostEntry{}
	files, err := os.ReadDir(*ConfigDir)
	if err != nil { c.JSON(500, gin.H{"error": "Ошибка чтения директории"}); return }

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".conf" { continue }
		fullPath := filepath.Join(*ConfigDir, f.Name())
		
		// Проверяем наличие .bak файла для UI
		hasBak := false
		if _, err := os.Stat(fullPath + ".bak"); err == nil { hasBak = true }

		content, _ := os.ReadFile(fullPath)
		lines := strings.Split(string(content), "\n")
		
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "dhcp-host=") {
				parts := strings.Split(strings.TrimPrefix(line, "dhcp-host="), ",")
				entry := HostEntry{File: fullPath}
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if macRegex.MatchString(p) { entry.Mac = p } else 
					if net.ParseIP(p) != nil { entry.Ip = p } else { entry.Hostname = p }
				}
				if entry.Mac != "" {
					// Костыль: добавляем флаг hasBak в поле File через разделитель |
					// (Чтобы не ломать структуру данных на фронте)
					if hasBak { entry.File = fullPath + "|has_bak" }
					hosts = append(hosts, entry)
				}
			}
		}
	}
	c.JSON(200, hosts)
}

func getLeasesHandler(c *gin.Context) {
	c.JSON(200, parseLeases())
}

func addHostHandler(c *gin.Context) {
	var req HostEntry
	if err := c.BindJSON(&req); err != nil { return }
	if !macRegex.MatchString(req.Mac) || net.ParseIP(req.Ip) == nil || !hostnameRegex.MatchString(req.Hostname) || !isSafePath(req.File) {
		c.JSON(400, gin.H{"error": "Неверные данные"}); return
	}

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(req.File) // БЭКАП ПЕРЕД ИЗМЕНЕНИЕМ

	content, err := os.ReadFile(req.File)
	if err != nil && !os.IsNotExist(err) { c.JSON(500, gin.H{"error": "Ошибка чтения файла"}); return }

	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "dhcp-host=") && strings.Contains(line, req.Mac) { continue }
		if strings.TrimSpace(line) != "" { newLines = append(newLines, line) }
	}
	newLines = append(newLines, fmt.Sprintf("dhcp-host=%s,%s,%s", req.Mac, req.Hostname, req.Ip))

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "Ошибка записи файла"}); return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// НОВОЕ: Массовый импорт
func bulkAddHostsHandler(c *gin.Context) {
	var req BulkHostReq
	if err := c.BindJSON(&req); err != nil { c.JSON(400, gin.H{"error": "Bad JSON"}); return }
	if !isSafePath(req.File) { c.JSON(403, gin.H{"error": "Access denied"}); return }

	mu.Lock()
	defer mu.Unlock()

	createLocalBackup(req.File) // БЭКАП ПЕРЕД ИЗМЕНЕНИЕМ

	content, err := os.ReadFile(req.File)
	if err != nil && !os.IsNotExist(err) { c.JSON(500, gin.H{"error": "Ошибка чтения"}); return }

	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	
	// Собираем все MAC из нового списка для фильтрации старых
	newMacs := make(map[string]bool)
	for _, h := range req.Hosts {
		if macRegex.MatchString(h.Mac) && net.ParseIP(h.Ip) != nil && hostnameRegex.MatchString(h.Hostname) {
			newMacs[strings.ToLower(h.Mac)] = true
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "dhcp-host=") {
			parts := strings.Split(line, ",")
			skip := false
			for _, p := range parts {
				if newMacs[strings.ToLower(strings.TrimSpace(p))] { skip = true; break }
			}
			if skip { continue }
		}
		if strings.TrimSpace(line) != "" { newLines = append(newLines, line) }
	}

	// Добавляем новые
	for _, h := range req.Hosts {
		if newMacs[strings.ToLower(h.Mac)] {
			newLines = append(newLines, fmt.Sprintf("dhcp-host=%s,%s,%s", h.Mac, h.Hostname, h.Ip))
		}
	}

	if err := os.WriteFile(req.File, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		c.JSON(500, gin.H{"error": "Ошибка записи"}); return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func deleteHostHandler(c *gin.Context) {
	mac := c.Param("mac")
	file := c.Query("file")
	if !macRegex.MatchString(mac) || !isSafePath(file) {
		c.JSON(400, gin.H{"error": "Bad request"})
		return
	}

	// === ИСПРАВЛЕНИЕ: ПРИВОДИМ MAC К НИЖНЕМУ РЕГИСТРУ ===
	macLower := strings.ToLower(mac)

	mu.Lock()
	defer mu.Unlock()
	
	createLocalBackup(file)

	content, err := os.ReadFile(file)
	if err != nil {
		c.JSON(500, gin.H{"error": "Ошибка чтения файла"})
		return
	}

	lines := strings.Split(string(content), "\n")
	newLines := []string{}
	found := false
	
	for _, line := range lines {
		cleanLine := strings.TrimSpace(line)
		// === ИСПРАВЛЕНИЕ: ПРИВОДИМ СТРОКУ К НИЖНЕМУ РЕГИСТРУ ДЛЯ ПОИСКА ===
		if strings.HasPrefix(cleanLine, "dhcp-host=") && strings.Contains(strings.ToLower(cleanLine), macLower) {
			found = true
			continue // Пропускаем (удаляем)
		}
		if cleanLine != "" { newLines = append(newLines, line) }
	}

	if found {
		os.WriteFile(file, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
		c.JSON(200, gin.H{"status": "deleted"})
	} else {
		c.JSON(404, gin.H{"error": "Хост не найден"})
	}
}

// НОВОЕ: Откат файла
func rollbackHandler(c *gin.Context) {
	var req struct { File string `json:"file"` }
	if err := c.BindJSON(&req); err != nil { return }

	mu.Lock()
	defer mu.Unlock()

	if err := rollbackFile(req.File); err != nil {
		c.JSON(500, gin.H{"error": "Ошибка отката: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "rollback_ok"})
}

func backupHandler(c *gin.Context) {
	zipBytes, fileName, err := createBackupZip()
	if err != nil { c.JSON(500, gin.H{"error": "Ошибка бэкапа"}); return }
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Data(200, "application/zip", zipBytes)
}

func reloadHandler(c *gin.Context) {
	out, err := reloadDnsmasq()
	if err != nil { c.JSON(400, gin.H{"error": "Ошибка перезапуска!\n" + string(out)}); return }
	c.JSON(200, gin.H{"status": "reloaded"})
}
