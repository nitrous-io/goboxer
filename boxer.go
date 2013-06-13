// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package goboxer

import (
	"fmt"
	"github.com/action-io/goboxer/host"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
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
type MessageHandler func(m Message, boxer *Boxer) (Message, error)

type Boxer struct {
	Id           string
	Root         string
	SelectedDisk string
	Disks        []string
	ResourcePool *ResourcePool
	Runtimes     map[string]string
	Handles      map[string]MessageHandler
}

func (boxer *Boxer) Receive(msg Message) (Message, error) {
	handle, ok := boxer.Handles[msg.Type]
	if !ok {
		return Message{}, fmt.Errorf("Invalid Message: unknown type %s", msg.Type)
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

func LocateBoxHandler(m Message, boxer *Boxer) (Message, error) {
	return boxer.NewAdvertiseMessage(), nil
}

func CreateBoxHandler(m Message, boxer *Boxer) (Message, error) {
	msg := Message{
		Type:  "aio:created",
		Id:    boxer.Id,
		BoxId: m.BoxId,
	}
	resource, err := boxer.ResourcePool.Acquire()
	if err != nil {
		msg.Success = false
		return msg, nil
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
		return msg, nil
	}
	if err := boxer.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
		return msg, nil
	}
	msg.Port = box.Port
	msg.Success = true
	return msg, nil
}

func StartBoxHandler(m Message, boxer *Boxer) (Message, error) {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := boxer.Start(box); !err.Success() {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStart: %s", m.Name)
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
	return msg, nil
}

func StopBoxHandler(m Message, boxer *Boxer) (Message, error) {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := boxer.Stop(box); !err.Success() {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStop: %s", m.Name)
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
	return msg, nil
}

func RestartBoxHandler(m Message, boxer *Boxer) (Message, error) {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := boxer.Restart(box); !err.Success() {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToRestart: %s", m.Name)
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
	return msg, nil
}

func DestroyBoxHandler(m Message, boxer *Boxer) (Message, error) {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := boxer.Destroy(box); !err.Success() {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToDestroy: %s", m.Name)
	}
	if err := boxer.ResourcePool.Release(box.Ip); err != nil {
		return boxer.NewInvalidParamMessage(),
			fmt.Errorf("BoxFailedToReleaseResource %s with ip: %s", box.Id, box.Ip)
	}
	return Message{
		Type:       "aio:destroyed",
		Id:         boxer.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}, nil
}

func ConfigureBoxHandler(m Message, boxer *Boxer) (Message, error) {
	box, err := boxer.ResourcePool.FindBox(m.Name)
	if err != nil {
		return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
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
			return boxer.NewInvalidParamMessage(), fmt.Errorf("FailedToAddPublicKey: %s", m.Name)
		}
		message.Success = true
		return message, nil
	case "aio:configure:remove_public_key":
		data := (m.Data).(string)
		if err := box.RemovePublicKey(data); err != nil {
			return boxer.NewInvalidParamMessage(), fmt.Errorf("FailedToRemovePublicKey: %s", m.Name)
		}
		message.Success = true
		return message, nil
	case "aio:configure:change_specs":
		data := (m.Data).(map[string]interface{})
		spec := &BoxSpec{}
		spec.Memory = (data["memory"]).(string)
		spec.DiskSpace = (data["diskSpace"]).(string)
		if err := box.ChangeSpec(spec); err != nil {
			return boxer.NewInvalidParamMessage(), fmt.Errorf("FailedToChangeSpec: %s", m.Name)
		}
		if err := boxer.Restart(box); !err.Success() {
			return boxer.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStop: %s", m.Name)
		}
		message.Success = true
		box.State = "running"
		if err := boxer.ResourcePool.SaveBox(box); err != nil {
			message.Success = false
		}
		return message, nil
	}
	return boxer.NewInvalidParamMessage(), fmt.Errorf("NotHandled: %s", m.DataType)
}

func runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	if _, err := command.CombinedOutput(); err != nil {
		return err
	}
	return nil
}

// Create delta for a box
// TODO: remove boxer reference here
func boxDeltaHomePath(boxer *Boxer, box *Box) string {
	return path.Join(box.Disk, box.ContainerName()+"_delta", "rootfs",
		"home", "action")
}

func boxPath(boxer *Boxer, box *Box) string {
	return path.Join(box.Disk, box.ContainerName())
}

func CreateContainerPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	return os.MkdirAll(boxPath(boxer, box), 0755)
}

func DestroyContainerPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	return os.RemoveAll(boxPath(boxer, box))
}

func CreateContainerDeltaPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	return os.MkdirAll(boxDeltaHomePath(boxer, box), 0755)
}

