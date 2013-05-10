// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package boxer

import (
	"github.com/action-io/goboxer/box"
	"github.com/action-io/goboxer/resource"
)

type Boxer struct {
	Root     string
	Runtimes map[string]string
}

func (boxer *Boxer) Create(config *box.Config, resource *resource.Resource) (*box.Box, error) {
	box := &box.Box{Config: config, Resource: resource}

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
		},
	}
	Advance(createCommand)
	return box, nil
}

func (boxer *Boxer) Start(box *box.Box) {
}

func (boxer *Boxer) Restart(box *box.Box) {
}

func (boxer *Boxer) Stop(box *box.Box) error {
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
	Advance(stopCommand)
	return nil
}

func (boxer *Boxer) Destroy(box *box.Box) {
}
