package mount

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func mount(source string, target string, fstype string, flags uintptr, data string) (err error) {
	return syscall.Mount(source, target, fstype, flags, data)
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
	return mount(source, target, fstype, 0, "")
}

func Union(unionType, target, upper, lower string) (err error) {
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("mount:Union failed mkdir %v", target)
	}
	if err := os.MkdirAll(upper, 0755); err != nil {
		return fmt.Errorf("mount:Union failed mkdir %v", upper)
	}
	if err := os.MkdirAll(lower, 0755); err != nil {
		return fmt.Errorf("mount:Union failed mkdir %v", lower)
	}
	var mountOptions string
	mounted, err := Mounted(target)
	if err != nil {
		return fmt.Errorf("mount:Union failed check mounted %v", err)
	}
	if mounted {
		return nil
	}
	if unionType == "aufs" {
		mountOptions = "br=" + upper + ":" + lower
	} else if unionType == "overlayfs" {
		mountOptions = "upperdir=" + upper + ",lowerdir=" + lower
	} else {
		return fmt.Errorf("%s is not a supported union type", unionType)
	}
	err = mount("none", target, unionType, 0, mountOptions)
	if err != nil {
		return fmt.Errorf("mount:Union failed mount %v", err)
	}
	return nil
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
	return mount(source, target, "", syscall.MS_BIND, "")
}

func Symlink(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Symlink(source, target)
}

func Unmount(mountType, target string) error {
	fmt.Printf("Unmount: %s with mount type %s\n", target, mountType)
	if mountType == "aufs" {
		if err := exec.Command("auplink", target, "flush").Run(); err != nil {
			fmt.Printf("[warning]: couldn't run auplink before unmount: %s", err)
		}
	}
	// if the target is not mounted, do not perform unmount here
	mounted, err := Mounted(target)
	if err != nil {
		return fmt.Errorf("%s failed to check mount status: %v", target, err)
	}
	if !mounted {
		return remove(target)
	}
	if err := umount(target); err != nil {
		return fmt.Errorf("%s failed to umount: %v", target, err)
	}
	// Even though we just unmounted the filesystem, AUFS will prevent deleting the mntpoint
	// for some time. We'll just keep retrying until it succeeds.
	for retries := 0; retries < 1000; retries++ {
		err := remove(target)
		if err == nil {
			return nil
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

func remove(target string) error {
	err := os.RemoveAll(target)
	if err == nil {
		// rm mntpoint succeeded
		return nil
	}
	if os.IsNotExist(err) {
		// mntpoint doesn't exist anymore. Success.
		return nil
	}
	return err
}
