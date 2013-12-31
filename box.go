package goboxer

import (
	"encoding/json"
	"fmt"
	"github.com/action-io/goboxer/fs"
	"github.com/action-io/goboxer/ip"
	"github.com/action-io/goboxer/lvm"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/utils"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const UID_START = 15000
const THIN_VG = "boxes"
const DEFAULT_SWAPPINESS = 60
const DEFAULT_CPU_SHARES = 512

type BoxAuth struct {
	Username string
	Password string
}

func (a BoxAuth) IsValid() bool {
	return ((len(a.Username) == 0 && len(a.Password) == 0) ||
		(len(a.Username) > 0 && len(a.Password) > 0))
}

// Return current box info.
// StorageUsage: total storage in mb
// MemoryUsage: total memory usage in mb
// LxcStatus: It is the string that lxc used to annotate
// container's state such as RUNNING, STOPPED
type BoxInfo struct {
	StorageUsage float64 `json:"storageUsage,omitempty"`
	MemoryUsage  float64 `json:"memoryUsage,omitempty"`
	LxcStatus    string  `json:"lxcStatus,omitempty"`
}

// UniqueId is the guaranteed unique id *only* for the current box over its lifetime
// i.e. a box will be guaranteed not to have its mountpoint clash when
// umount/remount when uid is implemented
type Box struct {
	Id               int      `json:"id"`
	UniqueId         string   `json:"uniqueId"`
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
	AuthUsername     string   `json:"authUsername"`
	AuthPassword     string   `json:"authPassword"`
	Swappiness       int      `json:"swappiness"`
	CpuShares        int      `json:"cpuShares"`
	VirtualMemory    string   `json:"virtualMemory"`
	Premium          bool     `json:"premium"`
	Runtime          string   `json:"-"`
	SnapshotURL      string   `json:"-"`
	PublicKeys       []string `json:"-"`
	CpuSet           []int    // the same for all boxes for now
	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
}

// Given a []byte representation of json, convert it into a box instance
func BoxFromJSON(boxJSON []byte) (*Box, error) {
	box := &Box{}
	if err := json.Unmarshal(boxJSON, box); err != nil {
		return nil, err
	}
	box.Prepare()
	return box, nil
}

// Prepare the box with useful default value
func (b *Box) Prepare() {
	if b.Swappiness == 0 {
		b.Swappiness = DEFAULT_SWAPPINESS
	}
	if b.CpuShares == 0 {
		b.CpuShares = DEFAULT_CPU_SHARES
	}
}

// return the valid value that should be applied to `cpuset.cpus`
func (b *Box) CpuSetStr() string {
	cpus := []string{}
	for _, cpu := range b.CpuSet {
		cpus = append(cpus, strconv.Itoa(cpu))
	}
	return strings.Join(cpus, ",")
}

// Create a new box
//
// This create a new box. The chosen provisioning scheme will depend on
// `MountMethod` and `QuotaMethod`.
// This also generate an unique id to be used for the next start
func (b *Box) Create() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Create", b.ContainerName()),
	)(time.Now())
	if err := b.generateUniqueId(); err != nil {
		return err
	}
	if err := b.Mount(b.boxer.Root); err != nil {
		return err
	}
	if err := b.writeConfig(); err != nil {
		return err
	}
	if err := b.extractTemplate(); err != nil {
		return err
	}
	if err := b.setEtcAction(); err != nil {
		return err
	}
	if err := b.UpdatePremiumStatus(); err != nil {
		return err
	}
	if err := b.AddPublicKey(b.PublicKeys...); err != nil {
		return err
	}
	if err := b.setHostname(); err != nil {
		return err
	}
	if err := b.setHosts(); err != nil {
		return err
	}
	if err := b.setNetworkInterface(); err != nil {
		return err
	}
	if err := b.setWorkspace(); err != nil {
		return err
	}
	if err := b.setDefaultKeyPair(); err != nil {
		return err
	}
	if err := b.setPermission(); err != nil {
		return err
	}
	// as the initial lvm volume is created in mount step, we dont need to
	// reset quota here
	if b.getMountMethod() != "lvm" {
		if err := b.SetQuota(); err != nil {
			return err
		}
	}
	b.State = "stopped"
	return nil
}

