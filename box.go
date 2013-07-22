package goboxer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/s3"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

const UidStart = 15000
const DOCKER_BIN = "/home/action/gocode/bin/docker"
const DOCKER_DISK = "/var/lib/docker"
const DOCKER_CONTAINER_PATH = "/var/lib/docker/containers"

type BoxSpec struct {
	Memory    string `json:"memory,omitempty"`
	DiskSpace string `json:"diskSpace,omitempty"`
}

type BoxInfo struct {
	StorageUsage float64 `json:"storageUsage,omitempty"`
	MemoryUsage  float64 `json:"memoryUsage,omitempty"`
	LxcStatus    string  `json:"lxcStatus,omitempty"`
}

type Box struct {
	Id               string   `json:"id"`
	DockerId         string   `json:"docker_id"`
	Name             string   `json:"name"`
	Template         string   `json:"containerTemplate"`
	Memory           string   `json:"memory"`
	DiskSpace        string   `json:"diskSpace"`
	Ip               string   `json:"ip"`
	Gateway          string   `json:"gateway"`
	NetworkInterface string   `json:"interface"`
	Port             int      `json:"port"`
	BoxerId          string   `json:"boxerId"`
	State            string   `json:"containerState"`
	Disk             string   `json:"disk"`
	Runtime          string   `json:"-"`
	SnapshotURL      string   `json:"-"`
	PublicKeys       []string `json:"-"`

	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
}

func (b *Box) ContainerName() string { return fmt.Sprintf("%s-%s", b.Name, b.Id) }
func (b *Box) SetPublicKeys() error  { return b.AddPublicKey(b.PublicKeys...) }

func (b *Box) BoxIdFile() string      { return path.Join(b.RootFS(), "etc", "action", "box_id") }
func (b *Box) PrivateKeyPath() string { return path.Join("home", "action", ".ssh", "id_rsa") }

func (b *Box) Home() string { return path.Join(b.RootFS(), "home", "action") }

func (b *Box) TarName() string {
	return fmt.Sprintf("%s-%v.tar.gz", b.ContainerName(), time.Now().Unix())
}

func (b *Box) RootFS() string      { return path.Join(DOCKER_CONTAINER_PATH, b.ContainerName(), "rootfs") }
func (b *Box) ContainerFS() string { return path.Join(b.Disk, b.ContainerName()) }

func (b *Box) EnvPath() string {
	containerPath, _ := b.DockerContainerPath()
	return path.Join(containerPath, "rootfs", "etc", "environment")
}

func (b *Box) PublicKeysPath() string {
	containerPath, _ := b.DockerContainerPath()
	return path.Join(containerPath, "rootfs", "etc", "ssh", "univ_public_keys")
}

func (b *Box) ContainerDeltaPath() string {
	return path.Join(b.Disk, b.ContainerName()+"_delta")
}

func (b *Box) Uid() string {
	id, _ := strconv.Atoi(b.Id)
	return strconv.Itoa(UidStart + id)
}

func (b *Box) Gid() string {
	id, _ := strconv.Atoi(b.Id)
	return strconv.Itoa(UidStart + id)
}

// http://www.funtoo.org/wiki/Linux_Containers
// http://serverfault.com/questions/40712/what-range-of-mac-addresses-can-i-safely-use-for-my-virtual-machines
func (b *Box) MacAddress() string {
	ips := strings.Split(b.Ip, ".")
	ipList := make([]interface{}, len(ips))
	for i, v := range ips {
		val, _ := strconv.Atoi(v)
		ipList[i] = interface{}(val)
	}
	return fmt.Sprintf("02:00:%x:%x:%x:%x", ipList...)
}

func (b *Box) TotalMemory() string {
	m := quota.ParseValueUnit(b.Memory)
	return fmt.Sprintf("%d%s", int(m.Value)*3, m.Unit)
}

func (b *Box) AddPublicKey(keys ...string) error {
	return Appendln(b.PublicKeysPath(), strings.Join(keys, "\n"))
}

func (b *Box) RemovePublicKey(key string) error {
	return DeleteLineContains(b.PublicKeysPath(), key)
}

func (b *Box) HomePath() string {
	return path.Join(b.Disk, b.ContainerName()+"_delta", "rootfs", "home", "action")
}

func (b *Box) SetQuota(q quota.QuotaInterface) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetQuota", b.ContainerName()),
	)(time.Now())
	return q.SetLimit(b.Id, b.ContainerName(), b.HomePath(),
		DOCKER_DISK, b.DiskSpace)
}

