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
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/utils"

	pr "github.com/action-io/goboxer/provisioner"
	_ "github.com/action-io/goboxer/provisioner/lxc"
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
	CustomDomain     string   `json:"customDomain"`
	CpuSet           []int
	CpuQuota         float64
	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
	// reference to the provisioner
	provisioner pr.Provisioner
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

func (b *Box) getProvisioner() pr.Provisioner {
	if b.provisioner == nil {
		b.provisioner, _ = pr.GetProvisioner("lxc", b.Disk)
	}
	return b.provisioner
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
	var data *pr.Data
	var err error

	if b.willUseTemplate() {
		data, err = b.getProvisioner().Create(b.ContainerName(), b.Template)
		if err != nil {
			return &BoxOpError{Op: "create box from template", Err: err}
		}
	} else {
		data, err = b.getProvisioner().CreateFromSnapshot(b.ContainerName(),
			b.Template, b.boxer.S3Auth)
		if err != nil {
			return &BoxOpError{Op: "create box from snapshot", Err: err}
		}
	}
	b.UniqueId = data.Id
	b.State = data.Status
	if b.willUseTemplate() {
		if err := b.setWorkspace(data.RootPath); err != nil {
			return &BoxOpError{Op: "set workspace", Err: err}
		}
		if err := b.addReadme(data.RootPath); err != nil {
			return &BoxOpError{Op: "add readme", Err: err}
		}
	}
	if err := b.writeConfig(); err != nil {
		return &BoxOpError{Op: "write lxc config", Err: err}
	}
	if err := b.setEtcAction(data.RootPath); err != nil {
		return &BoxOpError{Op: "create /etc/action", Err: err}
	}
	if err := b.UpdatePremiumStatus(); err != nil {
		return err
	}
	if err := b.setHostname(data.RootPath); err != nil {
		return &BoxOpError{Op: "set host name", Err: err}
	}
	if err := b.setHosts(data.RootPath); err != nil {
		return &BoxOpError{Op: "set /etc/hosts", Err: err}
	}
	if err := b.setNetworkInterface(data.RootPath); err != nil {
		return &BoxOpError{Op: "set network interface", Err: err}
	}
	if err := b.setDefaultKeyPair(data.RootPath); err != nil {
		return &BoxOpError{Op: "set default key pair", Err: err}
	}
	if err := b.setPermission(data.RootPath); err != nil {
		return &BoxOpError{Op: "set permission", Err: err}
	}
	if err := b.SetQuota(); err != nil {
		return err
	}
	return nil
}

func (b *Box) Start(spec *BoxSpec) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Start %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	beforeStart := func(data *pr.Data) error {
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
		if err := b.addPartsInitUpstart(); err != nil {
			return &BoxOpError{Op: "parts init upstart", Err: err}
		}
		if err := b.addNitrousboxrcUpstart(); err != nil {
			return &BoxOpError{Op: "nitrousboxrc upstart", Err: err}
		}
		if err := b.addNitrousboxrcSample(); err != nil {
			return &BoxOpError{Op: "nitrousboxrc sample", Err: err}
		}
		return nil
	}
	data, err := b.getProvisioner().Start(b.ContainerName(), b.UniqueId,
		beforeStart)
	if err != nil {
		return err
	}
	b.State = data.Status
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
	data, err := b.getProvisioner().Stop(b.ContainerName(), b.UniqueId, b.Uid())
	if err != nil {
		return err
	}
	b.UniqueId = data.Id
	b.State = data.Status
	return nil
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
	return b.getProvisioner().Destroy(b.ContainerName(), b.UniqueId)
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
	tarPath, err := Tar(tarName, homePath(b.RootFS()))
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
	return b.save()
}

func (b *Box) UpdateCustomDomain(customDomain string) error {
	if err := b.resourcePool().RemoveDomain(b.CustomDomain); err != nil {
		return err
	}
	b.CustomDomain = customDomain
	if err := b.save(); err != nil {
		return err
	}
	if len(b.CustomDomain) > 0 {
		return b.resourcePool().SetDomain(customDomain, b.ContainerName())
	}
	return nil
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
	return b.getProvisioner().GetLxc(b.ContainerName(), b.UniqueId)
}

func (b *Box) EnvPath() string {
	return path.Join(b.RootFS(), "etc", "environment")
}

