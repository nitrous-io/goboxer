package provisioner

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/action-io/goboxer/quota"
)

const DEFAULT_CPU_PERIOD = 1000000 // cpu.cfs_period_us: 1 million microseconds

// container resources such as memory, swap, cpu etc.
// provisioner should use this to manipulate the container resource when start
type Resource struct {
	Memory      string
	TotalMemory string // memory + swap
	CpuShares   int
	CpuQuota    float64
	CpuSet      []int
}

// return the valid value that should be applied to `cpuset.cpus`
func (r *Resource) CpuSetStr() string {
	cpus := []string{}
	for _, cpu := range r.CpuSet {
		cpus = append(cpus, strconv.Itoa(cpu))
	}
	return strings.Join(cpus, ",")
}

// Returns cpu.cfs_period_us setting for this box.
func (r *Resource) CpuPeriodMicroseconds() int {
	return DEFAULT_CPU_PERIOD
}

// Returns cpu.cfs_quota_us setting for this box.
func (r *Resource) CpuQuotaMicroseconds() int {
	return int(r.CpuQuota * DEFAULT_CPU_PERIOD)
}

func (r *Resource) ToCgroupArgs() []string {
	args := []string{}
	if len(r.Memory) > 0 {
		args = append(args, fmt.Sprintf(
			"memory.limit_in_bytes=%s", r.Memory))
	}
	if len(r.TotalMemory) > 0 {
		args = append(args, fmt.Sprintf(
			"memory.memsw.limit_in_bytes=%s", r.TotalMemory))
	}
	if r.CpuShares > 0 {
		args = append(args, fmt.Sprintf(
			"cpu.shares=%d", r.CpuShares))
	}
	if r.CpuQuota > 0 {
		args = append(args,
			fmt.Sprintf("cpu.cfs_period_us=%d", r.CpuPeriodMicroseconds()),
			fmt.Sprintf("cpu.cfs_quota_us=%d", r.CpuQuotaMicroseconds()))
	}
	if len(r.CpuSet) > 0 {
		args = append(args,
			fmt.Sprintf("cpuset.cpus=%s", r.CpuSetStr()))
	}
	return args
}

// MemorySpecChange contains old and new ValueUnit strings for memory and total
// memory
type MemorySpecChange struct {
	OldMemory      string
	OldTotalMemory string
	NewMemory      string
	NewTotalMemory string
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