func (b *Box) UnsetQuota(q quota.QuotaInterface) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	return q.RemoveLimit(b.Id, b.ContainerName(), b.HomePath(),
		DOCKER_DISK)
}

// valid data, start changing specs by
// - stop the container
// - commit all changes of the container into a new image
// - destroy the current container
// - create a new container based on the new image with new settings
// - start the new container
// - update redis pool
func (b *Box) ChangeSpec(spec *BoxSpec, q quota.QuotaInterface) error {
	if spec.Memory != b.Memory {
		b.Memory = spec.Memory
	}
	if err := b.StopContainer(); err != nil {
		return err
	}
	if err := b.CommitContainer(); err != nil {
		return err
	}
	if err := b.DestroyContainer(); err != nil {
		return err
	}
	if err := b.CreateContainer(); err != nil {
		return err
	}
	if err := b.StartContainer(); err != nil {
		logger.Debugln("what", err)
		return err
	}
	if spec.DiskSpace != b.DiskSpace {
		oldDiskSpace := b.DiskSpace
		b.DiskSpace = spec.DiskSpace
		if err := b.SetQuota(q); err != nil {
			b.DiskSpace = oldDiskSpace
			return err
		}
	}
	return nil
}

func (b *Box) WriteConfig() error {
	lxcConfigPath := path.Join(b.Disk, b.ContainerName(), "config")
	lxcConfig, err := os.Create(lxcConfigPath)
	if err != nil {
		return err
	}
	defer lxcConfig.Close()
	return LxcTemplateCompiled.Execute(lxcConfig, b)
}

func (b *Box) SetBoxId() error {
	boxIdFile, err := os.Create(b.BoxIdFile())
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	_, err = boxIdFile.Write([]byte(b.Id))
	return err
}

func (b *Box) SetNetworkConfig() error {
	networkConfigPath := path.Join(b.RootFS(), "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, b)
}

func (b *Box) SetHostname() error {
	hostNamePath := path.Join(b.RootFS(), "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.Write([]byte(b.ContainerName()))
	return err
}

func (b *Box) SetPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	return RunCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", strPort, "-j", "DNAT", "--to",
		b.Ip+":22")
}

func (b *Box) UnsetPortForwarding() error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	strPort := strconv.Itoa(b.Port)
	return RunCommand("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
		"--dport", strPort, "-j", "DNAT", "--to", b.Ip+":22")
}

func (b *Box) SetDefaultKeypair() error {
	sshPath := path.Join(b.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(b.RootFS(), b.PrivateKeyPath())
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		if err := RunCommand("/bin/sh", "-c", sshKeygen); err != nil {
			return err
		}
	} else {
		return err
	}

	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	return RunCommand("chroot", b.RootFS(), "chown", "-R", permission,
		"/home/action/.ssh")
}

func (b *Box) Mount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:Mount", b.ContainerName()),
	)(time.Now())

	boxPath := b.ContainerFS()
	boxDeltaPath := b.ContainerDeltaPath()
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	baseContainerPath := path.Join(b.Disk, "base_container_template")
	lxcBoxPath := path.Join(root, b.ContainerName())

	if err := os.MkdirAll(b.ContainerFS(), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(boxDeltaHomePath, 0755); err != nil {
		return err
	}
	if err := MountOverlay(boxPath, boxDeltaPath, baseContainerPath); err != nil {
		return err
	}
	if err := os.RemoveAll(lxcBoxPath); err != nil {
		return err
	}
	return Symlink(boxPath, lxcBoxPath)
}

func (b *Box) Unmount(root string) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	lxcBoxPath := path.Join(root, b.ContainerName())
	if err := os.RemoveAll(lxcBoxPath); err != nil {
		return err
	}
	return UnmountOverlay(b.ContainerFS())
}

func (b *Box) DeleteContainerData() error {
	if err := os.RemoveAll(b.ContainerDeltaPath()); err != nil {
		return err
	}
	return os.RemoveAll(b.ContainerFS())
}

// Return current memory usage of the box as memory cgroup reported it
// the unit if B, so if we want to convert to MB, it needs to be divided with
// 1024 * 1024
func (b *Box) MemoryUsageInBytes(container dockerContainer) (m int, err error) {
	filename := fmt.Sprintf("/sys/fs/cgroup/memory/lxc/%s/memory.usage_in_bytes",
		container.Id)
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(file)
	scanner.Scan()
	if err = scanner.Err(); err != nil {
		return
	}
	return strconv.Atoi(scanner.Text())
}

