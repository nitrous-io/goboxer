package goboxer

import (
	"os"
	"path/filepath"
	"testing"
)

func setUp() (source, delta, target string) {
	source, _ = filepath.Abs("/tmp/test2")
	delta, _ = filepath.Abs("/tmp/test2_delta")
	target, _ = filepath.Abs("/tmp/test1")
	_ = os.MkdirAll(source, 0755)
	_ = os.MkdirAll(delta, 0755)
	_ = os.MkdirAll(target, 0755)
	return source, delta, target
}

func tearDown(source, delta, target string) {
	os.RemoveAll(source)
	os.RemoveAll(delta)
	os.RemoveAll(target)
}

func TestMount(t *testing.T) {
	source, delta, target := setUp()
	defer tearDown(source, delta, target)

	var err error
	var mounted bool

	err = MountOverlay(source, delta, target)
	if err != nil {
		t.Errorf("Failed to overlay %s over %s", source, target)
	}
	mounted, err = Mounted(source)
	if err != nil || !mounted {
		t.Errorf("Endpoint %s is not mounted", source)
	}

	UnmountOverlay(source)
	mounted, err = Mounted(source)
	if err != nil {
		t.Errorf("Failed to check mounted status of %s", source)
	}
	if mounted {
		t.Errorf("Failed to unmount %s", source)
	}
}
