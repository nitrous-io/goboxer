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
	"text/template"
	"time"
)

const UidStart = 15000
const DOCKER_BIN = "docker"

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
	DockerSocket     string   `json:"dockerSocket"`

	// hold a reference to boxer so that we can access mutex capabilities
	boxer *Boxer
}

func (b *Box) ContainerName() string {
	return fmt.Sprintf("%s-%s", b.Name, b.Id)
}

func (b *Box) Home() string {
	containerPath, _ := b.DockerContainerPath()
	return path.Join(containerPath, "rootfs", "home", "action")
}

func (b *Box) TarName() string {
	return fmt.Sprintf("%s-%v.tar.gz", b.ContainerName(), time.Now().Unix())
}

func (b *Box) EnvPath() string {
	containerPath, _ := b.DockerContainerPath()
	return path.Join(containerPath, "rootfs", "etc", "environment")
}

func (b *Box) PublicKeysPath() string {
	containerPath, _ := b.DockerContainerPath()
	return path.Join(containerPath, "rootfs", "etc", "ssh", "univ_public_keys")
}

func (b *Box) Uid() string {
	id, _ := strconv.Atoi(b.Id)
	return strconv.Itoa(UidStart + id)
}

func (b *Box) Gid() string {
	id, _ := strconv.Atoi(b.Id)
	return strconv.Itoa(UidStart + id)
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

func (b *Box) SetQuota(q quota.QuotaInterface) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	defer logger.LogDuration(
		fmt.Sprintf("%s:SetQuota", b.ContainerName()),
	)(time.Now())
	return q.SetLimit(b.Id, b.DockerRoot(), b.DiskSpace)
}

func (b *Box) UnsetQuota(q quota.QuotaInterface) error {
	b.boxer.Lock()
	defer b.boxer.Unlock()
	return q.RemoveLimit(b.Id, b.DockerRoot())
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

// Return current memory usage of the box as memory cgroup reported it
// the unit if B, so if we want to convert to MB, it needs to be divided with
// 1024 * 1024
func (b *Box) MemoryUsageInBytes(container dockerContainer) (m int, err error) {
	var file *os.File
	var filename string
	filename = fmt.Sprintf(
		"/sys/fs/cgroup/memory/sysdefault/lxc/%s/memory.usage_in_bytes",
		container.Id)
	file, err = os.Open(filename)
	// falback to old default location if we cant find cgroup data
	if os.IsNotExist(err) {
		filename = fmt.Sprintf(
			"/sys/fs/cgroup/memory/lxc/%s/memory.usage_in_bytes",
			container.Id)
		file, err = os.Open(filename)
	}
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
	quotaInfo, err := q.ViewLimit(b.Id, b.DockerRoot())
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

func (b *Box) Image() string { return "docker-registry.nitrous.io/nitrous/" + b.Id }

func (b *Box) CreateImage() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:CreateImage", b.Image()),
	)(time.Now())
	var stdinPipe io.WriteCloser
	cmd := b.docker("build", "-t", b.Image(), "-")
	var b2 bytes.Buffer
	cmd.Stdout = &b2
	if stdinPipe, err = cmd.StdinPipe(); err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		logger.Println("start", err)
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

// download the content from s3 and perform the migrate. The end result should
// be a running box
func (b *Box) Migrate(auth s3.AuthInterface) (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:Migrate", b.Image()),
	)(time.Now())
	if b.SnapshotURL == "" {
		return fmt.Errorf("Missing Snapshot: %v", b)
	}
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
		return
	}
	return b.CreateFromSnapshot(stream)
}

// instead of usual image creation routing, we are going to create the image
// given a snapshot tar file.
// How we are going to do it
// - run a container off the base image
// - untar the content into the container layer
// - commit the container changes into a new image
// - run all necessary stuff to make the image usable (i.e. permission etc)
// - use that image
// - ...
// - profit
func (b *Box) CreateFromSnapshot(stream io.ReadCloser) (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:CreateFromSnapshot", b.Image()),
	)(time.Now())

	// create a container from the base image, and not from
	// docker_template
	id, err := createContainer(b, b.boxer.Runtimes["base"])
	if err != nil {
		logger.Println("fail to retrieve id", id, err)
		return
	}
	b.DockerId = id
	// untar the stream to home directory of user
	if err = UntarStream(stream, b.Home()); err != nil {
		return
	}
	// commit the content and remove the current container
	if err = b.CommitContainer(); err != nil {
		return
	}
	if err = b.DestroyContainer(); err != nil {
		return
	}
	// create a new image from the current image. this is where we make sure
	// that everything post-untar is correct
	createImage(b, MigrateDockerfileCompiled)
	// create a new container from the new image
	if err = b.CreateContainer(); err != nil {
		return
	}
	// set quota for the box. This should only works with usrquota for now
	b.SetQuota(b.boxer.Quota)
	// start the new container
	if err = b.StartContainer(); err != nil {
		return
	}
	return
}

