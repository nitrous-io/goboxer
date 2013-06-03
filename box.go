package goboxer

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

const UidStart = 15000

type Box struct {
	Id               int    `json:"id"`
	Name             string `json:"name"`
	Template         string `json:"containerTemplate"`
	Memory           string `json:"memory"`
	DiskSpace        string `json:"diskSpace"`
	Ip               string `json:"ip"`
	Gateway          string `json:"gateway"`
	NetworkInterface string `json:"interface"`
	Port             string `json:"port"`
	BoxerId          string `json:"boxerId"`
	State            string `json:"containerState"`
	Runtime          string
	SnapshotURL      string
	PublicKeys       []string
}

func (b *Box) ContainerName() string {
	return fmt.Sprintf("%s-%d", b.Name, b.Id)
}

func (b *Box) TemplatePath() string {
	return b.Template + ".tar.gz"
}

func (b *Box) Uid() string {
	return strconv.Itoa(UidStart + b.Id)
}

func (b *Box) Gid() string {
	return strconv.Itoa(UidStart + b.Id)
}

func (b *Box) BoxIdFile() string {
	return path.Join(b.RootFS(), "etc", "action", "box_id")
}

// http://www.funtoo.org/wiki/Linux_Containers
// http://serverfault.com/questions/40712/what-range-of-mac-addresses-can-i-safely-use-for-my-virtual-machines
func (b *Box) MacAddress() string {
	ips := strings.Split(b.Ip, ".")
	ipList := make([]interface{}, len(ips))
	for i, v := range ips {
		val, _ := strconv.Atoi(v)
		ipList[i] = interface{}(val)
	}
	return fmt.Sprintf("02:00:%x:%x:%x:%x", ipList...)
}

// TODO: do it properly
func (b *Box) TotalMemory() string {
	return "768M"
}

func (b *Box) PrivateKeyPath() string {
	return path.Join("home", "action", ".ssh", "id_rsa")
}

// TODO: move this elsewhere as rootfs really should depends on boxer,
// box doesnt have anything to do with its own absolute rootfs
func (b *Box) RootFS() string {
	return path.Join("/var/lib/lxc", b.ContainerName(), "rootfs")
}

func (b *Box) ContainerFS() string {
	return path.Join("/var/lib/lxc", b.ContainerName())
}
