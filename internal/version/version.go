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

// Package version содержит единственный источник истины для версии сборки.
// Default — "1.0.0-pre"; перекрывается через -ldflags при релизной сборке.
package version

// Version приложения. Перекрывается через:
//
//	-ldflags "-X intermask/internal/version.Version=<tag>"
var Version = "1.0.0-pre"
