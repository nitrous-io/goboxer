package goboxer

type BoxSpec struct {
	Memory        string `json:"memory,omitempty"`
	DiskSpace     string `json:"diskSpace,omitempty"`
	CpuShares     int    `json:"cpuShares,omitempty"`
	VirtualMemory string `json:"virtualMemory,omitempty"`
}

// this return true if all values are parsed and merge cleanly
func (s *BoxSpec) MergeData(m Message) bool {
	data, ok := (m.Data).(map[string]interface{})
	if !ok {
		return false
	}
	var memoryOk, virtualMemoryOk, cpuSharesOk, diskSpaceOk bool
	var memory, diskSpace, virtualMemory string
	var cpuShares float64

	// if data is missing, we assume that its safely to ignore that field
	memoryOk = data["memory"] == nil
	virtualMemoryOk = data["virtualMemory"] == nil
	cpuSharesOk = data["cpuShares"] == nil
	diskSpaceOk = data["diskSpace"] == nil

	// convert the data if its not missing
	if !memoryOk {
		memory, memoryOk = (data["memory"]).(string)
		if memoryOk {
			s.Memory = memory
		}
	}
	if !virtualMemoryOk {
		virtualMemory, virtualMemoryOk = (data["virtualMemory"]).(string)
		if virtualMemoryOk {
			s.VirtualMemory = virtualMemory
		}
	}
	if !cpuSharesOk {
		cpuShares, cpuSharesOk = (data["cpuShares"]).(float64)
		if cpuSharesOk {
			s.CpuShares = int(cpuShares)
		}
	}
	if !diskSpaceOk {
		diskSpace, diskSpaceOk = (data["diskSpace"]).(string)
		if diskSpaceOk {
			s.DiskSpace = diskSpace
		}
	}

	return memoryOk && virtualMemoryOk && cpuSharesOk && diskSpaceOk
}

func (s *BoxSpec) ToData() map[string]interface{} {
	return map[string]interface{}{
		"memory":        s.Memory,
		"diskSpace":     s.DiskSpace,
		"cpuShares":     s.CpuShares,
		"virtualMemory": s.VirtualMemory,
	}
}
