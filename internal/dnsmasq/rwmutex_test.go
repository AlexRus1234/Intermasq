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

package dnsmasq

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReadScansAreSafeForConcurrentCallers(t *testing.T) {
	dir := newTestDir(t)
	file := filepath.Join(dir, "hosts.conf")
	if err := os.WriteFile(file, []byte("dhcp-host=aa:bb:cc:dd:ee:01,10.0.0.1,host1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := ReadAllHosts(); len(got) != 1 {
				t.Errorf("ReadAllHosts returned %d entries, want 1", len(got))
			}
			if got := FindHostsByIP("10.0.0.1", ""); len(got) != 1 {
				t.Errorf("FindHostsByIP returned %d entries, want 1", len(got))
			}
		}()
	}
	wg.Wait()
}
