// A service which provisions and manages boxes (LXC containers).
//
// It's somewhat similar to Docker.
// We differentiate from Docker by using a `ResourcePool` to manage available
// resources like ip, open port instead of auto-generating them on-the-fly.
package goboxer

import (
	"fmt"
	"github.com/action-io/goboxer/cartman"
	"github.com/action-io/goboxer/host"
	"github.com/action-io/goboxer/s3"
	"runtime"
)

var handlers map[string]MessageHandler

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
		"aio:snapshot":     logger.LogHandler("aio:snapshot", SnapshotBoxHandler),
	}
}

// a handler will receive message and runtime, do some work with it and return
// a new message
type MessageHandler func(m Message, boxer *Boxer) Message

type Boxer struct {
	locker          BoxLocker
	Id              string
	Vagrant         bool
	Root            string
	SelectedDisk    string
	Disks           []string
	ResourcePool    *ResourcePool
	Runtimes        map[string]string
	Handles         map[string]MessageHandler
	MountMethod     string
	QuotaMethod     string
	SelectedVG      string
	CpuSet          []int
	Cartman         *cartman.Cartman
	PremiumCPUQuota float64
	DefaultCPUQuota float64
	S3Auth          s3.AuthInterface
	S3Region        string
	S3Bucket        string
}

func (boxer *Boxer) Configure() {
	boxer.Handles = handlers
	boxer.SetCpuSet(runtime.NumCPU())
	boxer.locker = BoxLocker{}
}

func (boxer *Boxer) Lock(id int) {
	boxer.locker.Lock(id)
}

func (boxer *Boxer) Unlock(id int) {
	boxer.locker.Unlock(id)
}

func (boxer *Boxer) SetCpuSet(numCpu int) {
	boxer.CpuSet = []int{}
	switch numCpu {
	case 1:
		boxer.CpuSet = append(boxer.CpuSet, 0)
	default:
		for i := 1; i < numCpu; i++ {
			boxer.CpuSet = append(boxer.CpuSet, i)
		}
	}
}

func (boxer *Boxer) ReceiveAsync(message Message) MessageChannel {
	c := make(MessageChannel, 1)
	go func() {
		c <- boxer.Receive(message)
	}()
	return c
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
			logger.Println("PANIC", r)
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

func CreateBoxHandler(m Message, boxer *Boxer) (msg Message) {
	msg = Message{
		Type:    "aio:created",
		Id:      boxer.Id,
		BoxId:   m.BoxId,
		Success: false,
	}
	resource, err := boxer.ResourcePool.Acquire()
	if err != nil {
		logger.Println("ERROR: Cant acquire resource", err)
		return
	}
	box := &Box{
		Id:               m.BoxId,
		BoxerId:          m.Id,
		Name:             m.Name,
		Runtime:          m.Runtime,
		Memory:           m.Memory,
		DiskSpace:        m.DiskSpace,
		PublicKeys:       m.PublicKeys,
		SnapshotId:       m.SnapshotId,
		SnapshotUrl:      m.SnapshotUrl,
		Template:         boxer.Runtimes[m.Runtime],
		Ip:               resource.Ip,
		Gateway:          resource.Gateway,
		NetworkInterface: resource.NetworkInterface,
		Port:             resource.GetPort(),
		Disk:             boxer.SelectedDisk,
		MountMethod:      boxer.MountMethod,
		QuotaMethod:      boxer.QuotaMethod,
		SelectedVG:       boxer.SelectedVG,
		CpuShares:        m.CpuShares,
		VirtualMemory:    m.VirtualMemory,
		Premium:          m.Premium,
	}
	box.boxer = boxer
	box.Prepare()
	if err := box.Create(); err != nil {
		logger.Printf("ERROR: Failed to create %v, err: %v\n", box, err)
		// Try to destroy the box.
		if err = box.Destroy(); err != nil {
			logger.Printf("ERROR: Failed to destroy %v, err: %v\n", box, err)
		}
		boxer.ResourcePool.Release(resource.Ip)
		return
	}
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		logger.Printf("ERROR: Failed to save %v, err: %v\n", box, err)
		return
	}
	msg.Port = box.Port
	msg.Success = true
	return
}

