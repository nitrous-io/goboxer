package goboxer

import (
	"bufio"
	"fmt"
	"github.com/action-io/goboxer/fs"
	"github.com/action-io/goboxer/lvm"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const UidStart = 15000
const ThinVG = "boxes"

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
	if b.GetMountMethod() != "lvm" {
		if err := b.SetQuota(); err != nil {
			return err
		}
	}
	if err := b.setPortForwarding(); err != nil {
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
	if err := b.unsetPortForwarding(); err != nil {
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

func (b *Box) ContainerName() string {
	return b.Name + "-" + b.Id
}

func (b *Box) PublicKeysPath() string {
	return path.Join(b.RootFS(), "etc", "ssh", "univ_public_keys")
}

func (b *Box) LxcPath() string {
	return path.Join(b.boxer.Root, b.ContainerName())
}

func (b *Box) ContainerFS() string {
	if b.GetMountMethod() == "lvm" {
		return b.LxcPath()
	} else {
		return path.Join(b.Disk, b.ContainerName())
	}
}

func (b *Box) EnvPath() string {
	return path.Join(b.RootFS(), "etc", "environment")
}

// rootfs of the container. This is the root after unionized so its typically
// will be in boxer.Root i.e /var/lib/lxc
func (b *Box) RootFS() string {
	return path.Join(b.boxer.Root, b.ContainerName(), "rootfs")
}

// path of the box's templates.  (i.e. ruby_template.tar.gz,
// python_template.tar.gz etc)
func (b *Box) TemplatePath() string {
	templateName := b.Template + ".tar.gz"
	if b.GetMountMethod() == "lvm" {
		return path.Join(b.boxer.Root, templateName)
	}
	return path.Join(b.Disk, templateName)
}

func (b *Box) ContainerDeltaPath() string {
	if b.GetMountMethod() == "lvm" {
		return b.ContainerFS() + "_delta"
	}
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
		if err := b.writeConfig(); err != nil {
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
	return b.Restart()
}

func (b *Box) Mount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:Mount", b.ContainerName()),
	)(time.Now())

	lxcBoxPath := b.LxcPath()
	boxDeltaPath := b.ContainerDeltaPath()
	switch b.GetMountMethod() {
	case "overlayfs", "aufs":
		// overlayfs or aufs strategy
		//
		// we are going to create a delta folder which will hold user data,
		// union with the base container template in the same disk, and mount
		// it over to root ( /var/lib/lxc )
		boxPath := b.ContainerFS()
		baseContainerPath := path.Join(b.Disk, "base_container_template")
		if err := mount.Union(b.GetMountMethod(), boxPath, boxDeltaPath, baseContainerPath); err != nil {
			fmt.Println("failed to union")
			return err
		}
		// FIXME: MountBind to make it similar to boxer. Ideally we probably
		// want to symlink as it doesnt make any difference and seems to be
		// more stable. Less mountpoint is better
		return mount.MountBind(boxPath, lxcBoxPath)
	case "lvm":
		// lvm strategy
		// we are going to create an independent thin pool lv with size equals
		// to DiskSpace, format it as ext4 volume and mount the volume as the
		// box delta folder
		if err := lvm.CreateVolume(b.SelectedVG, ThinVG, b.DiskSpace,
			b.ContainerName()); err != nil {
			return err
		}
		deviceName := fmt.Sprintf("/dev/%s/%s", b.SelectedVG, b.ContainerName())
		if err := fs.MkfsExt4(fs.Ext4Device{Name: deviceName, BytesPerInode: "8192"}); err != nil {
			return err
		}
		boxDeltaPath := lxcBoxPath + "_delta"
		baseContainerPath := path.Join(root, "base_container_template")
		if err := mount.Mount(deviceName, boxDeltaPath, "ext4"); err != nil {
			return err
		}
		return mount.Union("aufs", lxcBoxPath, boxDeltaPath, baseContainerPath)
	default:
		return fmt.Errorf("%s is not a supported mount method",
			b.GetMountMethod())
	}
}

func (b *Box) Unmount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	lxcBoxPath := b.LxcPath()
	if err := mount.Unmount(lxcBoxPath); err != nil {
		return err
	}
	if err := os.RemoveAll(lxcBoxPath); err != nil {
		return err
	}
	switch b.GetMountMethod() {
	case "overlayfs", "aufs":
		boxPath := b.ContainerFS()
		return mount.Unmount(boxPath)
	case "lvm":
		boxDeltaPath := lxcBoxPath + "_delta"
		return mount.Unmount(boxDeltaPath)
	default:
		return fmt.Errorf("%s is not a supported unmount method",
			b.GetMountMethod())
	}
	return mount.Unmount(b.ContainerFS())
}

func (b *Box) DeleteContainerData() error {
	if b.GetMountMethod() == "lvm" {
		if err := lvm.DestroyVolume(b.SelectedVG, b.ContainerName()); err != nil {
			return err
		}
	}
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
	quotaInfo, err := b.quota().ViewLimit(b.quotaArg())
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
	// write lxc config of the box
	if err := b.writeConfig(); err != nil {
		return err
	}
	root := b.RootFS()

	// extract template
	boxHomePath := path.Join(root, "home", "action")
	if err := Untar(b.TemplatePath(), boxHomePath); err != nil {
		return err
	}

	// set box's id
	actionPath := path.Join(root, "etc", "action")
	boxIdPath := path.Join(actionPath, "box_id")
	if err := os.MkdirAll(actionPath, 0755); err != nil {
		return err
	}
	boxIdFile, err := os.Create(boxIdPath)
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	if _, err = boxIdFile.Write([]byte(b.Id)); err != nil {
		return err
	}

	// set public keys
	if err := b.AddPublicKey(b.PublicKeys...); err != nil {
		return err
	}

	// set box's /etc/hostname
	hostNamePath := path.Join(root, "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	if _, err := hostName.Write([]byte(b.ContainerName())); err != nil {
		return err
	}

	// set /etc/network/interfaces
	networkConfigPath := path.Join(root, "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	if err := NetworkConfigCompiled.Execute(networkConfig, b); err != nil {
		return err
	}

	// set workspace
	workspacePath := path.Join(root, "home", "action", "workspace")
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return err
	}

	// set default key pair
	sshPath := path.Join(root, "home", "action", ".ssh")
	sshKeyFile := path.Join(sshPath, "id_rsa")
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

	if err := b.setPermission(); err != nil {
		return err
	}
	return nil
}

