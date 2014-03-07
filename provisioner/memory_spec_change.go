package provisioner

import (
	"github.com/action-io/goboxer/quota"
)

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
func (msc MemorySpecChange) SettingOrder() [][]string {
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
