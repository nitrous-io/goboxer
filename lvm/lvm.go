package lvm

import (
	"os"
	"os/exec"
	"path"
	"strings"
)

func CreateVolume(vg, pool, diskLimit, name string) error {
	poolName := path.Join(vg, pool)
	fullName := path.Join("/dev", "mapper", vg+"-"+nameToMapper(name))
	if _, err := os.Stat(fullName); err == nil {
		return nil
	}
	cmd := exec.Command("lvcreate", "-T", poolName, "-V", diskLimit, "-n", name)
	return cmd.Run()
}

func DestroyVolume(vg, name string) error {
	fullName := path.Join("/dev", vg, name)
	if _, err := os.Stat(fullName); os.IsNotExist(err) {
		return nil
	}
	cmd := exec.Command("lvremove", "-f", fullName)
	return cmd.Run()
}

func nameToMapper(name string) string {
	return strings.Replace(name, "-", "--", -1)
}
