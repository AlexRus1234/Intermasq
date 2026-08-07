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

// Type aliases re-exporting the DTOs from internal/models so the existing
// package main call-sites keep compiling unchanged during the modular
// refactoring. These aliases are temporary and will be removed once all
// packages reference intermask/internal/models directly (stage 11).
package main

import (
	"github.com/gin-gonic/gin"
	"intermask/internal/audit"
	"intermask/internal/models"
	"intermask/internal/netstate"
	templatepkg "intermask/internal/templates"
)

type (
	HostEntry             = models.HostEntry
	BulkHostReq           = models.BulkHostReq
	LeaseEntry            = models.LeaseEntry
	AuthReq               = models.AuthReq
	Template              = models.Template
	ApplyTemplateReq      = models.ApplyTemplateReq
	BulkMoveReq           = models.BulkMoveReq
	BulkEditReq           = models.BulkEditReq
	IPTransformSpec       = models.IPTransformSpec
	HostnameTransformSpec = models.HostnameTransformSpec
	Directive             = models.Directive
	ConfigFile            = models.ConfigFile
	ConfigSnapshot        = models.ConfigSnapshot
	DhcpRange             = models.DhcpRange
	ConfigUpdateReq       = models.ConfigUpdateReq
	CreateConfigFileReq   = models.CreateConfigFileReq
	DnsAliasEntry         = models.DnsAliasEntry
	BulkAliasReq          = models.BulkAliasReq
	DeleteAliasReq        = models.DeleteAliasReq
	HistoryRestoreReq     = models.HistoryRestoreReq
	UserPasswordReq       = models.UserPasswordReq
	NewDeviceInfo         = models.NewDeviceInfo
	BulkLeaseToStaticReq  = models.BulkLeaseToStaticReq
	AuditEntry            = audit.AuditEntry
)

var (
	ArpPath       = netstate.ArpPath
	LeasesPath    = netstate.LeasesPath
	AuditLogPath  = audit.AuditLogPath
	TemplatesPath = templatepkg.TemplatesPath
)

func getArpTable() map[string]bool                   { return netstate.GetArpTable() }
func parseArpContent(content string) map[string]bool { return netstate.ParseArpContent(content) }
func parseLeases() []LeaseEntry                      { return netstate.ParseLeases() }
func getNewDevices() []NewDeviceInfo                 { return netstate.GetNewDevices() }
func writeAudit(entry AuditEntry)                    { audit.WriteAudit(entry) }
func auditHandler(c *gin.Context)                    { audit.Handler(c) }
func resetTemplates()                                { templatepkg.Reset() }
func setTemplate(id string, t Template)              { templatepkg.Set(id, t) }
func hasTemplate(id string) bool                     { _, ok := templatepkg.Get(id); return ok }