func (b *Box) quotaArg() quota.QuotaArg {
	arg := quota.QuotaArg{
		Id:    b.Id,
		Name:  b.ContainerName(),
		Disk:  b.Disk,
		Limit: b.DiskSpace,
	}
	if b.GetQuotaMethod() == "lvm" {
		arg.Path = b.ContainerDeltaPath()
	} else {
		arg.Path = b.HomePath()
	}
	return arg
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
	if b.GetMountMethod() == "lvm" {
		return "lvm"
	}
	if b.QuotaMethod == "" {
		return "prjquota"
	}
	return b.QuotaMethod
}

// return the quota strategy that the box should use. The return object will be
// an instance that supports quota.QuotaInterface interface
func (b *Box) quota() quota.QuotaInterface {
	return quota.Provisioner.GetQuota(b.GetQuotaMethod())
}

// rewrite lxc config of the box
func (b *Box) writeConfig() error {
	lxcConfigPath := path.Join(b.boxer.Root, b.ContainerName(), "config")
	lxcConfig, err := os.Create(lxcConfigPath)
	if err != nil {
		return err
	}
	defer lxcConfig.Close()
	return LxcTemplateCompiled.Execute(lxcConfig, b)
}

// set correct permission for user's home folder so he/she can write onto it
func (b *Box) setPermission() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetPermission", b.ContainerName()),
	)(time.Now())
	uid := b.Uid()
	gid := b.Gid()
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	root := b.RootFS()
	if err := RunCommand("chroot", root, "usermod", "-u", uid, "action"); err != nil {
		return err
	}
	if err := RunCommand("chroot", root, "groupmod", "-g", gid, "action"); err != nil {
		return err
	}
	return RunCommand("chroot", root, "chown", "-R", permission, "/home/action")
}

func (b *Box) setPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	return RunCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", strPort, "-j", "DNAT", "--to",
		b.Ip+":22")
}

func (b *Box) unsetPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	return RunCommand("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
		"--dport", strPort, "-j", "DNAT", "--to", b.Ip+":22")
}
