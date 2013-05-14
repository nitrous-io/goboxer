package goboxer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func MountOverlay(source, delta, target string) (err error) {
	mountOptions := "rw,upperdir=" + delta + ",lowerdir=" + target
	mounted, err := Mounted(target)
	if err != nil {
		Debug("err", err)
		return err
	}
	if mounted {
		Debug("mounted")
		return nil
	}
	return exec.Command("mount", "-n", "-t", "overlayfs", "-o", mountOptions,
		"none", source).Run()
}

func UnmountOverlay(target string) error {
	mounted, err := Mounted(target)
	if err != nil {
		return err
	}
	if !mounted {
		return nil
	}
	if err := syscall.Unmount(target, 0); err != nil {
		return err
	}
	// We'll just keep retrying until umount success
	for retries := 0; retries < 1000; retries++ {
		err := os.Remove(target)
		if err == nil {
			// rm mntpoint succeeded
			return nil
		}
		if os.IsNotExist(err) {
			// mntpoint doesn't exist anymore. Success.
			return nil
		}
		// fmt.Printf("(%v) Remove %v returned: %v\n", retries, target, err)
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
