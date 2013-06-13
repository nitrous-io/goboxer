// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package goboxer

import (
	"github.com/action-io/goboxer/host"
)

var runtimes []string

func init() {
	runtimes = []string{
		"rails",
		"nodejs",
		"django",
		"golang",
		"test",
	}
}

// we concat all messages type here and use `Receive` to distribute correct
// type to correct function for now
type Message struct {
	// all messages can have these
	Id         string            `json:"id"`
	BoxId      string            `json:"boxId"`
	Type       string            `json:"type"`
	ReturnVars map[string]string `json:"returnVars"`

	// for advertise message
	FreeMemory int      `json:"freeMemory"`
	Runtimes   []string `json:"runtimes"`

	// for create message
	// Name is used subsequently in coach message to describe containerName
	// i.e. 123-testbox
	Name        string   `json:"name"`
	Port        string   `json:"port"`
	Runtime     string   `json:"runtime"`
	SnapshotURL string   `json:"snapshotURL"`
	Memory      string   `json:"memory"`
	DiskSpace   string   `json:"diskSpace"`
	PublicKeys  []string `json:"publicKeys"`
	SshKeyPair  []string `json:"sshKeyPair"`

	// for normal message
	DataType string      `json:"dataType"`
	Data     interface{} `json:"data"`
	Success  bool        `json:"success"`
}

// a handler will receive message and runtime, do some work with it and return
// a new message
type MessageHandler func(m Message, boxer *Boxer) Message

type Boxer struct {
	Id           string
	Root         string
	SelectedDisk string
	Disks        []string
	ResourcePool *ResourcePool
	Runtimes     map[string]string
	Handles      map[string]MessageHandler
}

func (boxer *Boxer) Receive(msg Message) Message {
	handle, ok := boxer.Handles[msg.Type]
	if !ok {
		return boxer.NewInvalidParamMessage()
	}
	msg.Id = boxer.Id
	return handle(msg, boxer)
}

func (boxer *Boxer) NewAdvertiseMessage() Message {
	return Message{
		Type:       "aio:advertise",
		Id:         boxer.Id,
		Runtimes:   runtimes,
		FreeMemory: int(host.FreeMemory()),
	}
}

func (boxer *Boxer) NewInvalidParamMessage() Message {
	return Message{
		Type:    "aio:invalid:params",
		Id:      boxer.Id,
		Success: false,
	}
}

func LocateBoxHandler(m Message, boxer *Boxer) Message {
	return boxer.NewAdvertiseMessage()
}

func CreateBoxHandler(m Message, boxer *Boxer) Message {
	msg := Message{
		Type:  "aio:created",
		Id:    boxer.Id,
		BoxId: m.BoxId,
	}
	resource, err := boxer.ResourcePool.Acquire()
	if err != nil {
		msg.Success = false
		return msg
	}
	box := &Box{
		Id:               m.BoxId,
		BoxerId:          m.Id,
		Name:             m.Name,
		Runtime:          m.Runtime,
		Memory:           m.Memory,
		DiskSpace:        m.DiskSpace,
		PublicKeys:       m.PublicKeys,
		SnapshotURL:      m.SnapshotURL,
		Disk:             boxer.SelectedDisk,
		Template:         boxer.Runtimes[m.Runtime],
		Ip:               resource.Ip,
		Gateway:          resource.Gateway,
		NetworkInterface: resource.NetworkInterface,
		Port:             resource.Port,
	}
	if err := boxer.Create(box); !err.Success() {
		boxer.ResourcePool.Release(resource.Ip)
		msg.Success = false
		return msg
	}
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
		return msg
	}
	msg.Port = box.Port
	msg.Success = true
	return msg
}

func StartBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	if err := boxer.Start(box); !err.Success() {
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:started",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	box.State = "running"
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
	}
	return msg
}

func StopBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	if err := boxer.Stop(box); !err.Success() {
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:stopped",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	box.State = "stopped"
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
	}
	return msg
}

func RestartBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	if err := boxer.Restart(box); !err.Success() {
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:restarted",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	box.State = "running"
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
	}
	return msg
}

func DestroyBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	if err := boxer.Destroy(box); !err.Success() {
		return boxer.NewInvalidParamMessage()
	}
	if err := boxer.ResourcePool.Release(box.Ip); err != nil {
		return boxer.NewInvalidParamMessage()
	}
	return Message{
		Type:       "aio:destroyed",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
}

func ConfigureBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	message := Message{
		Type:       "aio:configured",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
	}
	switch m.DataType {
	case "aio:configure:add_public_key":
		data := (m.Data).(string)
		if err := box.AddPublicKey(data); err != nil {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		return message
	case "aio:configure:remove_public_key":
		data := (m.Data).(string)
		if err := box.RemovePublicKey(data); err != nil {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		return message
	case "aio:configure:change_specs":
		data := (m.Data).(map[string]interface{})
		spec := &BoxSpec{}
		spec.Memory = (data["memory"]).(string)
		spec.DiskSpace = (data["diskSpace"]).(string)
		if err := box.ChangeSpec(spec); err != nil {
			return boxer.NewInvalidParamMessage()
		}
		if err := boxer.Restart(box); !err.Success() {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		box.State = "running"
		if err := boxer.ResourcePool.SaveBox(box); err != nil {
			message.Success = false
		}
		return message
	}
	return boxer.NewInvalidParamMessage()
}

func SetupContainer(c *BoxCommand) error {
	box := c.Box
	if err := box.ExtractTemplate(); err != nil {
		return err
	}
	if err := box.WriteConfig(); err != nil {
		return err
	}
	if err := box.SetHostname(); err != nil {
		return err
	}
	if err := box.SetNetworkConfig(); err != nil {
		return err
	}
	if err := box.SetWorkspace(); err != nil {
		return err
	}
	if err := box.SetDefaultKeypair(); err != nil {
		return err
	}
	if err := box.SetPermission(); err != nil {
		return err
	}
	if err := box.SetBoxId(); err != nil {
		return err
	}
	if err := box.SetPublicKeys(); err != nil {
		return err
	}
	return nil
}

func MountContainer(c *BoxCommand) error      { return c.Box.Mount(c.Boxer.Root) }
func UnmountContainer(c *BoxCommand) error    { return c.Box.Unmount(c.Boxer.Root) }
func SetQuota(c *BoxCommand) error            { return c.Box.SetQuota() }
func UnsetQuota(c *BoxCommand) error          { return c.Box.UnsetQuota() }
func SetPortForwarding(c *BoxCommand) error   { return c.Box.SetPortForwarding() }
func UnsetPortForwarding(c *BoxCommand) error { return c.Box.UnsetPortForwarding() }
func StartContainer(c *BoxCommand) error      { return c.Box.StartContainer() }
func StopContainer(c *BoxCommand) error       { return c.Box.StopContainer() }
func DeleteContainerData(c *BoxCommand) error { return c.Box.DeleteContainerData() }
func Pass(c *BoxCommand) error                { return nil }

var CreateStates []State
var StartStates []State
var StopStates []State
var RestartStates []State
var DestroyStates []State

func init() {
	CreateStates = []State{
		State{Advance: MountContainer, Rollback: UnmountContainer},
		State{Advance: SetupContainer, Rollback: Pass},
		State{Advance: SetQuota, Rollback: UnsetQuota},
		State{Advance: SetPortForwarding, Rollback: UnsetPortForwarding},
	}

	StartStates = []State{
		State{Advance: MountContainer, Rollback: UnmountContainer},
		State{Advance: StartContainer, Rollback: StopContainer},
	}

	StopStates = []State{
		State{Advance: StopContainer, Rollback: Pass},
		State{Advance: UnmountContainer, Rollback: MountContainer},
		State{Advance: UnsetQuota, Rollback: SetQuota},
	}

	DestroyStates = []State{
		State{Advance: StopContainer, Rollback: Pass},
		State{Advance: UnsetPortForwarding, Rollback: SetPortForwarding},
		State{Advance: UnmountContainer, Rollback: MountContainer},
		State{Advance: UnsetQuota, Rollback: SetQuota},
		State{Advance: DeleteContainerData, Rollback: Pass},
	}

	RestartStates = []State{
		State{Advance: StopContainer, Rollback: Pass},
		State{Advance: UnmountContainer, Rollback: Pass},
		State{Advance: MountContainer, Rollback: Pass},
		State{Advance: StartContainer, Rollback: Pass},
	}
}

func (boxer *Boxer) Create(box *Box) *CommandError {
	return Advance(&BoxCommand{
		Name:   "Create",
		Boxer:  boxer,
		Box:    box,
		States: CreateStates,
	})
}

func (boxer *Boxer) Start(box *Box) *CommandError {
	return Advance(&BoxCommand{
		Name:   "Start",
		Boxer:  boxer,
		Box:    box,
		States: StartStates,
	})
}

func (boxer *Boxer) Restart(box *Box) *CommandError {
	return Advance(&BoxCommand{
		Name:   "Restart",
		Boxer:  boxer,
		Box:    box,
		States: RestartStates,
	})
}

func (boxer *Boxer) Stop(box *Box) *CommandError {
	return Advance(&BoxCommand{
		Name:   "Stop",
		Boxer:  boxer,
		Box:    box,
		States: StopStates,
	})
}

func (boxer *Boxer) Destroy(box *Box) *CommandError {
	return Advance(&BoxCommand{
		Name:   "Destroy",
		Boxer:  boxer,
		Box:    box,
		States: DestroyStates,
	})
}
