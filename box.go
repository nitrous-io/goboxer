package goboxer

import (
	"bufio"
	"fmt"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/s3"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const UidStart = 15000

type BoxSpec struct {
	Memory    string `json:"memory,omitempty"`
	DiskSpace string `json:"diskSpace,omitempty"`
}

type BoxInfo struct {
	StorageUsage float64 `json:"storageUsage,omitempty"`
	MemoryUsage  float64 `json:"memoryUsage,omitempty"`
	LxcStatus    string  `json:"lxcStatus,omitempty"`
}

type Box struct {
	Id               string   `json:"id"`
	Name             string   `json:"name"`
	Template         string   `json:"containerTemplate"`
	Memory           string   `json:"memory"`
	DiskSpace        string   `json:"diskSpace"`
	Ip               string   `json:"ip"`
	Gateway          string   `json:"gateway"`
	NetworkInterface string   `json:"interface"`
	Port             int      `json:"port"`
	BoxerId          string   `json:"boxerId"`
	State            string   `json:"containerState"`
	Disk             string   `json:"disk"`
	Runtime          string   `json:"-"`
	SnapshotURL      string   `json:"-"`
	PublicKeys       []string `json:"-"`
	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
}

func (b *Box) ContainerName() string { return fmt.Sprintf("%s-%s", b.Name, b.Id) }
func (b *Box) SetPublicKeys() error  { return b.AddPublicKey(b.PublicKeys...) }

func (b *Box) BoxIdFile() string      { return path.Join(b.RootFS(), "etc", "action", "box_id") }
func (b *Box) PublicKeysPath() string { return path.Join(b.RootFS(), "etc", "ssh", "univ_public_keys") }
func (b *Box) PrivateKeyPath() string { return path.Join("home", "action", ".ssh", "id_rsa") }
func (b *Box) RootFS() string         { return path.Join(b.Disk, b.ContainerName(), "rootfs") }
func (b *Box) Home() string           { return path.Join(b.RootFS(), "home", "action") }
func (b *Box) ContainerFS() string    { return path.Join(b.Disk, b.ContainerName()) }
func (b *Box) EnvPath() string        { return path.Join(b.RootFS(), "etc", "environment") }

func (b *Box) TarName() string {
	return fmt.Sprintf("%s-%v.tar.gz", b.ContainerName(), time.Now().Unix())
}

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
	m := quota.ParseValueUnit(b.Memory)
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

func (b *Box) SetQuota(q quota.QuotaInterface) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetQuota", b.ContainerName()),
	)(time.Now())
	return q.SetLimit(b.Id, b.ContainerName(), b.HomePath(), b.Disk, b.DiskSpace)
}

func (b *Box) UnsetQuota(q quota.QuotaInterface) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	return q.RemoveLimit(b.Id, b.ContainerName(), b.HomePath(), b.Disk)
}

func (b *Box) ChangeSpec(spec *BoxSpec, q quota.QuotaInterface) error {
	if spec.Memory != b.Memory {
		b.Memory = spec.Memory
		if err := b.WriteConfig(); err != nil {
			return err
		}
	}
	if spec.DiskSpace != b.DiskSpace {
		oldDiskSpace := b.DiskSpace
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(q); err != nil {
			b.DiskSpace = oldDiskSpace
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

func (b *Box) SetPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	return RunCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", strPort, "-j", "DNAT", "--to",
		b.Ip+":22")
}

func (b *Box) UnsetPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	return RunCommand("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
		"--dport", strPort, "-j", "DNAT", "--to", b.Ip+":22")
}

func (b *Box) StopContainer() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:StopContainer", b.ContainerName()),
	)(time.Now())
	return lxc.Stop(b.ContainerName())
}

func (b *Box) StartContainer() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:StartContainer", b.ContainerName()),
	)(time.Now())
	return lxc.Start(b.ContainerName())
}

func (b *Box) Mount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:Mount", b.ContainerName()),
	)(time.Now())

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
	b.boxer.Lock()
	defer b.boxer.Unlock()
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

// Return current memory usage of the box as memory cgroup reported it
// the unit if B, so if we want to convert to MB, it needs to be divided with
// 1024 * 1024
func (b *Box) MemoryUsageInBytes() (int, error) {
	filename := fmt.Sprintf("/sys/fs/cgroup/memory/lxc/%s/memory.usage_in_bytes",
		b.ContainerName())
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return strconv.Atoi(scanner.Text())
}

