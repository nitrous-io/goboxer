package fs

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Ext4Device struct {
	Name          string
	BytesPerInode string
}

func MkfsExt4(d Ext4Device) error {
	if _, err := os.Stat(d.Name); err != nil {
		return err
	}
	fs, err := GetFSType(d)
	if err != nil {
		return err
	}
	if len(fs) > 0 && fs != "ext4" {
		return fmt.Errorf("%s is formatted as %s. Unable to reformat to ext4",
			d.Name, fs)
	}
	if fs == "ext4" {
		return nil
	}
	cmd := exec.Command("mkfs.ext4", "-i", d.BytesPerInode, d.Name)
	return cmd.Run()
}

func GetFSType(d Ext4Device) (string, error) {
	stdout, err := exec.Command("blkid", "-o", "value", "-s", "TYPE",
		d.Name).Output()
	if err != nil {
		// 2 means device doesnt have any fs
		if err.Error() == "exit status 2" {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}
