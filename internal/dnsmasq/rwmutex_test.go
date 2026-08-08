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