func (b *Box) Start(spec *BoxSpec) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Start %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	if err := b.Mount(b.boxer.Root); err != nil {
		return err
	}
	if err := b.setPortForwarding(); err != nil {
		return err
	}
	// this change spec without restarting the box
	if err := b.handleSpecChange(spec, true); err != nil {
		return err
	}
	return lxc.Start(b.MountName())
}

func (b *Box) Stop() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Stop %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	if err := lxc.Stop(b.MountName()); err != nil {
		return err
	}
	return b.Unmount(b.boxer.Root)
}

func (b *Box) Restart(spec *BoxSpec) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Restart %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	if err := b.Stop(); err != nil {
		return err
	}
	return b.Start(spec)
}

func (b *Box) Destroy() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Destroy %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	if err := b.Stop(); err != nil {
		return err
	}
	if err := b.unsetPortForwarding(); err != nil {
		// ignore error from unset port forwarding here as its not harmful
		logger.Printf("%s:Destroy - unsetPortForwarding %v \n", err)
	}
	if err := b.UnsetQuota(); err != nil {
		return err
	}
	return b.DeleteContainerData()
}

func (b *Box) UpdateAuth(auth BoxAuth) error {
	b.AuthUsername = auth.Username
	b.AuthPassword = auth.Password
	if err := b.boxer.ResourcePool.SaveBox(b); err != nil {
		return err
	}
	return b.boxer.Cartman.InvalidateCache(b.ContainerName())
}

func (b *Box) MountName() string {
	base := b.ContainerName()
	if len(b.UniqueId) > 0 {
		base = base + "-" + b.UniqueId
	}
	return base
}

func (b *Box) ContainerName() string {
	return b.Name + "-" + strconv.Itoa(b.Id)
}

func (b *Box) PublicKeysPath() string {
	return path.Join(b.RootFS(), "etc", "ssh", "univ_public_keys")
}

func (b *Box) LxcPath() string {
	return path.Join(b.boxer.Root, b.MountName())
}

func (b *Box) ContainerFS() string {
	if b.getMountMethod() == "lvm" {
		return b.LxcPath()
	} else {
		return path.Join(b.Disk, b.MountName())
	}
}

func (b *Box) EnvPath() string {
	return path.Join(b.RootFS(), "etc", "environment")
}

// rootfs of the container. This is the root after unionized so its typically
// will be in boxer.Root i.e /var/lib/lxc
func (b *Box) RootFS() string {
	return path.Join(b.boxer.Root, b.MountName(), "rootfs")
}

// path of the box's templates.  (i.e. ruby_template.tar.gz,
// python_template.tar.gz etc)
func (b *Box) TemplatePath() string {
	templateName := b.Template + ".tar.gz"
	if b.getMountMethod() == "lvm" {
		return path.Join(b.boxer.Root, templateName)
	}
	return path.Join(b.Disk, templateName)
}

// we actually want the delta folders for non-lvm containers to be consistent
// because they will never be mountpoints, so we use Box#ContainerName() here
// and not Box#MountName()
func (b *Box) ContainerDeltaPath() string {
	if b.getMountMethod() == "lvm" {
		return b.ContainerFS() + "_delta"
	}
	return path.Join(b.Disk, b.ContainerName()+"_delta")
}

func (b *Box) Uid() string {
	return strconv.Itoa(UID_START + b.Id)
}

