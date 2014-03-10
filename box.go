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

	"github.com/action-io/goboxer/archive"
	"github.com/action-io/goboxer/ip"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/utils"

	pr "github.com/action-io/goboxer/provisioner"
	_ "github.com/action-io/goboxer/provisioner/lxc"
)

const UID_START = 15000
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
	return box, nil
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

func (b *Box) provisionerConfig() pr.Config {
	if b.Swappiness == 0 {
		b.Swappiness = DEFAULT_SWAPPINESS
	}
	if b.CpuShares == 0 {
		b.CpuShares = DEFAULT_CPU_SHARES
	}

	return pr.Config{
		Id:                    b.UniqueId,
		ContainerName:         b.ContainerName(),
		ContainerId:           b.Id,
		Uid:                   b.Uid(),
		Template:              b.Template,
		Ip:                    b.Ip,
		Gateway:               b.Gateway,
		NetworkInterface:      b.NetworkInterface,
		MacAddress:            b.MacAddress(),
		Memory:                b.Memory,
		TotalMemory:           b.TotalMemory(),
		CpuShares:             b.CpuShares,
		CpuQuota:              b.CpuQuota,
		CpuPeriodMicroseconds: b.CpuPeriodMicroseconds(),
		CpuQuotaMicroseconds:  b.CpuQuotaMicroseconds(),
		CpuSet:                b.CpuSet,
		CpuSetStr:             b.CpuSetStr(),
		PublicKeys:            b.PublicKeys,
		SnapshotUrl:           b.SnapshotUrl,
		Runtime:               b.Runtime,
		Premium:               b.Premium,
	}
}

func (b *Box) getConfigFromSpec(spec *BoxSpec) pr.Config {
	msc := pr.MemorySpecChange{
		OldMemory:      b.Memory,
		OldTotalMemory: b.TotalMemory(),
	}
	if spec.Memory != b.Memory {
		b.Memory = spec.Memory
	}
	if spec.CpuShares != b.CpuShares {
		b.CpuShares = spec.CpuShares
	}
	if spec.VirtualMemory != b.VirtualMemory {
		b.VirtualMemory = spec.VirtualMemory
	}
	config := b.provisionerConfig()
	msc.NewMemory = b.Memory
	msc.NewTotalMemory = b.TotalMemory()
	config.MemorySpecChange = msc
	return config
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
	var id string
	var err error

	if b.willUseTemplate() {
		id, _, err = b.getProvisioner().Create(b.provisionerConfig())
		if err != nil {
			return &BoxOpError{Op: "create box from template", Err: err}
		}
	} else {
		id, _, err = b.getProvisioner().CreateFromSnapshot(b.provisionerConfig(),
			b.boxer.S3Auth)
		if err != nil {
			return &BoxOpError{Op: "create box from snapshot", Err: err}
		}
	}
	b.UniqueId = id
	b.State = "stopped"
	if err := b.SetQuota(); err != nil {
		return err
	}
	return nil
}

func (b *Box) Start(spec *BoxSpec) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Start %s", b.ContainerName(), b.MountName()),
	)(time.Now())
	beforeStart := func(root string) error {
		if err := b.WritePublicKeys(); err != nil {
			return &BoxOpError{Op: "write public keys", Err: err}
		}
		if err := b.setPortForwarding(); err != nil {
			return &BoxOpError{Op: "set port forwarding", Err: err}
		}
		if err := b.changeQuota(spec); err != nil {
			return &BoxOpError{Op: "spec change", Err: err}
		}
		return nil
	}
	_, _, err := b.getProvisioner().Start(b.getConfigFromSpec(spec), beforeStart)
	if err != nil {
		return err
	}
	b.State = "running"
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
	id, err := b.getProvisioner().Stop(b.provisionerConfig())
	if err != nil {
		return err
	}
	b.UniqueId = id
	b.State = "stopped"
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
	return b.getProvisioner().Destroy(b.provisionerConfig())
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

	tarPath, err := archive.Tar(tarName, path.Join(b.Rootfs(), "home", "action"))
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
	return path.Join(b.Rootfs(), "etc", "ssh", "univ_public_keys")
}

func (b *Box) EnvPath() string {
	return path.Join(b.Rootfs(), "etc", "environment")
}

// rootfs of the container. This is the root after unionized so its typically
// will be in boxer.Root i.e /var/lib/lxc
func (b *Box) Rootfs() string {
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
	config := b.getConfigFromSpec(spec)
	err := b.getProvisioner().ChangeConfig(config)
	if err != nil {
		return err
	}
	if err := b.changeQuota(spec); err != nil {
		return err
	}
	return b.boxer.ResourcePool.SaveBox(b)
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
	actionPath := path.Join(b.Rootfs(), "etc", "action")
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

func (b *Box) changeQuota(spec *BoxSpec) error {
	if len(spec.DiskSpace) > 0 && spec.DiskSpace != b.DiskSpace {
		oldDiskSpace := b.DiskSpace
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(); err != nil {
			b.DiskSpace = oldDiskSpace
			return &BoxOpError{Op: "set disk quota", Err: err}
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
	return arg
}

// return the quota strategy that the box should use. The return object will be
// an instance that supports quota.QuotaInterface interface
func (b *Box) quota() quota.QuotaInterface {
	return quota.Provisioner.GetQuota(b.QuotaMethod)
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
