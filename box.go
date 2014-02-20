package goboxer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/action-io/goboxer/ip"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/s3"
	"github.com/action-io/goboxer/utils"
)

const UID_START = 15000
const THIN_VG = "boxes"
const DEFAULT_SWAPPINESS = 60
const DEFAULT_CPU_SHARES = 512
const DEFAULT_CPU_PERIOD = 1000000 // cpu.cfs_period_us: 1 million microseconds

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

// MemorySpecChange contains old and new ValueUnit strings for memory and total
// memory
type MemorySpecChange struct {
	OldMemory      string
	OldTotalMemory string
	NewMemory      string
	NewTotalMemory string
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
	SnapshotId       int      `json:"-"`
	SnapshotUrl      string   `json:"-"`
	PublicKeys       []string `json:"publicKeys"`
	CpuSet           []int
	CpuQuota         float64
	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
}

type BoxOpError struct {
	Op  string // Box operation.
	Err error
}

func (e *BoxOpError) Error() string {
	return "box " + e.Op + ": " + e.Err.Error()
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

// Returns cpu.cfs_period_us setting for this box.
func (b *Box) CpuPeriodMicroseconds() int {
	return DEFAULT_CPU_PERIOD
}

// Returns cpu.cfs_quota_us setting for this box.
func (b *Box) CpuQuotaMicroseconds() int {
	return int(b.CpuQuota * DEFAULT_CPU_PERIOD)
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
		return &BoxOpError{Op: "generate unique ID", Err: err}
	}
	if err := b.Mount(b.boxer.Root); err != nil {
		return &BoxOpError{Op: "mount box", Err: err}
	}
	if err := b.generateUserContent(); err != nil {
		return &BoxOpError{Op: "generate user content", Err: err}
	}
	if err := b.writeConfig(); err != nil {
		return &BoxOpError{Op: "write lxc config", Err: err}
	}
	if err := b.setEtcAction(); err != nil {
		return &BoxOpError{Op: "create /etc/action", Err: err}
	}
	if err := b.UpdatePremiumStatus(); err != nil {
		return err
	}
	if err := b.setHostname(); err != nil {
		return &BoxOpError{Op: "set host name", Err: err}
	}
	if err := b.setHosts(); err != nil {
		return &BoxOpError{Op: "set /etc/hosts", Err: err}
	}
	if err := b.setNetworkInterface(); err != nil {
		return &BoxOpError{Op: "set network interface", Err: err}
	}
	if err := b.setDefaultKeyPair(); err != nil {
		return &BoxOpError{Op: "set default key pair", Err: err}
	}
	if err := b.setPermission(); err != nil {
		return &BoxOpError{Op: "set permission", Err: err}
	}
	if err := b.SetQuota(); err != nil {
		return err
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
	if err := b.WritePublicKeys(); err != nil {
		return &BoxOpError{Op: "write public keys", Err: err}
	}
	if err := b.setPortForwarding(); err != nil {
		return &BoxOpError{Op: "set port forwarding", Err: err}
	}
	// this change spec without restarting the box
	if err := b.handleSpecChange(spec, true); err != nil {
		return &BoxOpError{Op: "spec change", Err: err}
	}
	err := lxc.Start(b.ContainerName(), b.MountName())
	if err != nil {
		return &BoxOpError{Op: "lxc-start " + b.MountName(), Err: err}
	}
	return nil
}

// implementation of lxc-shutdown
//
// - then  we try to poweroff the container
// - if that cant stop the container, force-kill all current user
// processes as sometimes defunct processes can stop the container from
// shutting down, then stop the container again
func (b *Box) Stop() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Stop %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	if err := b.unsetPortForwarding(); err != nil {
		// ignore error from unset port forwarding here as its not harmful
		logger.Printf("%v", &BoxOpError{Op: "unset port forwarding", Err: err})
	}
	if lxc.Poweroff(b.MountName()) != nil {
		utils.RunCommand("pkill", "-9", "-u", b.Uid())
		if err := lxc.Stop(b.ContainerName(), b.MountName()); err != nil {
			return &BoxOpError{Op: "lxc-stop " + b.MountName(), Err: err}
		}
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
	if err := b.UnsetQuota(); err != nil {
		return &BoxOpError{Op: "unset quota", Err: err}
	}
	return b.DeleteContainerData()
}

func (b *Box) Snapshot(snapshotId int) (snapshot *Snapshot, err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Snapshot %s", b.ContainerName(), b.MountName()),
	)(time.Now())

	quotaInfo, err := b.quota().ViewLimit(b.quotaArg())
	if err != nil {
		err = fmt.Errorf("Snapshot: failed to get uncompressed snapshot size: %v", err)
		return
	}
	uncompressedSize := quota.ValueUnit{Value: float64(quotaInfo.Used), Unit: "K"}

	tarName := fmt.Sprintf("%s-snapshot-%d.tar.gz", b.ContainerName(), snapshotId)
	logger.Printf("%s: creating snapshot tarball %s\n", b.ContainerName(), tarName)
	tarPath, err := Tar(tarName, b.ContainerUserHomePath())
	if err != nil {
		err = fmt.Errorf("Snapshot: failed to compress delta %v", err)
		return
	}
	// delete the tar path after finishing the upload process success or not
	defer os.RemoveAll(tarPath)
	// upload tarPath to s3
	boxer := b.boxer
	client, err := boxer.S3Auth.NewClient(boxer.S3Region, boxer.S3Bucket)
	if err != nil {
		err = fmt.Errorf("Snapshot: failed to create s3 client %v", err)
		return
	}
	snapshotUrl, err := client.Upload(tarName, tarPath)
	if err != nil {
		err = fmt.Errorf("Snapshot: failed to upload to s3 %v", err)
		return
	}
	snapshot = &Snapshot{Url: snapshotUrl, Size: uncompressedSize}
	return
}