func (b *Box) Gid() string {
	return strconv.Itoa(UID_START + b.Id)
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

// memory + swap size
// if virtual memory (a.k.a swap) is specified, we will use it, otherwise,
// default is memory x 2
func (b *Box) TotalMemory() string {
	var m, vm quota.ValueUnit
	m = quota.ParseValueUnit(b.Memory)
	if len(b.VirtualMemory) > 0 {
		vm = quota.ParseValueUnit(b.VirtualMemory)
	} else {
		vm = m
	}
	return quota.SumValueUnitMb(m, vm).ToString()
}

func (b *Box) AddPublicKey(keys ...string) error {
	return utils.Appendln(b.PublicKeysPath(), strings.Join(keys, "\n"))
}

func (b *Box) RemovePublicKey(key string) error {
	return utils.DeleteLineContains(b.PublicKeysPath(), key)
}

func (b *Box) SetQuota() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetQuota", b.ContainerName()),
	)(time.Now())
	arg := b.quotaArg()
	q := b.quota()
	if b.getMountMethod() == "lvm" {
		// for lvm, we will check free space outside of the quota methods because
		// we need the volume to be mounted to check limit, and then unmounted to
		// resize
		// FIXME: we assume that the box is running at this point
		quotaInfo, err := q.ViewLimit(arg)
		if err != nil {
			return err
		}
		newLimit := quota.ConvertValueUnitToByte(quota.ParseValueUnit(arg.Limit))
		logger.Println(quotaInfo, newLimit)
		if quotaInfo.Used >= newLimit {
			return fmt.Errorf("Current usage %d exceeded new limit %d",
				quotaInfo.Used, newLimit)
		}
		if err := b.Stop(); err != nil {
			return fmt.Errorf("%s:SetQuota failed to stop %v",
				b.ContainerName(), err)
		}
	}
	b.boxer.Lock()
	defer b.boxer.Unlock()
	return q.SetLimit(arg)
}

func (b *Box) UnsetQuota() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	return b.quota().RemoveLimit(b.quotaArg())
}

func (b *Box) ChangeSpec(spec *BoxSpec) error {
	if err := b.handleSpecChange(spec, false); err != nil {
		return err
	}
	return b.Restart(spec)
}

func (b *Box) CurrentSpec() *BoxSpec {
	return &BoxSpec{
		Memory:        b.Memory,
		CpuShares:     b.CpuShares,
		VirtualMemory: b.VirtualMemory,
		DiskSpace:     b.DiskSpace,
	}
}

func (b *Box) UpdatePremiumStatus() error {
	actionPath := path.Join(b.RootFS(), "etc", "action")
	premiumPath := path.Join(actionPath, "premium")
	if b.Premium {
		if err := os.MkdirAll(actionPath, 0755); err != nil {
			return err
		}
		premiumFile, err := os.Create(premiumPath)
		if err != nil {
			return err
		}
		defer premiumFile.Close()
		_, err = premiumFile.WriteString("1")
		return err
	} else {
		return os.RemoveAll(premiumPath)
	}
}

func (b *Box) Mount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:Mount", b.ContainerName()),
	)(time.Now())

	lxcBoxPath := b.LxcPath()
	boxDeltaPath := b.ContainerDeltaPath()
	mountMethod := b.getMountMethod()
	switch mountMethod {
	case "overlayfs", "aufs":
		// overlayfs or aufs strategy
		//
		// we are going to create a delta folder which will hold user data,
		// union with the base container template in the same disk, and mount
		// it over to root ( /var/lib/lxc )
		boxPath := b.ContainerFS()
		baseContainerPath := path.Join(b.Disk, "base_container_template")
		if err := mount.Union(b.getMountMethod(), boxPath, boxDeltaPath, baseContainerPath); err != nil {
			return fmt.Errorf("box:Mount failed %v", err)
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
		if err := lvm.CreateVolume(b.SelectedVG, THIN_VG, b.DiskSpace,
			b.ContainerName()); err != nil {
			return err
		}
		deviceName := fmt.Sprintf("/dev/%s/%s", b.SelectedVG, b.ContainerName())
		device := fs.Ext4Device{Name: deviceName, BytesPerInode: "8192"}
		if err := fs.MkfsExt4(device); err != nil {
			return err
		}
		baseContainerPath := path.Join(root, "base_container_template")
		err := mount.Mount(deviceName, boxDeltaPath, "ext4")
		if err != nil {
			return err
		}
		return mount.Union("aufs", lxcBoxPath, boxDeltaPath, baseContainerPath)
	default:
		return fmt.Errorf("%s is not a supported mount method", mountMethod)
	}
}

