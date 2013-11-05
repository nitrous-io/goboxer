package goboxer

import (
	"bufio"
	"fmt"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/quota/quotatest"
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
	Aufs             bool     `json:"aufs"`
	MountMethod      string   `json:"mount"`
	QuotaMethod      string   `json:"quotaMethod"`
	SelectedVG       string   `json:"selectedVG"`
	Runtime          string   `json:"-"`
	SnapshotURL      string   `json:"-"`
	PublicKeys       []string `json:"-"`
	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
}

func (b *Box) Create() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Create", b.ContainerName()),
	)(time.Now())
	if err := b.Mount(b.boxer.Root); err != nil {
		return err
	}
	if err := b.setupContainer(); err != nil {
		return err
	}
	if err := b.SetQuota(); err != nil {
		return err
	}
	if err := b.SetPortForwarding(); err != nil {
		return err
	}
	return nil
}

func (b *Box) Start() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Start", b.ContainerName()),
	)(time.Now())
	if err := b.Mount(b.boxer.Root); err != nil {
		return err
	}
	return lxc.Start(b.ContainerName())
}

func (b *Box) Stop() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Stop", b.ContainerName()),
	)(time.Now())
	if err := lxc.Stop(b.ContainerName()); err != nil {
		return err
	}
	return b.Unmount(b.boxer.Root)
}

func (b *Box) Restart() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Restart", b.ContainerName()),
	)(time.Now())
	if err := b.Stop(); err != nil {
		return err
	}
	return b.Start()
}

func (b *Box) Destroy() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Destroy", b.ContainerName()),
	)(time.Now())
	if err := b.Stop(); err != nil {
		return err
	}
	if err := b.UnsetPortForwarding(); err != nil {
		return err
	}
	if err := b.UnsetQuota(); err != nil {
		return err
	}
	if err := b.DeleteContainerData(); err != nil {
		return err
	}
	return nil
}

func (b *Box) ContainerName() string  { return fmt.Sprintf("%s-%s", b.Name, b.Id) }
func (b *Box) TemplatePath() string   { return b.Template + ".tar.gz" }
func (b *Box) SetPublicKeys() error   { return b.AddPublicKey(b.PublicKeys...) }
func (b *Box) PublicKeysPath() string { return path.Join(b.RootFS(), "etc", "ssh", "univ_public_keys") }
func (b *Box) PrivateKeyPath() string { return path.Join("home", "action", ".ssh", "id_rsa") }
func (b *Box) RootFS() string         { return path.Join(b.Disk, b.ContainerName(), "rootfs") }
func (b *Box) ContainerFS() string    { return path.Join(b.Disk, b.ContainerName()) }
func (b *Box) EnvPath() string        { return path.Join(b.RootFS(), "etc", "environment") }

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

func (b *Box) SetQuota() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetQuota", b.ContainerName()),
	)(time.Now())
	return b.quota().SetLimit(b.quotaArg())
}

func (b *Box) UnsetQuota() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	return b.quota().RemoveLimit(b.quotaArg())
}