func (b *Box) CgroupSet(subsystem, value string) error {
	return lxc.CgroupSet(b.MountName(), subsystem, value)
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
	return path.Join(b.Disk, b.MountName())
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
	return path.Join(b.boxer.Root, templateName)
}

// we actually want the delta folders for non-lvm containers to be consistent
// because they will never be mountpoints, so we use Box#ContainerName() here
// and not Box#MountName()
func (b *Box) ContainerDeltaPath() string {
	return path.Join(b.Disk, b.ContainerName()+"_delta")
}

// ContainerUserHomePath returns the full path to the user's home directory.
func (b *Box) ContainerUserHomePath() string {
	return path.Join(b.ContainerDeltaPath(), "rootfs", "home", "action")
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

// Returns the union of a box's keys from Redis and from its root filesystem.
func (b *Box) GetPublicKeys() []string {
	// Merge box's public keys from Redis record and from
	// rootfs/etc/ssh/univ_public_keys (removing any duplicates).
	return utils.UnionSlices(b.publicKeysFromFile(), b.PublicKeys)
}

// Sets a box's public keys to the given keys, writes those keys into its root
// filesystem and to Redis.
func (b *Box) SetPublicKeys(keys []string) error {
	b.PublicKeys = keys

	// Write to Redis.
	if err := b.boxer.ResourcePool.SaveBox(b); err != nil {
		return err
	}

	// Write to public key file.
	return b.writePublicKeysFile(b.PublicKeys)
}

// Unions a box's keys from Redis and from its root filesystem, and writes
// those keys into its root filesystem and to Redis.
func (b *Box) WritePublicKeys() error {
	return b.SetPublicKeys(b.GetPublicKeys())
}

func (b *Box) writePublicKeysFile(keys []string) error {
	f, err := os.OpenFile(b.PublicKeysPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(strings.Join(keys, "\n"))
	return err
}

// Like writePublicKeysFile(), except nothing is done if the public keys file
// doesn't already exist.
func (b *Box) writePublicKeysFileIfExists(keys []string) error {
	if _, err := os.Stat(b.PublicKeysPath()); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return b.writePublicKeysFile(keys)
}

// Reads public keys of a box from the public key file in its rootfs.
// If the file doesn't exist or if there's any errors reading the file, an
// empty slice is returned.
func (b *Box) publicKeysFromFile() []string {
	keys := make([]string, 0)
	filepath := b.PublicKeysPath()
	if _, err := os.Stat(filepath); err != nil {
		if os.IsNotExist(err) {
			return keys
		}
		return keys
	}
	content, err := ioutil.ReadFile(filepath)
	if err != nil {
		return keys
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, line := range lines {
		if line != "" {
			keys = append(keys, line)
		}
	}
	return keys
}

// Adds a public key to a box only if the key doesn't already exist. The box's
// record is updated with the new key, and an attempt is made to write the new
// key into the box if its root filesystem is mounted.
func (b *Box) AddPublicKey(key string) error {
	key = strings.TrimSpace(key)
	for _, elem := range b.GetPublicKeys() {
		if elem == key {
			return nil
		}
	}
	b.PublicKeys = append(b.GetPublicKeys(), key)
	if err := b.boxer.ResourcePool.SaveBox(b); err != nil {
		return err
	}
	return b.writePublicKeysFileIfExists(b.PublicKeys)
}

// Remove a public key from a box's record in Redis, and attempt to write the
// new set of public keys into the box if its root filesystem is mounted.
func (b *Box) RemovePublicKey(key string) error {
	key = strings.TrimSpace(key)
	var newKeys []string
	for _, elem := range b.GetPublicKeys() {
		elem = strings.TrimSpace(elem)
		if elem != key {
			newKeys = append(newKeys, elem)
		}
	}
	b.PublicKeys = newKeys
	if err := b.boxer.ResourcePool.SaveBox(b); err != nil {
		return err
	}
	return b.writePublicKeysFileIfExists(b.PublicKeys)
}

func (b *Box) SetQuota() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetQuota", b.ContainerName()),
	)(time.Now())
	arg := b.quotaArg()
	q := b.quota()
	b.boxer.Lock(b.Id)
	defer b.boxer.Unlock(b.Id)
	return q.SetLimit(arg)
}

func (b *Box) UnsetQuota() error {
	b.boxer.Lock(b.Id)
	defer b.boxer.Unlock(b.Id)
	return b.quota().RemoveLimit(b.quotaArg())
}

func (b *Box) ChangeSpec(spec *BoxSpec) error {
	return b.handleSpecChange(spec, false)
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
	b.boxer.Lock(b.Id)
	defer b.boxer.Unlock(b.Id)
	defer logger.LogDuration(
		fmt.Sprintf("%s:Mount", b.ContainerName()),
	)(time.Now())

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
			return &BoxOpError{Op: "overlayfs/aufs mount", Err: err}
		}
		// FIXME: MountBind to make it similar to boxer. Ideally we probably
		// want to symlink as it doesnt make any difference and seems to be
		// more stable. Less mountpoint is better
		err := mount.MountBind(boxPath, b.LxcPath())
		if err != nil {
			return &BoxOpError{Op: "overlayfs/aufs mount-bind", Err: err}
		}
		return err
	default:
		return fmt.Errorf("box: %s is not a supported mount method", mountMethod)
	}
}