// Return current state of the box. It is the string that lxc used to annotate
// container's state such as
// - RUNNING
// - STOPPED
func (b *Box) LxcState() (string, error) {
	return lxc.Status(b.ContainerName())
}

// Return current storage usage in kb
func (b *Box) StorageUsage(q quota.QuotaInterface) (int, error) {
	quotaInfo, err := q.ViewLimit(b.Id, b.Disk)
	if err != nil {
		return 0, err
	}
	return quotaInfo.Used, nil
}

// Return BoxInfo with `MB` units
func (b *Box) Info(q quota.QuotaInterface) (BoxInfo, error) {
	var info BoxInfo
	if memoryUsage, err := b.MemoryUsageInBytes(); err != nil {
		return info, err
	} else {
		info.MemoryUsage = float64(memoryUsage) / (1024 * 1024)
	}
	if state, err := b.LxcState(); err != nil {
		return info, err
	} else {
		info.LxcStatus = state
	}
	if storageUsage, err := b.StorageUsage(q); err != nil {
		return info, err
	} else {
		info.StorageUsage = quota.ScaleUnit(storageUsage, "M")
	}
	return info, nil
}

// set env of the box
// This set env as KEY=value lines into /etc/environment of the box, which
// should be loaded automatically when environment starts
func (b *Box) SetEnv(data map[string]interface{}) error {
	newEnv := []string{}
	for key, value := range data {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", key, value.(string)))
	}
	return Appendln(b.EnvPath(), strings.Join(newEnv, "\n"))
}

// Remove env variables from the box
func (b *Box) RemoveEnv(data []string) (err error) {
	for _, value := range data {
		if err = DeleteLineContains(b.EnvPath(), value+"="); err != nil {
			return err
		}
	}
	return
}

// tar the box delta content.
// This returns the path that we get the compress content, and any errors that
// we had during the compression
func (b Box) Tar() (compressedPath string, err error) {
	return Tar(b.TarName(), b.Home())
}

// if the box has SnapshotURL, then we use that to populate the home directory,
// otherwise, we use Runtime to get a base on which we do more interesting
// things such as setting up keys, workspace etc
func (b Box) SetupHome(auth s3.AuthInterface) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetupHome", b.ContainerName()),
	)(time.Now())

	if len(b.SnapshotURL) > 0 {
		// parse snapshot url which should be an s3 path
		url, err := s3.ParseUrl(b.SnapshotURL)
		if err != nil {
			return err
		}
		client, err := auth.NewClient(url.Region, url.Bucket)
		if err != nil {
			return err
		}
		stream, err := client.Download(url.File)
		if err != nil {
			return err
		}
		boxDeltaHomePath := path.Join(b.ContainerDeltaPath(), "rootfs", "home", "action")
		if err := UntarStream(stream, boxDeltaHomePath); err != nil {
			return nil
		}
	} else {
		templatePath := b.Template + ".tar.gz"
		boxDeltaHomePath := path.Join(b.ContainerDeltaPath(), "rootfs", "home", "action")
		homeDataPath := path.Join(b.Disk, templatePath)
		if err := Untar(homeDataPath, boxDeltaHomePath); err != nil {
			return err
		}
		if err := b.setWorkspace(); err != nil {
			return err
		}
		if err := b.setDefaultKeypair(); err != nil {
			return err
		}
	}
	return b.setPermission()
}

// set correct permission to user home folder.
// This is necessary because the permission comes with overlay folders are
// incorrect
func (b *Box) setPermission() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetPermission", b.ContainerName()),
	)(time.Now())
	uid := b.Uid()
	gid := b.Gid()
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	if err := RunCommand("chroot", b.RootFS(), "usermod", "-u", uid, "action"); err != nil {
		return err
	}
	if err := RunCommand("chroot", b.RootFS(), "groupmod", "-g", gid, "action"); err != nil {
		return err
	}
	return RunCommand("chroot", b.RootFS(), "chown", "-R", permission, "/home/action")
}

// create a workspace folder for home directory
func (b *Box) setWorkspace() error {
	workspacePath := path.Join(b.RootFS(), "home", "action", "workspace")
	return os.MkdirAll(workspacePath, 0755)
}

// set default keypair for a new box. This generally should only be called when
// the box is newly setup from a preconfigured runtime
func (b *Box) setDefaultKeypair() error {
	sshPath := path.Join(b.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(b.RootFS(), b.PrivateKeyPath())
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		return RunCommand("/bin/sh", "-c", sshKeygen)
	} else {
		return err
	}
}
