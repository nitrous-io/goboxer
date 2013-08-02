// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package goboxer

import (
	"fmt"
	"github.com/action-io/goboxer/host"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/s3"
	"sync"
)

var handlers map[string]MessageHandler
var CreateStates []State
var StartStates []State
var StopStates []State
var RestartStates []State
var DestroyStates []State

func init() {
	handlers = map[string]MessageHandler{
		"aio:locate":       logger.LogHandler("aio:locate", LocateBoxHandler),
		"aio:create":       logger.LogHandler("aio:create", CreateBoxHandler),
		"aio:start":        logger.LogHandler("aio:start", StartBoxHandler),
		"aio:stop":         logger.LogHandler("aio:stop", StopBoxHandler),
		"aio:restart":      logger.LogHandler("aio:restart", RestartBoxHandler),
		"aio:destroy":      logger.LogHandler("aio:destroy", DestroyBoxHandler),
		"aio:configure":    logger.LogHandler("aio:configure", ConfigureBoxHandler),
		"aio:info_request": logger.LogHandler("aio:info_request", InfoRequestBoxHandler),
		"aio:migrate":      logger.LogHandler("aio:migrate", MigrateBoxHandler),
	}

	CreateStates = []State{
		State{Name: "CreateMountContainer", Advance: MountContainer, Rollback: UnmountContainer},
		State{Name: "CreateSetupContainer", Advance: SetupContainer, Rollback: Pass},
		State{Name: "CreateSetQuota", Advance: SetQuota, Rollback: UnsetQuota},
		State{Name: "CreateSetPortForwarding", Advance: SetPortForwarding, Rollback: UnsetPortForwarding},
	}

	StartStates = []State{
		State{Name: "StartMountContainer", Advance: MountContainer, Rollback: UnmountContainer},
		State{Name: "StartSetQuota", Advance: SetQuota, Rollback: UnsetQuota},
		State{Name: "StartStartContainer", Advance: StartContainer, Rollback: StopContainer},
	}

	StopStates = []State{
		State{Name: "StopStopContainer", Advance: StopContainer, Rollback: Pass},
		State{Name: "StopUnmountContainer", Advance: UnmountContainer, Rollback: MountContainer},
		State{Name: "StopUnsetQuota", Advance: UnsetQuota, Rollback: SetQuota},
	}

	DestroyStates = []State{
		State{Name: "DestroyStopContainer", Advance: StopContainer, Rollback: Pass},
		State{Name: "DestroyUnsetPortForwarding", Advance: UnsetPortForwarding, Rollback: SetPortForwarding},
		State{Name: "DestroyUnmountCcontainer", Advance: UnmountContainer, Rollback: MountContainer},
		State{Name: "DestroyUnsetQuota", Advance: UnsetQuota, Rollback: SetQuota},
		State{Name: "DestroyDeleteContainerData", Advance: DeleteContainerData, Rollback: Pass},
	}

	RestartStates = []State{
		State{Name: "RestartStopContainer", Advance: StopContainer, Rollback: Pass},
		State{Name: "RestartUnmountContainer", Advance: UnmountContainer, Rollback: Pass},
		State{Name: "RestartMountContainer", Advance: MountContainer, Rollback: Pass},
		State{Name: "RestartStartContainer", Advance: StartContainer, Rollback: Pass},
	}
}

// a handler will receive message and runtime, do some work with it and return
// a new message
type MessageHandler func(m Message, boxer *Boxer) Message

type Boxer struct {
	sync.Mutex
	Id           string
	Root         string
	SelectedDisk string
	Disks        []string
	ResourcePool *ResourcePool
	Runtimes     map[string]string
	Handles      map[string]MessageHandler
	Quota        quota.QuotaInterface
	S3Auth       s3.AuthInterface
	S3Region     string
	S3Bucket     string
}

func NewBoxer(id, root, selectedDisk string, disks []string,
	runtimes map[string]string, resourcePool *ResourcePool,
	useProjQuota bool, s3auth s3.AuthInterface, region string,
	bucket string) *Boxer {
	boxer := &Boxer{
		Id:           id,
		Root:         root,
		Runtimes:     runtimes,
		SelectedDisk: selectedDisk,
		Disks:        disks,
		Handles:      handlers,
		ResourcePool: resourcePool,
		S3Auth:       s3auth,
		S3Region:     region,
		S3Bucket:     bucket,
	}
	if useProjQuota {
		boxer.Quota = quota.Quota{}
	} else {
		boxer.Quota = quota.UserQuota{}
	}
	return boxer
}

// return back a slice of keys for the runtimes
//
//		boxer.Runtimes = map[string]string{
// 		 	"rails": "ruby_template",
// 		 	"django": "python_template",
// 		}
// 		boxer.runtimeKeys()
// 		>> []string{"django", "rails"}
func (boxer *Boxer) runtimeKeys() []string {
	var keys []string
	for k := range boxer.Runtimes {
		keys = append(keys, k)
	}
	return keys
}

