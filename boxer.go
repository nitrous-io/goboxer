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

func (boxer *Boxer) Create(box *Box) *CommandError {
	createCommand := &BoxCommand{
		Name:         "Create",
		Boxer:        boxer,
		Box:          box,
		CurrentIndex: 0,
		States: []State{
			State{Advance: CreateContainerPath, Rollback: DestroyContainerPath},
			State{Advance: CreateContainerDeltaPath, Rollback: DestroyContainerDeltaPath},
			State{Advance: MountContainer, Rollback: UnmountContainer},
			State{Advance: ExtractTemplate, Rollback: Pass},
			State{Advance: SetEtcProjects, Rollback: UnsetEtcProjects},
			State{Advance: SetEtcProjid, Rollback: UnsetEtcProjid},
			State{Advance: SetXfsQuota, Rollback: UnsetXfsQuota},
			State{Advance: SetLxcConfig, Rollback: Pass},
			State{Advance: SetHostname, Rollback: Pass},
			State{Advance: SetNetworkConfig, Rollback: Pass},
			State{Advance: SetUserUid, Rollback: Pass},
			State{Advance: SetUserGid, Rollback: Pass},
			State{Advance: SetHomePermission, Rollback: Pass},
			State{Advance: SetWorkspace, Rollback: Pass},
			State{Advance: SetBoxId, Rollback: UnsetBoxId},
			State{Advance: SetPortForwarding, Rollback: UnsetPortForwarding},
			State{Advance: SetDefaultKeyPair, Rollback: Pass},
			State{Advance: SetPublicKeys, Rollback: UnsetPublicKeys},
		},
	}
	return Advance(createCommand)
}

func (boxer *Boxer) Start(box *Box) *CommandError {
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

func (boxer *Boxer) Restart(box *Box) *CommandError {
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

func (boxer *Boxer) Stop(box *Box) *CommandError {
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

func (boxer *Boxer) Destroy(box *Box) *CommandError {
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