func DestroyContainerDeltaPath(c *BoxCommand) error {
	box := c.Box
	target := path.Join(box.Disk, box.ContainerName()+"_delta")
	return os.RemoveAll(target)
}

// mount the container
func MountContainer(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join(box.Disk, box.ContainerName()+"_delta")
	boxPath := path.Join(box.Disk, box.ContainerName())
	baseContainerPath := path.Join(box.Disk, "base_container_template")
	return MountOverlay(boxPath, boxDeltaPath, baseContainerPath)
}

func UnmountContainer(c *BoxCommand) error {
	boxPath := path.Join(c.Box.Disk, c.Box.ContainerName())
	return UnmountOverlay(boxPath)
}

// mount the container from selected disk to lxc root
func MountContainerLxcRoot(c *BoxCommand) error {
	boxPath := path.Join(c.Box.Disk, c.Box.ContainerName())
	lxcBoxPath := path.Join(c.Boxer.Root, c.Box.ContainerName())
	return Symlink(boxPath, lxcBoxPath)
}

func UnmountContainerLxcRoot(c *BoxCommand) error {
	lxcBoxPath := path.Join(c.Boxer.Root, c.Box.ContainerName())
	return os.RemoveAll(lxcBoxPath)
}

// untar the template
func ExtractTemplate(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join(box.Disk, box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	homeDataPath := path.Join(box.Disk, box.TemplatePath())
	return Untar(homeDataPath, boxDeltaHomePath)
}

// Set box quota
func SetQuota(c *BoxCommand) error {
	return c.Box.SetQuota()
}

func UnsetQuota(c *BoxCommand) error {
	return c.Box.UnsetQuota()
}

// set lxc_config
func SetLxcConfig(c *BoxCommand) error {
	return c.Box.WriteConfig()
}

// set hostname
func SetHostname(c *BoxCommand) error {
	box := c.Box
	hostNamePath := path.Join(box.RootFS(), "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.Write([]byte(box.ContainerName()))
	return err
}

// set network interface
func SetNetworkConfig(c *BoxCommand) error {
	box := c.Box
	networkConfigPath := path.Join(box.RootFS(), "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, box)
}

// set uid
func SetUserUid(c *BoxCommand) error {
	box := c.Box
	return runCommand("chroot", box.RootFS(), "usermod", "-u", box.Uid(), "action")
}

func SetUserGid(c *BoxCommand) error {
	box := c.Box
	return runCommand("chroot", box.RootFS(), "groupmod", "-g", box.Gid(), "action")
}

// set gid
func SetHomePermission(c *BoxCommand) error {
	box := c.Box
	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	return runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action")
}

// create workspace
func SetWorkspace(c *BoxCommand) error {
	box := c.Box
	if err := os.MkdirAll(path.Join(box.RootFS(), "home", "action",
		"workspace"), 0755); err != nil {
		return err
	}
	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	return runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action/workspace")
}

// set /etc/action/box_id
func SetBoxId(c *BoxCommand) error {
	box := c.Box
	boxIdFile, err := os.Create(box.BoxIdFile())
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	_, err = boxIdFile.Write([]byte(box.Id))
	return err
}

func UnsetBoxId(c *BoxCommand) error {
	return os.RemoveAll(c.Box.BoxIdFile())
}

// setup port forwarding
func SetPortForwarding(c *BoxCommand) error {
	box := c.Box
	runCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", box.Port, "-j", "DNAT", "--to",
		box.Ip+":22")
	return nil
}

func UnsetPortForwarding(c *BoxCommand) error {
	box := c.Box
	runCommand("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
		"--dport", box.Port, "-j", "DNAT", "--to", box.Ip+":22")
	return nil
}

// setup default key pair
func SetDefaultKeyPair(c *BoxCommand) error {
	box := c.Box
	// chroot and run ssh-keygen
	sshPath := path.Join(c.Box.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(c.Box.RootFS(), c.Box.PrivateKeyPath())
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		if err := runCommand("/bin/sh", "-c", sshKeygen); err != nil {
			return err
		}
	} else {
		return err
	}

	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	return runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action/.ssh")
}

// Set Public Keys
func SetPublicKeys(c *BoxCommand) error {
	return c.Box.AddPublicKey(c.Box.PublicKeys...)
}

func UnsetPublicKeys(c *BoxCommand) error {
	box := c.Box
	sshPath := path.Join(box.RootFS(), "etc", "ssh", "univ_public_keys")
	return ioutil.WriteFile(sshPath, []byte{}, 0644)
}

// TODO: improve how we are handling lxc operations. So inefficient to have to
// wait like this!
func waitLxc(name, status string) error {
	return runCommand("lxc-wait", "-n", name, "-s", status)
}

// Stop Container
func StopContainer(c *BoxCommand) error {
	box := c.Box
	err := runCommand("lxc-stop", "-n", box.ContainerName())
	if err != nil {
		return err
	}
	return waitLxc(box.ContainerName(), "STOPPED")
}

func StartContainer(c *BoxCommand) error {
	box := c.Box
	err := runCommand("lxc-start", "-d", "-n", box.ContainerName())
	if err != nil {
		return err
	}
	return waitLxc(box.ContainerName(), "RUNNING")
}

// Default functionw when you want to continue over to the next state
func Pass(c *BoxCommand) error {
	return nil
}

var CreateStates []State
var StartStates []State
var StopStates []State
var RestartStates []State
var DestroyStates []State

func init() {
	CreateStates = []State{
		State{Advance: CreateContainerPath, Rollback: DestroyContainerPath},
		State{Advance: CreateContainerDeltaPath, Rollback: DestroyContainerDeltaPath},
		State{Advance: MountContainer, Rollback: UnmountContainer},
		State{Advance: MountContainerLxcRoot, Rollback: UnmountContainerLxcRoot},
		State{Advance: ExtractTemplate, Rollback: Pass},
		State{Advance: SetQuota, Rollback: UnsetQuota},
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
	}

	StartStates = []State{
		State{Advance: MountContainer, Rollback: UnmountContainer},
		State{Advance: MountContainerLxcRoot, Rollback: UnmountContainerLxcRoot},
		State{Advance: StartContainer, Rollback: StopContainer},
	}

	StopStates = []State{
		State{Advance: StopContainer, Rollback: Pass},
		State{Advance: UnmountContainerLxcRoot, Rollback: MountContainerLxcRoot},
		State{Advance: UnmountContainer, Rollback: MountContainer},
		State{Advance: UnsetQuota, Rollback: SetQuota},
	}

	DestroyStates = []State{
		State{Advance: StopContainer, Rollback: Pass},
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
		State{Advance: UnmountContainerLxcRoot, Rollback: MountContainerLxcRoot},
		State{Advance: UnmountContainer, Rollback: MountContainer},
		State{Advance: UnsetQuota, Rollback: SetQuota},

		State{Advance: DestroyContainerDeltaPath, Rollback: CreateContainerDeltaPath},
		State{Advance: DestroyContainerPath, Rollback: CreateContainerPath},
	}

	RestartStates = []State{
		State{Advance: StopContainer, Rollback: Pass},
		State{Advance: UnmountContainer, Rollback: MountContainer},
		State{Advance: MountContainer, Rollback: UnmountContainer},
		State{Advance: StartContainer, Rollback: StopContainer},
	}
}

func (boxer *Boxer) Create(box *Box) *CommandError {
	createCommand := &BoxCommand{
		Name:         "Create",
		Boxer:        boxer,
		Box:          box,
		CurrentIndex: 0,
		States:       CreateStates,
	}
	return Advance(createCommand)
}

func (boxer *Boxer) Start(box *Box) *CommandError {
	startCommand := &BoxCommand{
		Name:   "Start",
		Boxer:  boxer,
		Box:    box,
		States: StartStates,
	}
	return Advance(startCommand)
}

func (boxer *Boxer) Restart(box *Box) *CommandError {
	restartCommand := &BoxCommand{
		Name:   "Restart",
		Boxer:  boxer,
		Box:    box,
		States: RestartStates,
	}
	return Advance(restartCommand)
}

func (boxer *Boxer) Stop(box *Box) *CommandError {
	stopCommand := &BoxCommand{
		Name:   "Stop",
		Boxer:  boxer,
		Box:    box,
		States: StopStates,
	}
	return Advance(stopCommand)
}

func (boxer *Boxer) Destroy(box *Box) *CommandError {
	destroyCommand := &BoxCommand{
		Name:   "Destroy",
		Boxer:  boxer,
		Box:    box,
		States: DestroyStates,
	}
	return Advance(destroyCommand)
}