// Return current state of the box. It is the string that lxc used to annotate
// container's state such as
// - RUNNING
// - STOPPED
func (b *Box) LxcState(container dockerContainer) string {
	if container.State.Running {
		return "RUNNING"
	} else {
		return "STOPPED"
	}
}

// Return current storage usage in kb
func (b *Box) StorageUsage(q quota.QuotaInterface) (int, error) {
	quotaInfo, err := q.ViewLimit(b.Id, DOCKER_DISK)
	if err != nil {
		return 0, err
	}
	return quotaInfo.Used, nil
}

// Return BoxInfo with `MB` units
func (b *Box) Info(q quota.QuotaInterface) (info BoxInfo, err error) {
	container, err := b.dockerInfo()
	if err != nil {
		return
	}
	memoryUsage, err := b.MemoryUsageInBytes(container)
	if err != nil {
		return
	} else {
		info.MemoryUsage = float64(memoryUsage) / (1024 * 1024)
	}
	info.LxcStatus = b.LxcState(container)
	storageUsage, err := b.StorageUsage(q)
	if err != nil {
		return
	} else {
		info.StorageUsage = quota.ScaleUnit(storageUsage, "M")
	}
	return
}

// set env of the box
// This set env as KEY=value lines into /etc/environment of the box, which
// should be loaded automatically when environment starts
func (b *Box) SetEnv(data map[string]interface{}) error {
	newEnv := []string{}
	for key, value := range data {
		// FIXME: probably going through the file once and remove all is faster
		if err := DeleteLineContains(b.EnvPath(), key+"="); err != nil {
			return err
		}
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", key, value.(string)))
	}
	return Appendln(b.EnvPath(), strings.Join(newEnv, "\n"))
}

// Remove env variables from the box
func (b *Box) RemoveEnv(data []string) (err error) {
	for _, value := range data {
		if err = DeleteLineContains(b.EnvPath(), value+"="); err != nil {
			return err
		}
	}
	return
}

// tar the box delta content.
// This returns the path that we get the compress content, and any errors that
// we had during the compression
func (b Box) Tar() (compressedPath string, err error) {
	return Tar(b.TarName(), b.Home())
}

// if the box has SnapshotURL, then we use that to populate the home directory,
// otherwise, we use Runtime to get a base on which we do more interesting
// things such as setting up keys, workspace etc
func (b Box) SetupHome(auth s3.AuthInterface) error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetupHome", b.ContainerName()),
	)(time.Now())

	if len(b.SnapshotURL) > 0 {
		// parse snapshot url which should be an s3 path
		url, err := s3.ParseUrl(b.SnapshotURL)
		if err != nil {
			return err
		}
		client, err := auth.NewClient(url.Region, url.Bucket)
		if err != nil {
			return err
		}
		stream, err := client.Download(url.File)
		if err != nil {
			return err
		}
		boxDeltaHomePath := path.Join(b.ContainerDeltaPath(), "rootfs", "home", "action")
		if err := UntarStream(stream, boxDeltaHomePath); err != nil {
			return nil
		}
	} else {
		templatePath := b.Template + ".tar.gz"
		boxDeltaHomePath := path.Join(b.ContainerDeltaPath(), "rootfs", "home", "action")
		homeDataPath := path.Join(b.Disk, templatePath)
		if err := Untar(homeDataPath, boxDeltaHomePath); err != nil {
			return err
		}
		if err := b.setWorkspace(); err != nil {
			return err
		}
		if err := b.setDefaultKeypair(); err != nil {
			return err
		}
	}
	return b.setPermission()
}

// set correct permission to user home folder.
// This is necessary because the permission comes with overlay folders are
// incorrect
func (b *Box) setPermission() error {
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetPermission", b.ContainerName()),
	)(time.Now())
	uid := b.Uid()
	gid := b.Gid()
	permission := fmt.Sprintf("%s:%s", b.Uid(), b.Gid())
	if err := RunCommand("chroot", b.RootFS(), "usermod", "-u", uid, "action"); err != nil {
		return err
	}
	if err := RunCommand("chroot", b.RootFS(), "groupmod", "-g", gid, "action"); err != nil {
		return err
	}
	return RunCommand("chroot", b.RootFS(), "chown", "-R", permission, "/home/action")
}

// create a workspace folder for home directory
func (b *Box) setWorkspace() error {
	workspacePath := path.Join(b.RootFS(), "home", "action", "workspace")
	return os.MkdirAll(workspacePath, 0755)
}

