package mount

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	if stdout, err := command.CombinedOutput(); err != nil {
		fmt.Printf("Command %v failed with output %s\n", command, string(stdout))
		return err
	}
	return nil
}

func umount(target string) error {
	return syscall.Unmount(target, 0)
}

func Mount(source, target, fstype string) (err error) {
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	mounted, err := Mounted(target)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}
	return runCommand("mount", "-n", "-t", fstype, source, target)
}

func Union(unionType, target, upper, lower string) (err error) {
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(upper, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(lower, 0755); err != nil {
		return err
	}
	var mountOptions string
	mounted, err := Mounted(target)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}
	if unionType == "aufs" {
		mountOptions = "br=" + upper + ":" + lower
	} else if unionType == "overlayfs" {
		mountOptions = "rw,upperdir=" + upper + ",lowerdir=" + lower
	} else {
		return fmt.Errorf("%s is not a supported union type", unionType)
	}
	return runCommand("mount", "-n", "-t", unionType, "-o", mountOptions,
		"none", target)
}

func MountBind(source, target string) error {
	if err := os.MkdirAll(source, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	mounted, err := Mounted(target)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}
	return runCommand("mount", "--bind", source, target)
}

func Symlink(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Symlink(source, target)
}

func Unmount(mountType, target string) error {
	if mountType == "aufs" {
		if err := exec.Command("auplink", target, "flush").Run(); err != nil {
			fmt.Printf("[warning]: couldn't run auplink before unmount: %s", err)
		}
	}
	if err := umount(target); err != nil {
		return err
	}
	// Even though we just unmounted the filesystem, AUFS will prevent deleting the mntpoint
	// for some time. We'll just keep retrying until it succeeds.
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