func (b *Box) ChangeSpec(spec *BoxSpec) error {
	if spec.Memory != b.Memory {
		b.Memory = spec.Memory
		if err := b.WriteConfig(); err != nil {
			return err
		}
	}
	if spec.DiskSpace != b.DiskSpace {
		oldDiskSpace := b.DiskSpace
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(); err != nil {
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

func (b *Box) SetPermission() error {
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
	actionPath := path.Join(b.RootFS(), "etc", "action")
	boxIdPath := path.Join(actionPath, "box_id")
	if err := os.MkdirAll(actionPath, 0755); err != nil {
		return err
	}
	boxIdFile, err := os.Create(boxIdPath)
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
	defer logger.LogDuration(
		fmt.Sprintf("%s:ExtractTemplate", b.ContainerName()),
	)(time.Now())
	boxDeltaPath := path.Join(b.Disk, b.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	homeDataPath := path.Join(b.Disk, b.TemplatePath())
	return Untar(homeDataPath, boxDeltaHomePath)
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
	switch b.GetMountMethod() {
	case "overlayfs", "aufs":
		if err := mount.Union(b.GetMountMethod(), boxPath, boxDeltaPath, baseContainerPath); err != nil {
			return err
		}
		if err := os.MkdirAll(lxcBoxPath, 0755); err != nil {
			return err
		}
		// FIXME: MountBind to make it similar to boxer. Ideally we probably
		// want to symlink as it doesnt make any difference and seems to be
		// more stable. Less mountpoint is better
		return mount.MountBind(boxPath, lxcBoxPath)
	case "lvm":
		return fmt.Errorf("%s is not yet supported", b.GetMountMethod())
	default:
		return fmt.Errorf("%s is not a supported mount method",
			b.GetMountMethod())
	}
}

func (b *Box) Unmount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	lxcBoxPath := path.Join(root, b.ContainerName())
	if err := mount.Unmount(lxcBoxPath); err != nil {
		return err
	}
	if err := os.RemoveAll(lxcBoxPath); err != nil {
		return err
	}
	return mount.Unmount(b.ContainerFS())
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
func (b *Box) StorageUsage() (int, error) {
	quotaInfo, err := b.quota().ViewLimit(quota.QuotaArg{Id: b.Id, Disk: b.Disk})
	if err != nil {
		return 0, err
	}
	return quotaInfo.Used, nil
}

// Return BoxInfo with `MB` units
func (b *Box) Info() (BoxInfo, error) {
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
	if storageUsage, err := b.StorageUsage(); err != nil {
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
		// FIXME: probably going through the file once and remove all is faster
		if err := DeleteLineContains(b.EnvPath(), key+"="); err != nil {
			return err
		}
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

// Populate the contents of user's container. This should be done after the
// container is mounted/unionized so we only works on the mount point and not
// the delta storage.
func (b *Box) setupContainer() error {
	if err := b.ExtractTemplate(); err != nil {
		return err
	}
	if err := b.WriteConfig(); err != nil {
		return err
	}
	if err := b.SetHostname(); err != nil {
		return err
	}
	if err := b.SetNetworkConfig(); err != nil {
		return err
	}
	if err := b.SetWorkspace(); err != nil {
		return err
	}
	if err := b.SetDefaultKeypair(); err != nil {
		return err
	}
	if err := b.SetPermission(); err != nil {
		return err
	}
	if err := b.SetBoxId(); err != nil {
		return err
	}
	if err := b.SetPublicKeys(); err != nil {
		return err
	}
	return nil
}

func (b *Box) quotaArg() quota.QuotaArg {
	return quota.QuotaArg{
		Id:    b.Id,
		Name:  b.ContainerName(),
		Path:  b.HomePath(),
		Disk:  b.Disk,
		Limit: b.DiskSpace,
	}
}

func (b *Box) GetMountMethod() string {
	if b.MountMethod == "lvm" {
		return "lvm"
	} else if b.MountMethod == "aufs" || b.Aufs {
		return "aufs"
	} else if b.MountMethod == "failed" {
		return "failed"
	}
	return "overlayfs"
}

func (b *Box) GetQuotaMethod() string {
	if b.QuotaMethod == "usrquota" {
		return "usrquota"
	}
	return "prjquota"
}

// return the quota strategy that the box should use. The return object will be
// an instance that supports quota.QuotaInterface interface
//
// We specifically supports MountMethod: "failed" here so we dont have to
// inject special quota instance in test. This should never happens in
// production
func (b *Box) quota() quota.QuotaInterface {
	if b.GetMountMethod() == "lvm" {
		return quota.LvmQuota{}
	} else if b.GetQuotaMethod() == "usrquota" {
		return quota.UsrQuota{}
	} else if b.GetQuotaMethod() == "failed" {
		return quotatest.FailQuota{}
	}
	return quota.PrjQuota{}
}
