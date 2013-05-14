package goboxer

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

const UidStart = 15000

type Box struct {
	Config   *Config
	Resource *Resource
}

type Config struct {
	Id          int
	Name        string
	Runtime     string
	Template    string
	Memory      string
	DiskSpace   string
	PublicKeys  []string
	SnapshotURL string
}

func (b *Box) ContainerName() string {
	return fmt.Sprintf("%s-%d", b.Config.Name, b.Config.Id)
}

func (b *Box) Id() int {
	return b.Config.Id
}

func (b *Box) DiskSpace() string {
	return b.Config.DiskSpace
}

func (b *Box) Template() string {
	return b.Config.Template + ".tar.gz"
}

func (b *Box) Hostname() string {
	return b.ContainerName()
}

func (b *Box) NetworkConfig() string {
	return b.Resource.NetworkInterface
}

func (b *Box) Uid() string {
	return strconv.Itoa(UidStart + b.Id())
}

func (b *Box) Gid() string {
	return strconv.Itoa(UidStart + b.Id())
}

func (b *Box) BoxIdFile() string {
	return path.Join(b.RootFS(), "etc", "action", "box_id")
}

func (b *Box) Port() string {
	return b.Resource.Port
}

func (b *Box) IpAddress() string {
	return b.Resource.Ip
}

func (b *Box) NetworkInterface() string {
	return b.Resource.NetworkInterface
}

// http://www.funtoo.org/wiki/Linux_Containers
// http://serverfault.com/questions/40712/what-range-of-mac-addresses-can-i-safely-use-for-my-virtual-machines
func (b *Box) MacAddress() string {
	ips := strings.Split(b.IpAddress(), ".")
	ipList := make([]interface{}, len(ips))
	for i, v := range ips {
		val, _ := strconv.Atoi(v)
		ipList[i] = interface{}(val)
	}
	return fmt.Sprintf("02:00:%x:%x:%x:%x", ipList...)
}

func (b *Box) Memory() string {
	return b.Config.Memory
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

func (b *Box) ToJSON() (string, error) {
	return "", nil
}