// Start message will have this format
// {
//	id: boxerId,
//	type: 'aio:start',
//	name: containerName-id,
//	returnVars: {...},
//	data: {
//	 memory: ...,
//	 virtualMemory: ...,
//	 cpuShares: ...
//	}
// }
func StartBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:started",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}

	spec := box.CurrentSpec()
	spec.MergeData(m)

	if err := box.Start(spec); err != nil {
		logger.Printf("ERROR: Fail to start %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	// Override public keys if they're sent in the message.
	if len(m.PublicKeys) > 0 {
		box.SetPublicKeys(m.PublicKeys)
	}
	if err := boxer.ResourcePool.UpdateState(box, "running"); err != nil {
		logger.Printf("ERROR: Fail to update box state %v, err: %v\n", box, err)
		msg.Success = false
	}
	// send back the final spec of box
	msg.Data = box.CurrentSpec().ToData()
	return msg
}

func StopBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		logger.Printf("ERROR: Fail to find box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	if err := box.Stop(); err != nil {
		logger.Printf("ERROR: Fail to stop box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:stopped",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	if err := boxer.ResourcePool.UpdateState(box, "stopped"); err != nil {
		logger.Printf("ERROR: Fail to update box state %v, err: %v\n", box, err)
		msg.Success = false
	}
	return msg
}

// Restart message will have this format
// {
//	id: boxerId,
//	type: 'aio:restart',
//	name: containerName-id,
//	returnVars: {...},
//	data: {
//	 memory: ...,
//	 virtualMemory: ...,
//	 cpuShares: ...
//	}
// }
func RestartBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		logger.Printf("ERROR: Fail to find box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	spec := box.CurrentSpec()
	spec.MergeData(m)

	if err := box.Restart(spec); err != nil {
		logger.Printf("ERROR: Fail to restart box %v, err: %v\n", box, err)
		return boxer.NewInvalidParamMessage()
	}
	msg := Message{
		Type:       "aio:restarted",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
		Data:       box.CurrentSpec().ToData(),
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
	if err := box.Destroy(); err != nil {
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
	}
	switch m.DataType {
	case "aio:configure:add_public_key":
		message.DataType = "aio:configured:add_public_key"
		data, ok := (m.Data).(string)
		if !ok {
			return boxer.NewInvalidParamMessage()
		}
		if err := box.AddPublicKey(data); err != nil {
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
		spec := box.CurrentSpec()
		message.Data = spec.ToData()
		if ok := spec.MergeData(m); !ok {
			logger.Debugln("aio:configure:change_specs: merge data", spec, m)
			message.Success = false
			return message
		}
		err := box.ChangeSpec(spec)
		message.Data = box.CurrentSpec().ToData()
		if err != nil {
			logger.Debugf("ChangeSpecs: %v\n", err)
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
	case "aio:configure:update_auth":
		var username, password string
		var ok bool
		message.DataType = "aio:configured:update_auth"
		data, ok := (m.Data).(map[string]interface{})
		if !ok {
			return boxer.NewInvalidParamMessage()
		}
		if data["auth_username"] == nil {
			username = ""
		} else {
			username, ok = data["auth_username"].(string)
			if !ok {
				return boxer.NewInvalidParamMessage()
			}
		}
		if data["auth_password"] == nil {
			password = ""
		} else {
			password, ok = data["auth_password"].(string)
			if !ok {
				return boxer.NewInvalidParamMessage()
			}
		}
		auth := BoxAuth{
			Username: username,
			Password: password,
		}
		if !auth.IsValid() {
			return boxer.NewInvalidParamMessage()
		}
		message.Success = true
		if err := box.UpdateAuth(auth); err != nil {
			message.Success = false
		}
		return message
	case "aio:configure:update_premium":
		message.DataType = "aio:configured:update_premium"
		data, ok := (m.Data).(map[string]interface{})
		if !ok {
			logger.Printf("ERROR: convert data %v, err: %v\n", data, err)
			return boxer.NewInvalidParamMessage()
		}
		premium, ok := data["premium"].(bool)
		if !ok {
			logger.Printf("ERROR: convert premium %v, err: %v\n", data, err)
			return boxer.NewInvalidParamMessage()
		}
		box.Premium = premium
		if err := box.UpdatePremiumStatus(); err != nil {
			logger.Printf("ERROR: update premium %v, err: %v\n", box, err)
			message.Success = false
			return message
		}
		if err := boxer.ResourcePool.SaveBox(box); err != nil {
			logger.Printf("ERROR: save %v, err: %v\n", box, err)
			message.Success = false
			return message
		}
		if err := boxer.Cartman.InvalidateCache(box.ContainerName()); err != nil {
			logger.Printf("ERROR: invalid cartman %v, err: %v\n", box, err)
			message.Success = false
			return message
		}
		message.Success = true
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
	}
	info, err := box.Info()
	if err != nil {
		message.Success = false
		return message
	}
	message.Info = info
	message.Success = true
	return message
}

func SnapshotBoxHandler(m Message, boxer *Boxer) Message {
	box, err := boxer.findBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage()
	}
	message := Message{
		Type:       "aio:snapshotted",
		Id:         boxer.Id,
		BoxId:      box.Id,
		SnapshotId: m.SnapshotId,
		ReturnVars: m.ReturnVars,
	}
	url, err := box.Snapshot(m.SnapshotId)
	if err != nil {
		message.Success = false
		return message
	}
	logger.Printf("DEBUG: Snapshot URL: %s", url)
	message.SnapshotUrl = url
	message.Success = true
	return message
}

// Given a list of boxes, try to start all of them.
// This should process all the boxes even when some cant be started
func (boxer *Boxer) StartAll(boxes []*Box) error {
	var errors []error
	for _, box := range boxes {
		box.boxer = boxer
		if err := box.Start(box.CurrentSpec()); err != nil {
			errors = append(errors, fmt.Errorf("Failed to start box: %s",
				box.ContainerName()))
		} else {
			// make sure that the box are in running state
			if err := boxer.ResourcePool.UpdateState(box, "running"); err != nil {
				errors = append(errors, err)
			}
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

// Wraps around ResourcePool#FindBox to set Boxer property of box and
// Boxer-dependent runtime properties.
func (boxer *Boxer) findBox(name string) (*Box, error) {
	box, err := boxer.ResourcePool.FindBox(name)
	if err != nil {
		return nil, err
	}
	box.boxer = boxer
	box.CpuQuota = boxer.BoxCPUQuota(box)
	box.CpuSet = boxer.CpuSet
	return box, nil
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

func (boxer *Boxer) BoxCPUQuota(box *Box) float64 {
	if box.Premium {
		return boxer.PremiumCPUQuota
	} else {
		return boxer.DefaultCPUQuota
	}
}