// This is the main logical entry point for a boxer program. It accepts an
// incoming `Message` ( from Coach via Redis ) and return back a `Message`
// which we can used to reply back to Coach
//
// The handlers are created when we created the `Boxer` instance and stored
// inside `Handles` attributes. For example:
//
//		boxer.Handlers = map[string]MessageHandler{
//			"aio:locate": LocateBoxHandler,
//			"aio:create": CreateBoxHandler,
//		}
//
// Then, whenever we received a `Message` with `Type` == "aio:locate" or
// "aio:create", we knows how to dispatch the message to `LocateBoxHandler` and
// `CreateBoxHandler` function correctly
func (boxer *Boxer) Receive(msg Message) (message Message) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); !ok {
				logger.Println("Error in Receive", msg, err)
			}
			message = boxer.NewInvalidParamMessage()
		}
	}()
	handle, ok := boxer.Handles[msg.Type]
	if !ok {
		message = boxer.NewInvalidParamMessage()
	} else {
		msg.Id = boxer.Id
		message = handle(msg, boxer)
	}
	return
}

func (boxer *Boxer) NewAdvertiseMessage() Message {
	return Message{
		Type:       "aio:advertise",
		Id:         boxer.Id,
		Runtimes:   boxer.runtimeKeys(),
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
		logger.Println("ERROR: Cant acquire resource", err)
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
		Port:             resource.GetPort(),
	}
	box.boxer = boxer
	// create the base image for the box. Base image will be named as
	// nitrous/<box_id>
	if err := box.CreateImage(); err != nil {
		logger.Printf("ERROR: Fail to create image %v, err: %v\n", box, err)
		boxer.ResourcePool.Release(resource.Ip)
		msg.Success = false
		return msg
	}
	// from the base image, we run a new container. This is where all user
	// changes happens
	if err := box.CreateContainer(); err != nil {
		logger.Printf("ERROR: Fail to create container %v, err: %v\n", box, err)
		msg.Success = false
		return msg
	}
	// set quota for the box. This should only works with usrquota for now
	box.SetQuota(boxer.Quota)
	// start the new container
	if err := box.StartContainer(); err != nil {
		logger.Printf("ERROR: Fail to start %v, err: %v\n", box, err)
		msg.Success = false
		return msg
	}
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		logger.Printf("ERROR: Fail to save %v, err: %v\n", box, err)
		msg.Success = false
		return msg
	}
	msg.box = box
	msg.Port = box.Port
	msg.Success = true
	return msg
}

func StartBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	if err := box.StartContainer(); err != nil {
		logger.Printf("ERROR: Fail to start %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:started",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
		box:        box,
	}
	if err := boxer.ResourcePool.UpdateState(box, "running"); err != nil {
		logger.Printf("ERROR: Fail to update box state %v, err: %v\n", box, err)
		msg.Success = false
	}
	return msg
}

func StopBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		logger.Printf("ERROR: Fail to find box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	if err := box.StopContainer(); err != nil {
		logger.Printf("ERROR: Fail to stop box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:stopped",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
		box:        box,
	}
	if err := boxer.ResourcePool.UpdateState(box, "stopped"); err != nil {
		logger.Printf("ERROR: Fail to update box state %v, err: %v\n", box, err)
		msg.Success = false
	}
	return msg
}

func RestartBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		logger.Printf("ERROR: Fail to find box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	if err := box.RestartContainer(); err != nil {
		logger.Printf("ERROR: Fail to restart box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:restarted",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
		box:        box,
	}
	if err := boxer.ResourcePool.UpdateState(box, "running"); err != nil {
		logger.Printf("ERROR: Fail to update box state %v, err: %v\n", box, err)
		msg.Success = false
	}
	return msg
}

func DestroyBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		logger.Printf("ERROR: Fail to find box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	if err := box.DestroyContainer(); err != nil {
		logger.Printf("ERROR: Fail to destroy box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	if err := boxer.ResourcePool.DeleteBox(box); err != nil {
		logger.Printf("ERROR: Fail to release box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	return Message{
		Type:       "aio:destroyed",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
		box:        box,
	}
}

func ConfigureBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	message := Message{
		Type:       "aio:configured",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		box:        box,
	}
	switch m.DataType {
	case "aio:configure:add_public_key":
		message.DataType = "aio:configured:add_public_key"
		data, ok := (m.Data).(string)
		if !ok {
			return boxer.NewInvalidParamMessage()
		}
		if err := box.AddPublicKey(data); err != nil {
			logger.Println("failed to add public key", err)
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		return message
	case "aio:configure:remove_public_key":
		message.DataType = "aio:configured:remove_public_key"
		data, ok := (m.Data).(string)
		if !ok {
			return boxer.NewInvalidParamMessage()
		}
		if err := box.RemovePublicKey(data); err != nil {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		return message
	case "aio:configure:change_specs":
		message.DataType = "aio:configured:change_specs"
		message.Data = map[string]interface{}{
			"memory":    box.Memory,
			"diskSpace": box.DiskSpace,
		}
		data, ok := (m.Data).(map[string]interface{})
		if !ok {
			message.Success = false
			return message
		}
		spec := &BoxSpec{}
		spec.Memory, ok = (data["memory"]).(string)
		if !ok {
			message.Success = false
			return message
		}
		spec.DiskSpace, ok = (data["diskSpace"]).(string)
		if !ok {
			message.Success = false
			return message
		}
		err = box.ChangeSpec(spec, boxer.Quota)
		message.Data = map[string]interface{}{
			"memory":    box.Memory,
			"diskSpace": box.DiskSpace,
		}
		if err != nil {
			message.Success = false
			return message
		}
		if err := boxer.ResourcePool.UpdateState(box, "running"); err != nil {
			message.Success = false
			return message
		}
		message.Success = true
		return message
	case "aio:configure:set_env_variables":
		message.DataType = "aio:configured:set_env_variables"
		data, ok := (m.Data).(map[string]interface{})
		if !ok {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		if err := box.SetEnv(data); err != nil {
			message.Success = false
		}
		return message
	case "aio:configure:remove_env_variables":
		message.DataType = "aio:configured:remove_env_variables"
		data := []string{}
		vdata, ok := (m.Data).([]interface{})
		if !ok {
			return boxer.NewInvalidParamMessage()
		}
		for i := 0; i < len(vdata); i++ {
			data = append(data, vdata[i].(string))
		}
		if len(data) == 0 {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		if err := box.RemoveEnv(data); err != nil {
			message.Success = false
		}
		return message
	}
	return boxer.NewInvalidParamMessage()
}

func InfoRequestBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	message := Message{
		Type:       "aio:info_response",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		box:        box,
	}
	info, err := box.Info(boxer.Quota)
	if err != nil {
		message.Success = false
		return message
	}
	message.Info = info
	message.Success = true
	return message
}

func MigrateBoxHandler(m Message, boxer *Boxer) Message {
	// use the same sent message structure
	message := m
	message.Success = false

	switch m.DataType {
	case "aio:migrate:create":
		message.DataType = "aio:migrate:created"
		resource, err := boxer.ResourcePool.Acquire()
		if err != nil {
			logger.Println("ERROR: Cant acquire resource", err)
			return message
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
			Port:             resource.GetPort(),
		}
		box.boxer = boxer
		if err := box.Migrate(boxer.S3Auth); err != nil {
			logger.Printf("ERROR: Fail to migrate %v, err: %v\n", box, err)
			boxer.ResourcePool.Release(resource.Ip)
			return message
		}
		if err := boxer.ResourcePool.SaveBox(box); err != nil {
			logger.Printf("ERROR: Fail to save %v, err: %v\n", box, err)
			return message
		}
		message.box = box
		message.Port = box.Port
		message.Success = true
	}
	return message
}

func SetupContainer(c *BoxCommand) error {
	box := c.Box
	if err := box.WriteConfig(); err != nil {
		return err
	}
	if err := box.SetHostname(); err != nil {
		return err
	}
	if err := box.SetNetworkConfig(); err != nil {
		return err
	}
	if err := box.SetupHome(c.Boxer.S3Auth); err != nil {
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
func SetQuota(c *BoxCommand) error            { return c.Box.SetQuota(c.Boxer.Quota) }
func UnsetQuota(c *BoxCommand) error          { return c.Box.UnsetQuota(c.Boxer.Quota) }
func SetPortForwarding(c *BoxCommand) error   { return c.Box.SetPortForwarding() }
func UnsetPortForwarding(c *BoxCommand) error { return c.Box.UnsetPortForwarding() }
func StartContainer(c *BoxCommand) error      { return c.Box.StartContainer() }
func StopContainer(c *BoxCommand) error       { return c.Box.StopContainer() }
func DeleteContainerData(c *BoxCommand) error { return c.Box.DeleteContainerData() }
func Pass(c *BoxCommand) error                { return nil }

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

func (boxer *Boxer) Migrate(box *Box) *CommandError {
	return Advance(&BoxCommand{
		Name:   "Migrate",
		Boxer:  boxer,
		Box:    box,
		States: CreateStates,
	})
}

// Given a list of boxes, try to start all of them.
// This should process all the boxes even when some cant be started
func (boxer *Boxer) StartAll(boxes []*Box) error {
	var errors []error
	for _, box := range boxes {
		box.boxer = boxer
		err := Advance(&BoxCommand{
			Name:   "Start",
			Boxer:  boxer,
			Box:    box,
			States: StartStates,
		})
		if err.Success() {
			// make sure that the box are in running state
			if err := boxer.ResourcePool.UpdateState(box, "running"); err != nil {
				errors = append(errors, err)
			}
		} else {
			errors = append(errors, fmt.Errorf("Failed to start box: %s",
				box.ContainerName()))
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("Failed to start all boxes %v", errors)
	}
	return nil
}

// start all running boxes based on data from `ResourcePool`
func (boxer *Boxer) Init() error {
	boxes, err := boxer.ResourcePool.FindRunningBoxes()
	if err != nil {
		return err
	}
	return boxer.StartAll(boxes)
}

// wrapper around ResourcePool#FindBox so we can set Boxer property of box
func (boxer *Boxer) findBox(name string) (*Box, error) {
	box, err := boxer.ResourcePool.FindBox(name)
	if err != nil {
		return nil, err
	}
	box.boxer = boxer
	return box, nil
}
