package goboxer

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const UidStart = 15000

type BoxSpec struct {
	Memory    string `json:"memory"`
	DiskSpace string `json:"diskSpace"`
}

type Box struct {
	Id               string `json:"id"`
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
	Disk             string `json:"disk"`
	Runtime          string
	SnapshotURL      string
	PublicKeys       []string
}

func (b *Box) ContainerName() string { return fmt.Sprintf("%s-%s", b.Name, b.Id) }
func (b *Box) TemplatePath() string  { return b.Template + ".tar.gz" }
func (b *Box) SetPublicKeys() error  { return b.AddPublicKey(b.PublicKeys...) }

func (b *Box) BoxIdFile() string      { return path.Join(b.RootFS(), "etc", "action", "box_id") }
func (b *Box) PublicKeysPath() string { return path.Join(b.RootFS(), "etc", "ssh", "univ_public_keys") }
func (b *Box) PrivateKeyPath() string { return path.Join("home", "action", ".ssh", "id_rsa") }
func (b *Box) RootFS() string         { return path.Join(b.Disk, b.ContainerName(), "rootfs") }
func (b *Box) ContainerFS() string    { return path.Join(b.Disk, b.ContainerName()) }

func (b *Box) ContainerDeltaPath() string {
	return path.Join(b.Disk, b.ContainerName()+"_delta")
}

func (b *Box) Uid() string {
	id, _ := strconv.Atoi(b.Id)
	return strconv.Itoa(UidStart + id)
}

func (b *Box) Gid() string {
	id, _ := strconv.Atoi(b.Id)
	return strconv.Itoa(UidStart + id)
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

func (b *Box) TotalMemory() string {
	m := ParseValueUnit(b.Memory)
	return fmt.Sprintf("%d%s", int(m.Value)*3, m.Unit)
}

func (b *Box) AddPublicKey(keys ...string) error {
	return Appendln(b.PublicKeysPath(), strings.Join(keys, "\n"))
}

func (b *Box) RemovePublicKey(key string) error {
	return DeleteLineContains(b.PublicKeysPath(), key)
}

func (b *Box) HomePath() string {
	return path.Join(b.Disk, b.ContainerName()+"_delta", "rootfs", "home", "action")
}

func (b *Box) SetQuota() error {
	return SetQuotaLimit(b.Id, b.ContainerName(), b.HomePath(), b.Disk, b.DiskSpace)
}

func (b *Box) UnsetQuota() error {
	return RemoveQuotaLimit(b.Id, b.ContainerName(), b.HomePath(), b.Disk)
}

func (b *Box) ChangeSpec(spec *BoxSpec) error {
	if spec.Memory != b.Memory {
		b.Memory = spec.Memory
		if err := b.WriteConfig(); err != nil {
			return err
		}
	}
	if spec.DiskSpace != b.DiskSpace {
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(); err != nil {
			return err
		}
	}
	return nil
}

func (b *Box) WriteConfig() error {
	lxcConfigPath := path.Join(b.Disk, b.ContainerName(), "config")
	lxcConfig, err := os.Create(lxcConfigPath)
	if err != nil {
		return err
	}
	defer lxcConfig.Close()
	return LxcTemplateCompiled.Execute(lxcConfig, b)
}

func (b *Box) SetPermission() error {
	uid := b.Uid()
	gid := b.Gid()
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	start := time.Now()
	if err := RunCommand("chroot", b.RootFS(), "usermod", "-u", uid, "action"); err != nil {
		return err
	}
	fmt.Printf("set user id took %v\n", time.Now().Sub(start))
	start = time.Now()
	if err := RunCommand("chroot", b.RootFS(), "groupmod", "-g", gid, "action"); err != nil {
		return err
	}
	fmt.Printf("set group id took %v\n", time.Now().Sub(start))
	start = time.Now()
	return RunCommand("chroot", b.RootFS(), "chown", "-R", permission, "/home/action")
}

func (b *Box) SetWorkspace() error {
	workspacePath := path.Join(b.RootFS(), "home", "action", "workspace")
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return err
	}
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	return RunCommand("chroot", b.RootFS(), "chown", "-R", permission,
		"/home/action/workspace")
}

func (b *Box) SetBoxId() error {
	boxIdFile, err := os.Create(b.BoxIdFile())
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	_, err = boxIdFile.Write([]byte(b.Id))
	return err
}

func (b *Box) SetNetworkConfig() error {
	networkConfigPath := path.Join(b.RootFS(), "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, b)
}

func (b *Box) SetHostname() error {
	hostNamePath := path.Join(b.RootFS(), "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.Write([]byte(b.ContainerName()))
	return err
}

func (b *Box) ExtractTemplate() error {
	boxDeltaPath := path.Join(b.Disk, b.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	homeDataPath := path.Join(b.Disk, b.TemplatePath())
	return Untar(homeDataPath, boxDeltaHomePath)
}

func (b *Box) SetPortForwarding() error {
	return RunCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", b.Port, "-j", "DNAT", "--to",
		b.Ip+":22")
}

func (b *Box) UnsetPortForwarding() error {
	return RunCommand("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
		"--dport", b.Port, "-j", "DNAT", "--to", b.Ip+":22")
}

func (b *Box) SetDefaultKeypair() error {
	sshPath := path.Join(b.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(b.RootFS(), b.PrivateKeyPath())
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		if err := RunCommand("/bin/sh", "-c", sshKeygen); err != nil {
			return err
		}
	} else {
		return err
	}

	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	return RunCommand("chroot", b.RootFS(), "chown", "-R", permission,
		"/home/action/.ssh")
}

func (b *Box) StopContainer() error {
	if err := RunCommand("lxc-stop", "-n", b.ContainerName()); err != nil {
		return err
	}
	return WaitLxc(b.ContainerName(), "STOPPED")
}

func (b *Box) StartContainer() error {
	if err := RunCommand("lxc-start", "-d", "-n", b.ContainerName()); err != nil {
		return err
	}
	return WaitLxc(b.ContainerName(), "RUNNING")
}

func (b *Box) Mount(root string) error {
	boxPath := b.ContainerFS()
	boxDeltaPath := b.ContainerDeltaPath()
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	baseContainerPath := path.Join(b.Disk, "base_container_template")
	lxcBoxPath := path.Join(root, b.ContainerName())

	if err := os.MkdirAll(b.ContainerFS(), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(boxDeltaHomePath, 0755); err != nil {
		return err
	}
	if err := MountOverlay(boxPath, boxDeltaPath, baseContainerPath); err != nil {
		return err
	}
	if err := os.RemoveAll(lxcBoxPath); err != nil {
		return err
	}
	return Symlink(boxPath, lxcBoxPath)
}

func (b *Box) Unmount(root string) error {
	lxcBoxPath := path.Join(root, b.ContainerName())
	if err := os.RemoveAll(lxcBoxPath); err != nil {
		return err
	}
	return UnmountOverlay(b.ContainerFS())
}

func (b *Box) DeleteContainerData() error {
	if err := os.RemoveAll(b.ContainerDeltaPath()); err != nil {
		return err
	}
	return os.RemoveAll(b.ContainerFS())
}
