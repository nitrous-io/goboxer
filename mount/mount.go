package mount

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
	return remove(target)
}

// from ruby's Pathname#mountpoint?
// http://www.ruby-doc.org/stdlib-1.9.3/libdoc/pathname/rdoc/Pathname.html#method-i-mountpoint-3F
func Mounted(mountpoint string) (bool, error) {
	var stat1, stat2 os.FileInfo
	var err error
	if stat1, err = os.Stat(mountpoint); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if stat2, err = os.Stat(filepath.Join(mountpoint, "..")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	st1 := stat1.Sys().(*syscall.Stat_t)
	st2 := stat2.Sys().(*syscall.Stat_t)
	mounted := (st1.Dev == st2.Dev && st1.Ino == st2.Ino) || st1.Dev != st2.Dev
	return mounted, nil
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
