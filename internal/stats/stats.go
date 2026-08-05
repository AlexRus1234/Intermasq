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

// Package stats holds the in-process operational counters exported via
// /metrics. The counters are split out from the metrics handler so that
// the packages that increment them (dnsmasq, history, backup, control/sse)
// do not import the metrics package (which would create an import cycle:
// metrics -> dnsmasq -> metrics). All fields are atomic so they can be
// updated from any handler goroutine without taking a mutex.
package stats

import (
	"sync/atomic"
	"time"
)

// counters holds in-process operational counters exported via /metrics.
// The type is unexported on purpose: callers reach the single process-wide
// instance through the exported var Counters below, and never need to name
// the type. All fields are atomic so they can be updated from any handler
// goroutine without taking a mutex.
type counters struct {
	Reloads      atomic.Int64 // successful dnsmasq reloads
	TestFailures atomic.Int64 // dnsmasq --test failures (prevented reloads)
	StartedAt    time.Time
}

// Counters is the single process-wide instance, initialised at startup.
var Counters = &counters{StartedAt: time.Now()}