func createImage(b *Box, dockerFile *template.Template) (err error) {
	var stdinPipe io.WriteCloser
	cmd := b.docker("build", "-t", b.Image(), "-")
	var b2 bytes.Buffer
	cmd.Stdout = &b2
	if stdinPipe, err = cmd.StdinPipe(); err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		return
	}
	if err = dockerFile.Execute(stdinPipe, b); err != nil {
		logger.Println(err)
		return
	}
	stdinPipe.Close()
	err = cmd.Wait()
	io.Copy(os.Stdout, &b2)
	return
}

func createContainer(b *Box, base string) (dockerId string, err error) {
	port := fmt.Sprintf("%d:22", b.Port)
	// because of legacy reeason, we pass memory as 256M,this value has to be
	// converted to byte value here before so that we can passed it to docker
	cmd := b.docker("run", "-d",
		"-p", port,
		"-m", b.MemoryAsByte(),
		"-h", b.ContainerName(),
		base, "/usr/sbin/sshd", "-D")
	return ToString(cmd.CombinedOutput())
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
	cmd := b.docker("run", "-d",
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
	return b.UpdateState("running")
}

// dont complain if the container is running
func (b *Box) StartContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:StartContainer", b.ContainerName()),
	)(time.Now())
	if b.State == "running" {
		return
	}
	output, err := ToString(b.docker("start", b.DockerId).CombinedOutput())
	if err != nil && !strings.Contains(output, "is already running") {
		return
	}
	return b.UpdateState("running")
}

func (b *Box) StopContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:StopContainer", b.ContainerName()),
	)(time.Now())
	if b.State == "stopped" {
		return
	}
	if err = b.docker("stop", b.DockerId).Run(); err != nil {
		return
	}
	return b.UpdateState("stopped")
}

func (b *Box) RestartContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:RestartContainer", b.ContainerName()),
	)(time.Now())
	return b.docker("restart", b.DockerId).Run()
}

func (b *Box) DestroyContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:DestroyContainer", b.ContainerName()),
	)(time.Now())
	err = b.StopContainer()
	if err != nil {
		return
	}
	return b.docker("rm", b.DockerId).Run()
}

// commit changes in container into the base image
func (b *Box) CommitContainer() (err error) {
	defer logger.LogDuration(
		fmt.Sprintf("%s:CommitContainer", b.ContainerName()),
	)(time.Now())
	return b.docker("commit", "-m", fmt.Sprintf("version: %v", time.Now()),
		b.DockerId, b.Image()).Run()
}

// make sure that the current state of box is persisted to resource pool
func (b *Box) Persist() error {
	if len(b.State) == 0 {
		return fmt.Errorf("BoxStateNotFound: %v", b)
	}
	dockerInfo, err := b.dockerInfo()
	if err != nil {
		return fmt.Errorf("DockerInfoNotFound: err %v", err)
	}
	b.Ip = dockerInfo.NetworkSettings.IPAddress
	b.Gateway = dockerInfo.NetworkSettings.Gateway
	b.NetworkInterface = dockerInfo.NetworkSettings.Bridge
	return b.boxer.ResourcePool.SaveBox(b)
}

func (b *Box) UpdateState(state string) error {
	b.State = state
	return b.Persist()
}

type dockerPortMap map[string]string

type dockerContainer struct {
	Id    string `json:"id"`
	State struct {
		Running bool
	}
	NetworkSettings struct {
		IPAddress   string
		IPPrefixLen int
		Gateway     string
		Bridge      string
		PortMapping map[string]dockerPortMap
	}
}

type dockerCollection struct {
	Containers []dockerContainer
}

func (c dockerContainer) Path(prefix string) string {
	return path.Join(prefix, c.Id)
}

// Use this only when you have to inspect the content of the running container
// i.e. usually in test
func (b *Box) DockerContainerPath() (p string, err error) {
	c, err := b.dockerInfo()
	if err != nil {
		return
	}
	p = c.Path(path.Join(b.DockerRoot(), "containers"))
	return
}

// used to debug docker state
func (b *Box) dockerInfo() (c dockerContainer, err error) {
	inspectOutput, err := b.docker("inspect", b.DockerId).CombinedOutput()
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

func (b *Box) DockerRoot() string {
	return b.boxer.DockerDisks[b.socket()]
}

// convenience method to run docker cmd
func (b *Box) docker(args ...string) *exec.Cmd {
	return docker(b.socket(), args...)
}

// return the docker socket that the box should talk to
func (b *Box) socket() string {
	if len(b.DockerSocket) > 0 {
		return b.DockerSocket
	}
	return b.boxer.DockerSocket
}

// given a docker socket, run docker commands with this socket and not use the
// default socket
func docker(socket string, args ...string) *exec.Cmd {
	dockerArgs := []string{"-H", "unix://" + socket}
	dockerArgs = append(dockerArgs, args...)
	return exec.Command(DOCKER_BIN, dockerArgs...)
}
