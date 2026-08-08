package version

import "testing"

func TestVersion_Default(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}
