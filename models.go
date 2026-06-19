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
