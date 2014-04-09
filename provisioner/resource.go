package provisioner

import (
	"strconv"
	"strings"
)

const DEFAULT_CPU_PERIOD = 1000000 // cpu.cfs_period_us: 1 million microseconds

// container resources such as memory, swap, cpu etc.
// provisioner should use this to manipulate the container resource when start
type Resource struct {
	Memory      int
	TotalMemory int // memory + swap
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