// Unmount will always regenerate an unique id for the box, so it guarantees
// that the next mount operation will success even if `Unmount` fails
func (b *Box) Unmount(root string) error {
	b.boxer.Lock(b.Id)
	defer b.boxer.Unlock(b.Id)
	defer logger.LogDuration(
		fmt.Sprintf("%s:Unmount", b.ContainerName()),
	)(time.Now())
	// always re-generate a new unique id when unmount the path
	defer b.generateUniqueId()
	mountMethod := b.getMountMethod()
	if err := mount.Unmount(mountMethod, b.LxcPath()); err != nil {
		logger.Println(&BoxOpError{Op: mountMethod + " unmount", Err: err})
	}
	switch mountMethod {
	case "overlayfs", "aufs":
		if err := mount.Unmount(mountMethod, b.ContainerFS()); err != nil {
			logger.Println(&BoxOpError{Op: mountMethod + " unmount", Err: err})
		}
	default:
		return fmt.Errorf("%s is not a supported unmount method", mountMethod)
	}
	return nil
}

func (b *Box) lxcMounted() (bool, error) {
	return mount.Mounted(b.LxcPath())
}

func (b *Box) DeleteContainerData() error {
	// if golang failed to remove container, drops into cmdline as it seems to
	// work all the times. We just prefer to do it in golang first.
	if err := os.RemoveAll(b.ContainerDeltaPath()); err != nil {
		err = utils.RunCommand("rm", "-r", "-f", b.ContainerDeltaPath())
		if err != nil {
			return &BoxOpError{Op: "remove container delta", Err: err}
		}
	}
	err := os.RemoveAll(b.ContainerFS())
	if err != nil {
		return &BoxOpError{Op: "remove container FS", Err: err}
	}
	return nil
}