// Unmount will always regenerate an unique id for the box, so it guarantees
// that the next mount operation will success even if `Unmount` fails
func (b *Box) Unmount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:Unmount", b.ContainerName()),
	)(time.Now())
	// always re-generate a new unique id when unmount the path
	defer b.generateUniqueId()
	mountMethod := b.getMountMethod()
	if err := mount.Unmount(mountMethod, b.LxcPath()); err != nil {
		logger.Println("Unmount: err %v", err)
	}
	switch mountMethod {
	case "overlayfs", "aufs":
		if err := mount.Unmount(mountMethod, b.ContainerFS()); err != nil {
			logger.Println("Unmount: err %v", err)
		}
	case "lvm":
		if err := mount.Unmount(mountMethod, b.ContainerDeltaPath()); err != nil {
			logger.Println("Unmount: err %v", err)
		}
	default:
		return fmt.Errorf("%s is not a supported unmount method",
			mountMethod)
	}
	return nil
}

func (b *Box) DeleteContainerData() error {
	if b.getMountMethod() == "lvm" {
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
	name := b.MountName()
	filename := fmt.Sprintf("/sys/fs/cgroup/memory/lxc/%s/memory.usage_in_bytes",
		name)
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			alternate := fmt.Sprintf(
				"/sys/fs/cgroup/memory/sysdefault/lxc/%s/memory.usage_in_bytes",
				name)
			content, err := ioutil.ReadFile(alternate)
			if err == nil {
				return strconv.Atoi(string(content))
			}
		}
		logger.Printf("%s: Failed to read memory usage, err: %v", name, err)
		return 0, err
	}
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		logger.Printf("%s: Failed to read memory usage, err: %v\n", name, err)
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(content)))
}

func (b *Box) Info() (BoxInfo, error) {
	var info BoxInfo
	if memoryUsage, err := b.MemoryUsageInBytes(); err != nil {
		return info, err
	} else {
		info.MemoryUsage = float64(memoryUsage) / (1024 * 1024)
	}
	if state, err := lxc.Status(b.MountName()); err != nil {
		return info, err
	} else {
		info.LxcStatus = state
	}
	quotaInfo, err := b.quota().ViewLimit(b.quotaArg())
	if err != nil {
		return info, err
	} else {
		info.StorageUsage = quota.ScaleUnit(quotaInfo.Used, "M")
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
		if err := utils.DeleteLineContains(b.EnvPath(), key+"="); err != nil {
			return err
		}
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", key, value.(string)))
	}
	return utils.Appendln(b.EnvPath(), strings.Join(newEnv, "\n"))
}

// Remove env variables from the box
func (b *Box) RemoveEnv(data []string) (err error) {
	for _, value := range data {
		if err = utils.DeleteLineContains(b.EnvPath(), value+"="); err != nil {
			return err
		}
	}
	return
}

// this requires the box to be mounted because we are writing lxc config into
// the mounted path, not the _delta path
func (b *Box) handleSpecChange(spec *BoxSpec, forceWriteConfig bool) error {
	willWriteConfig := false
	if spec.Memory != b.Memory {
		b.Memory = spec.Memory
		willWriteConfig = true
	}
	if spec.CpuShares != b.CpuShares {
		b.CpuShares = spec.CpuShares
		willWriteConfig = true
	}
	if spec.VirtualMemory != b.VirtualMemory {
		b.VirtualMemory = spec.VirtualMemory
		willWriteConfig = true
	}
	if willWriteConfig || forceWriteConfig {
		if err := b.writeConfig(); err != nil {
			return err
		}
	}
	if len(spec.DiskSpace) > 0 && spec.DiskSpace != b.DiskSpace {
		oldDiskSpace := b.DiskSpace
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(); err != nil {
			b.DiskSpace = oldDiskSpace
			return err
		}
	}
	return nil
}

