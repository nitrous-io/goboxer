package goboxer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func umount(target string) error {
	return syscall.Unmount(target, 0)
}

func MountOverlay(source, delta, target string) (err error) {
	mountOptions := "rw,upperdir=" + delta + ",lowerdir=" + target
	mounted, err := Mounted(target)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}
	return exec.Command("mount", "-n", "-t", "overlayfs", "-o", mountOptions,
		"none", source).Run()
}

func Symlink(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Symlink(source, target)
}

func UnmountOverlay(target string) error {
	// We'll just keep retrying until umount success
	for retries := 0; retries < 1000; retries++ {
		mounted, err := Mounted(target)
		if err != nil {
			return err
		}
		if !mounted {
			return nil
		}
		if err := umount(target); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("Umount: Failed to umount %v", target)
}

func Mounted(mountpoint string) (bool, error) {
	mntpoint, err := os.Stat(mountpoint)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	parent, err := os.Stat(filepath.Join(mountpoint, ".."))
	if err != nil {
		return false, err
	}
	mntpointSt := mntpoint.Sys().(*syscall.Stat_t)
	parentSt := parent.Sys().(*syscall.Stat_t)
	return mntpointSt.Dev != parentSt.Dev, nil
}