// Return current memory usage of the box as memory cgroup reported it
// the unit if B, so if we want to convert to MB, it needs to be divided with
// 1024 * 1024
func (b *Box) MemoryUsageInBytes() (int, error) {
	name := b.MountName()
	output, err := exec.Command("lxc-cgroup", "-n", name,
		"memory.usage_in_bytes").CombinedOutput()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(output)))
}

func (b *Box) Info() (BoxInfo, error) {
	const MemoryUsageNotApplicable = -1.0
	var info BoxInfo
	if b.State == "running" {
		if memoryUsage, err := b.MemoryUsageInBytes(); err != nil {
			return info, err
		} else {
			info.MemoryUsage = float64(memoryUsage) / (1024 * 1024)
		}
	} else {
		info.MemoryUsage = MemoryUsageNotApplicable
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
		info.StorageUsage = float64(quota.ScaleUnitCeil(quotaInfo.Used, "M"))
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
	msc := &MemorySpecChange{
		OldMemory:      b.Memory,
		OldTotalMemory: b.TotalMemory(),
	}
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
		if err := b.boxer.ResourcePool.SaveBox(b); err != nil {
			return &BoxOpError{Op: "save", Err: err}
		}
		mounted, err := b.lxcMounted()
		if err != nil {
			return &BoxOpError{Op: "write lxc config", Err: err}
		}
		if mounted {
			// if not mounted (stopped state), the config will be written upon
			// next Start()
			err = b.writeConfig()
			if err != nil {
				return &BoxOpError{Op: "write lxc config", Err: err}
			}
		}
	}
	if len(spec.DiskSpace) > 0 && spec.DiskSpace != b.DiskSpace {
		oldDiskSpace := b.DiskSpace
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(); err != nil {
			b.DiskSpace = oldDiskSpace
			return &BoxOpError{Op: "set disk quota", Err: err}
		}
	}
	// apply new memory quota immediately for running boxes
	if b.State == "running" && (b.Memory != msc.OldMemory || b.TotalMemory() != msc.OldTotalMemory) {
		msc.NewMemory = b.Memory
		msc.NewTotalMemory = b.TotalMemory()
		if err := b.liveUpdateMemory(msc); err != nil {
			return &BoxOpError{Op: "live adjust memory quota", Err: err}
		}
	}
	return nil
}

func (b *Box) liveUpdateMemory(msc *MemorySpecChange) error {
	order := msc.settingOrder()
	for _, kvPair := range order {
		k, v := kvPair[0], kvPair[1]
		if err := b.CgroupSet(k, v); err != nil {
			return err
		}
	}
	return nil
}

