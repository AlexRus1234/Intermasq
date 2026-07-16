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

type HostEntry struct {
	Mac      string `json:"mac"`
	Ip       string `json:"ip"`
	Hostname string `json:"hostname"`
	File     string `json:"file"`
}

// НОВОЕ: Для массового импорта
type BulkHostReq struct {
	Hosts []HostEntry `json:"hosts"`
	File  string      `json:"file"`
}

type LeaseEntry struct {
	Ip       string `json:"ip"`
	Mac      string `json:"mac"`
	Hostname string `json:"hostname"`
}

type AuthReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Template struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IPRange         string `json:"ip_range"`
	HostnamePattern string `json:"hostname_pattern"`
	TargetFile      string `json:"target_file"`
}

type ApplyTemplateReq struct {
	Mac        string `json:"mac"`
	TemplateID string `json:"template_id"`
}

type BulkMoveReq struct {
	Hosts  []HostEntry `json:"hosts"`
	Target string      `json:"target"`
}

type BulkEditReq struct {
	Hosts             []HostEntry        `json:"hosts"`
	IPTransform       IPTransformSpec    `json:"ip_transform"`
	HostnameTransform HostnameTransformSpec `json:"hostname_transform"`
}

type IPTransformSpec struct {
	OldPrefix string `json:"old_prefix"`
	NewPrefix string `json:"new_prefix"`
}

type HostnameTransformSpec struct {
	Suffix    string `json:"suffix"`
	StripOld  string `json:"strip_old"`
}

// Directive represents a single dnsmasq directive line (excluding dhcp-host).
type Directive struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Active bool   `json:"active"`
	File   string `json:"file,omitempty"`
	LineNo int    `json:"line_no,omitempty"`
}

// ConfigFile groups directives by source file.
type ConfigFile struct {
	Path       string      `json:"path"`
	Name       string      `json:"name"`
	Directives []Directive `json:"directives"`
	HasBak     bool        `json:"has_bak"`
}

// ConfigSnapshot is the response for GET /api/config.
type ConfigSnapshot struct {
	Files      []ConfigFile `json:"files"`
	DhcpRanges []DhcpRange  `json:"dhcp_ranges"`
}

// DhcpRange is a structured representation of a dhcp-range= directive.
type DhcpRange struct {
	Raw       string `json:"raw"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Mask      string `json:"mask"`
	LeaseTime string `json:"lease_time"`
	Tag       string `json:"tag"`
	CIDR      string `json:"cidr"`
	File      string `json:"file"`
	LineNo    int    `json:"line_no"`
}

// ConfigUpdateReq is the body of PUT /api/config.
type ConfigUpdateReq struct {
	File       string      `json:"file"`
	Directives []Directive `json:"directives"`
}

// CreateConfigFileReq is the body of POST /api/config/file.
type CreateConfigFileReq struct {
	Name string `json:"name"`
}

// DnsAliasEntry represents a dnsmasq address= or cname= directive.
// Type is "A" for address=/domain/IP, "CNAME" for cname=alias,target.
type DnsAliasEntry struct {
	Type   string `json:"type"`
	Domain string `json:"domain"`
	Target string `json:"target"`
	File   string `json:"file"`
}

// BulkAliasReq is the body of POST /api/aliases/bulk.
type BulkAliasReq struct {
	Aliases []DnsAliasEntry `json:"aliases"`
	File    string          `json:"file"`
}

// DeleteAliasReq is the body of POST /api/aliases/delete.
type DeleteAliasReq struct {
	Type   string `json:"type"`
	Domain string `json:"domain"`
	File   string `json:"file"`
}

// HistoryRestoreReq is the body of POST /api/history/restore.
type HistoryRestoreReq struct {
	File    string `json:"file"`
	Version string `json:"version"`
}