func (b *Box) quotaArg() quota.QuotaArg {
	arg := quota.QuotaArg{
		Id:    strconv.Itoa(b.Id),
		Name:  b.ContainerName(),
		Disk:  b.Disk,
		Limit: b.DiskSpace,
	}
	if b.getQuotaMethod() == "lvm" {
		arg.Path = b.ContainerDeltaPath()
	} else {
		arg.Path = path.Join(b.ContainerDeltaPath(), "rootfs", "home", "action")
	}
	return arg
}

func (b *Box) getMountMethod() string {
	if b.MountMethod == "lvm" {
		return "lvm"
	} else if b.MountMethod == "aufs" || b.Aufs {
		return "aufs"
	} else if b.MountMethod == "failed" {
		return "failed"
	}
	return "overlayfs"
}

func (b *Box) getQuotaMethod() string {
	if b.getMountMethod() == "lvm" {
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
	return quota.Provisioner.GetQuota(b.getQuotaMethod())
}

// rewrite lxc config of the box
func (b *Box) writeConfig() error {
	// we want to re-check the box values here to make sure that only valid lxc
	// config value will be saved
	b.Prepare()
	lxcConfigPath := path.Join(b.LxcPath(), "config")
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
	if err := utils.RunCommand("chroot", root, "usermod", "-u", uid, "action"); err != nil {
		return err
	}
	if err := utils.RunCommand("chroot", root, "groupmod", "-g", gid, "action"); err != nil {
		return err
	}
	return utils.RunCommand("chroot", root, "chown", "-R", permission, "/home/action")
}

func (b *Box) setPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	if b.boxer.Vagrant {
		return ip.OpenInVagrant(strPort, b.Ip, "22")
	} else {
		return ip.Open(strPort, b.Ip, "22")
	}
}

func (b *Box) unsetPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	if b.boxer.Vagrant {
		return ip.CloseInVagrant(strPort, b.Ip, "22")
	} else {
		return ip.Close(strPort, b.Ip, "22")
	}
}

func (b *Box) extractTemplate() error {
	boxHomePath := path.Join(b.RootFS(), "home", "action")
	return Untar(b.TemplatePath(), boxHomePath)
}

func (b *Box) setEtcAction() error {
	actionPath := path.Join(b.RootFS(), "etc", "action")
	if err := os.MkdirAll(actionPath, 0755); err != nil {
		return err
	}
	// set box_id
	boxIdFile, err := os.Create(path.Join(actionPath, "box_id"))
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	if _, err = boxIdFile.WriteString(strconv.Itoa(b.Id)); err != nil {
		return err
	}
	// set box_user
	boxUserFile, err := os.Create(path.Join(actionPath, "box_user"))
	if err != nil {
		return err
	}
	defer boxUserFile.Close()
	_, err = boxUserFile.WriteString("action")
	return err
}

func (b *Box) setHostname() error {
	hostNamePath := path.Join(b.RootFS(), "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.WriteString(b.ContainerName())
	return err
}

func (b *Box) setHosts() error {
	hostsPath := path.Join(b.RootFS(), "etc", "hosts")
	hosts, err := os.Create(hostsPath)
	if err != nil {
		return err
	}
	defer hosts.Close()
	return HostsTemplateCompiled.Execute(hosts, b)
}

func (b *Box) setNetworkInterface() error {
	networkConfigPath := path.Join(b.RootFS(), "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, b)
}

func (b *Box) setWorkspace() error {
	workspacePath := path.Join(b.RootFS(), "home", "action", "workspace")
	return os.MkdirAll(workspacePath, 0755)
}

func (b *Box) setDefaultKeyPair() error {
	sshPath := path.Join(b.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(sshPath, "id_rsa")
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		if err := utils.RunCommand("/bin/sh", "-c", sshKeygen); err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

func (b *Box) generateUniqueId() error {
	b.UniqueId = strconv.FormatInt(time.Now().Unix(), 10)
	return b.boxer.ResourcePool.SaveBox(b)
}
