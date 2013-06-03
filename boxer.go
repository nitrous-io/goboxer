// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package goboxer

type Boxer struct {
	Root     string
	Runtimes map[string]string
}

func (boxer *Boxer) Create(box *Box) error {
	createCommand := &BoxCommand{
		Name:         "Create",
		Boxer:        boxer,
		Box:          box,
		CurrentIndex: 0,
		States: []State{
			State{Name: "1", Advance: CreateContainerPath, Rollback: DestroyContainerPath},
			State{Name: "2", Advance: CreateContainerDeltaPath, Rollback: DestroyContainerDeltaPath},
			State{Name: "3", Advance: MountContainer, Rollback: UnmountContainer},
			State{Name: "4", Advance: ExtractTemplate, Rollback: Pass},
			State{Name: "5", Advance: SetEtcProjects, Rollback: UnsetEtcProjects},
			State{Name: "6", Advance: SetEtcProjid, Rollback: UnsetEtcProjid},
			State{Name: "7", Advance: SetXfsQuota, Rollback: UnsetXfsQuota},
			State{Name: "8", Advance: SetLxcConfig, Rollback: Pass},
			State{Name: "9", Advance: SetHostname, Rollback: Pass},
			State{Name: "10", Advance: SetNetworkConfig, Rollback: Pass},
			State{Name: "11", Advance: SetUserUid, Rollback: Pass},
			State{Name: "12", Advance: SetUserGid, Rollback: Pass},
			State{Name: "13", Advance: SetHomePermission, Rollback: Pass},
			State{Name: "14", Advance: SetWorkspace, Rollback: Pass},
			State{Name: "15", Advance: SetBoxId, Rollback: UnsetBoxId},
			State{Name: "16", Advance: SetPortForwarding, Rollback: UnsetPortForwarding},
			State{Name: "17", Advance: SetDefaultKeyPair, Rollback: Pass},
		},
	}
	return Advance(createCommand)
}

func (boxer *Boxer) Start(box *Box) error {
	startCommand := &BoxCommand{
		Name:  "Start",
		Boxer: boxer,
		Box:   box,
		States: []State{
			State{Advance: MountContainer, Rollback: UnmountContainer},
			State{Advance: StartContainer, Rollback: StopContainer},
		},
	}
	return Advance(startCommand)
}

func (boxer *Boxer) Restart(box *Box) error {
	restartCommand := &BoxCommand{
		Name:  "Restart",
		Boxer: boxer,
		Box:   box,
		States: []State{
			State{Advance: StopContainer, Rollback: Pass},
			State{Advance: StartContainer, Rollback: Pass},
		},
	}
	return Advance(restartCommand)
}

func (boxer *Boxer) Stop(box *Box) error {
	stopCommand := &BoxCommand{
		Name:  "Stop",
		Boxer: boxer,
		Box:   box,
		States: []State{
			State{Advance: StopContainer, Rollback: Pass},
			State{Advance: UnsetEtcProjects, Rollback: SetEtcProjects},
			State{Advance: UnsetEtcProjid, Rollback: SetEtcProjid},
			State{Advance: UnsetXfsQuota, Rollback: SetXfsQuota},
			State{Advance: UnmountContainer, Rollback: Pass},
		},
	}
	return Advance(stopCommand)
}

func (boxer *Boxer) Destroy(box *Box) error {
	states := []State{
		State{Advance: Pass, Rollback: SetDefaultKeyPair},
		State{Advance: UnsetPortForwarding, Rollback: SetPortForwarding},
		State{Advance: UnsetBoxId, Rollback: SetBoxId},
		State{Advance: Pass, Rollback: SetWorkspace},
		State{Advance: Pass, Rollback: SetHomePermission},
		State{Advance: Pass, Rollback: SetUserGid},
		State{Advance: Pass, Rollback: SetUserUid},
		State{Advance: Pass, Rollback: SetNetworkConfig},
		State{Advance: Pass, Rollback: SetHostname},
		State{Advance: Pass, Rollback: SetLxcConfig},
		State{Advance: Pass, Rollback: ExtractTemplate},
	}

	boxMounted, _ := Mounted(boxPath(boxer, box))
	if boxMounted {
		states = append(states,
			State{Advance: UnsetXfsQuota, Rollback: SetXfsQuota},
			State{Advance: UnsetEtcProjid, Rollback: SetEtcProjid},
			State{Advance: UnsetEtcProjects, Rollback: SetEtcProjects},
			State{Advance: UnmountContainer, Rollback: MountContainer},
		)
	}

	states = append(states,
		State{Advance: DestroyContainerDeltaPath, Rollback: CreateContainerDeltaPath},
		State{Advance: DestroyContainerPath, Rollback: CreateContainerPath},
	)

	destroyCommand := &BoxCommand{
		Name:   "Destroy",
		Boxer:  boxer,
		Box:    box,
		States: states,
	}
	return Advance(destroyCommand)
}