// memorySettingOrder returns a slice of arguments to lxc-cgroup (slice of
// strings) signifying what order to change the settings in.   lxc-cgroup
// enforces invariant:
//   memory.limit_in_bytes < memory.memsw.limit_in_bytes
// so we need to order operations accordingly
func (msc *MemorySpecChange) settingOrder() [][]string {
	var (
		memKeyValue      = []string{"memory.limit_in_bytes", msc.NewMemory}
		totalMemKeyValue = []string{"memory.memsw.limit_in_bytes", msc.NewTotalMemory}
	)
	stringToByte := func(s string) int {
		return quota.ConvertValueUnitToByte(quota.ParseValueUnit(s))
	}
	if stringToByte(msc.OldMemory) > stringToByte(msc.NewTotalMemory) {
		return [][]string{memKeyValue, totalMemKeyValue}
	}
	return [][]string{totalMemKeyValue, memKeyValue}
}

func (b *Box) quotaArg() quota.QuotaArg {
	arg := quota.QuotaArg{
		Id:    strconv.Itoa(b.Id),
		Name:  b.ContainerName(),
		Disk:  b.Disk,
		Limit: b.DiskSpace,
	}
	arg.Path = b.ContainerUserHomePath()
	return arg
}

func (b *Box) getMountMethod() string {
	if b.MountMethod == "aufs" || b.Aufs {
		return "aufs"
	} else if b.MountMethod == "failed" {
		return "failed"
	}
	return "overlayfs"
}

func (b *Box) getQuotaMethod() string {
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
	b.boxer.Lock(b.Id)
	defer b.boxer.Unlock(b.Id)
	strPort := strconv.Itoa(b.Port)
	if b.boxer.Vagrant {
		return ip.OpenInVagrant(strPort, b.Ip, "22")
	} else {
		return ip.Open(strPort, b.Ip, "22")
	}
}

func (b *Box) unsetPortForwarding() error {
	b.boxer.Lock(b.Id)
	defer b.boxer.Unlock(b.Id)
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

func (b *Box) addReadme() error {
	readmePath := path.Join(b.RootFS(), "home", "action", "README.md")
	readme, err := os.Create(readmePath)
	if err != nil {
		return err
	}
	defer readme.Close()
	return RuntimeTemplates.Get(b.Runtime).Execute(readme, b)
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
	b.UniqueId = strconv.FormatInt(time.Now().UnixNano(), 10)
	return b.boxer.ResourcePool.SaveBox(b)
}

// this is used in Create() to generate the user content for the box. User
// content can come from a snapshot on s3, or from predefined template
func (b *Box) generateUserContent() error {
	if len(b.SnapshotUrl) > 0 {
		// parse snapshot url which should be an s3 path
		url, err := s3.ParseUrl(b.SnapshotUrl)
		if err != nil {
			return &BoxOpError{Op: "parse s3 url", Err: err}
		}
		client, err := b.boxer.S3Auth.NewClient(url.Region, url.Bucket)
		if err != nil {
			return &BoxOpError{Op: "create s3 client", Err: err}
		}
		stream, err := client.Download(url.File)
		if err != nil {
			return &BoxOpError{Op: "download s3 stream", Err: err}
		}
		if err := os.MkdirAll(b.ContainerUserHomePath(), 0755); err != nil {
			return &BoxOpError{Op: "creating user home directory", Err: err}
		}
		if err := UntarStream(stream, b.ContainerUserHomePath()); err != nil {
			return &BoxOpError{Op: "untar s3 stream", Err: err}
		}
	} else {
		if err := b.extractTemplate(); err != nil {
			return &BoxOpError{Op: "extract template", Err: err}
		}
		if err := b.setWorkspace(); err != nil {
			return &BoxOpError{Op: "set workspace", Err: err}
		}
		if err := b.addReadme(); err != nil {
			return &BoxOpError{Op: "add readme", Err: err}
		}
	}
	return nil
}

func (b *Box) String() string {
	return fmt.Sprintf("&{Id:%v Name:%v UniqueId:%v State:%v Ip:%v Port:%v Disk:%v MountMethod:%v BoxerId:%v Premium:%v}",
		b.Id,
		b.Name,
		b.UniqueId,
		b.State,
		b.Ip,
		b.Port,
		b.Disk,
		b.MountMethod,
		b.BoxerId,
		b.Premium,
	)
}
