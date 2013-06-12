// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package goboxer

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
)

type Boxer struct {
	Root         string
	SelectedDisk string
	Disks        []string
	Runtimes     map[string]string
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

// set /etc/projects
func SetEtcProjects(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join(box.Disk, box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	projectsFile, err := os.OpenFile("/etc/projects", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer projectsFile.Close()
	projectLine := box.Id + ":" + boxDeltaHomePath
	_, err = projectsFile.Write([]byte(projectLine))
	return err
}

func UnsetEtcProjects(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join(box.Disk, box.ContainerName()+"_delta")
	return DeleteLineContains("/etc/projects", boxDeltaPath)
}

// set /etc/projid
func SetEtcProjid(c *BoxCommand) error {
	box := c.Box
	projidFile, err := os.OpenFile("/etc/projid", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer projidFile.Close()
	projidLine := box.ContainerName() + ":" + box.Id
	_, err = projidFile.Write([]byte(projidLine))
	return err
}

func UnsetEtcProjid(c *BoxCommand) error {
	box := c.Box
	projidLine := box.ContainerName() + ":" + box.Id
	return DeleteLineContains("/etc/projid", projidLine)
}

// set xfs_quota
func SetXfsQuota(c *BoxCommand) error {
	box := c.Box
	runCommand("xfs_quota", "-x", "-c", "project -s "+box.ContainerName(),
		box.Disk)
	runCommand("xfs_quota", "-x", "-c",
		"limit -p bhard="+box.DiskSpace+" "+box.ContainerName(),
		box.Disk)
	return nil
}

func UnsetXfsQuota(c *BoxCommand) error {
	box := c.Box
	runCommand("xfs_quota", "-x", "-c",
		"\"limit -p bhard=0 "+box.ContainerName()+"\"", box.Disk)
	return nil
}

// set lxc_config
func SetLxcConfig(c *BoxCommand) error {
	box := c.Box
	lxcConfigPath := path.Join(box.Disk, box.ContainerName(), "config")
	lxcConfig, err := os.Create(lxcConfigPath)
	if err != nil {
		return err
	}
	defer lxcConfig.Close()
	return LxcTemplateCompiled.Execute(lxcConfig, box)
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
		State{Advance: UnsetEtcProjects, Rollback: SetEtcProjects},
		State{Advance: UnsetEtcProjid, Rollback: SetEtcProjid},
		State{Advance: UnsetXfsQuota, Rollback: SetXfsQuota},
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
		State{Advance: UnsetXfsQuota, Rollback: SetXfsQuota},
		State{Advance: UnsetEtcProjid, Rollback: SetEtcProjid},
		State{Advance: UnsetEtcProjects, Rollback: SetEtcProjects},

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