// rootfs of the container. This is the root after unionized so its typically
// will be in boxer.Root i.e /var/lib/lxc
func (b *Box) RootFS() string {
	return b.getProvisioner().GetRoot(b.ContainerName(), b.UniqueId)
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
	if err := b.save(); err != nil {
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
	if err := b.save(); err != nil {
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
	if err := b.save(); err != nil {
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

func (b *Box) lxcMounted() (bool, error) {
	return b.getProvisioner().IsMounted(b.ContainerName(), b.UniqueId)
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
	state, err := lxc.Status(b.MountName())
	if err == nil {
		info.LxcStatus = state
	} else {
		if b.State == "stopped" {
			// Stopped boxes will fail lxc.Status because they are unmounted so
			// lxc-info will not find their config file and consider them
			// non-existent.  Thus we swallow errors for stopped boxes
			info.LxcStatus = "STOPPED"
		} else {
			return info, err
		}
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
	return b.getProvisioner().SetConfig(b.ContainerName(), b.UniqueId,
		func(config *os.File) error {
			return LxcTemplateCompiled.Execute(config, b)
		})
}

// set correct permission for user's home folder so he/she can write onto it
func (b *Box) setPermission(root string) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetPermission", b.ContainerName()),
	)(time.Now())
	uid := b.Uid()
	gid := b.Gid()
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
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

func (b *Box) setEtcAction(root string) error {
	actionPath := path.Join(root, "etc", "action")
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

func (b *Box) setHostname(root string) error {
	hostNamePath := path.Join(root, "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.WriteString(b.ContainerName())
	return err
}

func (b *Box) setHosts(root string) error {
	hostsPath := path.Join(root, "etc", "hosts")
	hosts, err := os.Create(hostsPath)
	if err != nil {
		return err
	}
	defer hosts.Close()
	return HostsTemplateCompiled.Execute(hosts, b)
}

func (b *Box) setNetworkInterface(root string) error {
	networkConfigPath := path.Join(root, "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, b)
}

func (b *Box) setWorkspace(root string) error {
	workspacePath := path.Join(root, "home", "action", "workspace")
	return os.MkdirAll(workspacePath, 0755)
}

func (b *Box) addReadme(root string) error {
	readmePath := path.Join(root, "home", "action", "README.md")
	readme, err := os.Create(readmePath)
	if err != nil {
		return err
	}
	defer readme.Close()
	return RuntimeTemplates.Get(b.Runtime).Execute(readme, b)
}

func (b *Box) setDefaultKeyPair(root string) error {
	sshPath := path.Join(root, "home", "action", ".ssh")
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

func (b *Box) addPartsInitUpstart() error {
	confPath := path.Join(b.RootFS(), "etc", "init", "parts-init.conf")
	confFile, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer confFile.Close()
	return PartsInitConfTemplateCompiled.Execute(confFile, UpstartConfSettings)
}

func (b *Box) addNitrousboxrcUpstart() error {
	confPath := path.Join(b.RootFS(), "etc", "init", "run-nitrousboxrc.conf")
	confFile, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer confFile.Close()
	return RunNitrousboxrcConfTemplateCompiled.Execute(confFile, UpstartConfSettings)
}

func (b *Box) addNitrousboxrcSample() error {
	samplePath := path.Join("home", UpstartConfSettings["ActionUser"], UpstartConfSettings["NitrousboxrcSample"])
	sampleFile, err := os.Create(path.Join(b.RootFS(), samplePath))
	if err != nil {
		return err
	}
	err = NitrousboxrcSampleTemplateCompiled.Execute(sampleFile, UpstartConfSettings)
	if err != nil {
		sampleFile.Close()
		return err
	}
	sampleFile.Close()
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	return utils.RunCommand("chroot", b.RootFS(), "chown", "-R", permission, samplePath)
}

func (b *Box) willUseTemplate() bool {
	return len(b.SnapshotUrl) == 0
}

func homePath(root string) string {
	return path.Join(root, "home", "action")
}

// wraps around boxer and resourcePool so we have a single place to trigger
// callbacks after save
func (b *Box) save() error {
	if err := b.resourcePool().SaveBox(b); err != nil {
		return err
	}
	return b.boxer.Cartman.InvalidateCache(b.ContainerName())
}

func (b *Box) resourcePool() *ResourcePool {
	return b.boxer.ResourcePool
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