// set default keypair for a new box. This generally should only be called when
// the box is newly setup from a preconfigured runtime
func (b *Box) setDefaultKeypair() error {
	sshPath := path.Join(b.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(b.RootFS(), b.PrivateKeyPath())
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		return RunCommand("/bin/sh", "-c", sshKeygen)
	} else {
		return err
	}
}

func (b *Box) Image() string { return "nitrous/" + b.Id }

func (b *Box) BaseImage() string { return b.Template }

func (b *Box) PublicKeysStr() string { return b.PublicKeys[0] }

func (b *Box) CreateImage() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:CreateImage", b.Image()),
	)(time.Now())
	var stdinPipe io.WriteCloser
	cmd := exec.Command(DOCKER_BIN, "build", "-t", b.Image(), "-")
	var b2 bytes.Buffer
	cmd.Stdout = &b2
	if stdinPipe, err = cmd.StdinPipe(); err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		return
	}
	if err = DockerfileCompiled.Execute(stdinPipe, b); err != nil {
		logger.Println(err)
		return
	}
	stdinPipe.Close()
	err = cmd.Wait()
	io.Copy(os.Stdout, &b2)
	return
}

func (b *Box) MemoryAsByte() string {
	memory := quota.ConvertValueUnitToByte(quota.ParseValueUnit(b.Memory)) * 1024
	return strconv.Itoa(memory)
}

// create container and associate a docker id with it
func (b *Box) CreateContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:CreateContainer", b.Image()),
	)(time.Now())
	port := fmt.Sprintf("%d:22", b.Port)
	// because of legacy reeason, we pass memory as 256M,this value has to be
	// converted to byte value here before so that we can passed it to docker
	cmd := exec.Command(DOCKER_BIN, "run", "-d",
		"-p", port,
		"-m", b.MemoryAsByte(),
		"-h", b.ContainerName(),
		b.Image(), "/usr/sbin/sshd", "-D")
	id, err := ToString(cmd.CombinedOutput())
	if err != nil {
		logger.Println("fail to retrieve id", id, err)
		return
	}
	b.DockerId = id
	// docker id changes here, we will save the new data into redis
	return b.boxer.ResourcePool.SaveBox(b)
}

// dont complain if the container is running
func (b *Box) StartContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:StartContainer", b.ContainerName()),
	)(time.Now())
	if b.State == "running" {
		return
	}
	output, err := ToString(exec.Command(DOCKER_BIN, "start",
		b.DockerId).CombinedOutput())
	if err != nil && !strings.Contains(output, "is already running") {
		return
	}
	return b.boxer.ResourcePool.UpdateState(b, "running")
}

func (b *Box) StopContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:StopContainer", b.ContainerName()),
	)(time.Now())
	if b.State == "stopped" {
		return
	}
	if err = exec.Command(DOCKER_BIN, "stop", b.DockerId).Run(); err != nil {
		return
	}
	return b.boxer.ResourcePool.UpdateState(b, "stopped")
}

func (b *Box) RestartContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:RestartContainer", b.ContainerName()),
	)(time.Now())
	return exec.Command(DOCKER_BIN, "restart",
		b.DockerId).Run()
}

func (b *Box) DestroyContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:DestroyContainer", b.ContainerName()),
	)(time.Now())
	err = b.StopContainer()
	if err != nil {
		return
	}
	return exec.Command(DOCKER_BIN, "rm", b.DockerId).Run()
}

// commit changes in container into the base image
func (b *Box) CommitContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:CommitContainer", b.ContainerName()),
	)(time.Now())
	return exec.Command(DOCKER_BIN, "commit",
		"-m", fmt.Sprintf("version: %v", time.Now()),
		b.DockerId).Run()
}

type dockerContainer struct {
	Id    string `json:"id"`
	State struct {
		Running bool
	}
}

type dockerCollection struct {
	Containers []dockerContainer
}

func (c dockerContainer) Path() string {
	return path.Join(DOCKER_CONTAINER_PATH, c.Id)
}

// Use this only when you have to inspect the content of the running container
// i.e. usually in test
func (b *Box) DockerContainerPath() (p string, err error) {
	c, err := b.dockerInfo()
	if err != nil {
		return
	}
	p = c.Path()
	return
}

// used to debug docker state
func (b *Box) dockerInfo() (c dockerContainer, err error) {
	inspectOutput, err := exec.Command(DOCKER_BIN, "inspect",
		b.DockerId).CombinedOutput()
	if err != nil {
		return
	}
	collection := &dockerCollection{}
	err = json.Unmarshal(inspectOutput, &collection.Containers)
	if err != nil {
		return
	}
	c = collection.Containers[0]
	return
}
