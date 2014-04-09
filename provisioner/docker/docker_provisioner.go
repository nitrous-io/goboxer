// docker provisioner
package docker

import (
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/action-io/goboxer/mount"
	pr "github.com/action-io/goboxer/provisioner"
)

const DOCKER_PATH = "/var/lib/docker"   // where docker stores its data
const ROOT_DIR = "/var/lib/docker/aufs" // where container's data is stored
const BASE_IMAGE = "nitrous/base_box"
const STOP_TIMEOUT = 30
const STOP_TIMEOUT_STR = "30"

func init() {
	pr.Register("docker", Init)
}

type Provisioner struct {
	root string
}

func generateId() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// - try to start the id first in case it's a running container
// This is useful when we want runContainer to be idempotent when passing the
// same id multiple times.
// - run the image as a new container with provided name
// returns the container id
func runContainer(id, name string) (string, error) {
	output, err := exec.Command("docker", "start", id).CombinedOutput()
	if err != nil {
		// this is probably an image, just create a new container
		if strings.Contains(string(output), "No such container") {
			args := []string{
				"run", "-d", "-name", name + "-" + generateId(), id,
			}
			output, err = exec.Command("docker", args...).Output()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(output)), nil
		}
		// if the container is already running, just return the id
		if strings.Contains(string(output), "is already running") {
			return id, nil
		}
		return "", err
	}
	return id, nil
}

// commit and return the image id
func commitContainer(id string) (string, error) {
	args := []string{
		"commit", id,
	}
	output, err := exec.Command("docker", args...).Output()
	if err != nil {
		return "", &pr.OpError{Op: "commit container", Err: err}
	}
	return strings.TrimSpace(string(output)), nil
}

// - stop the container
// - commit it as a new image
func stopContainer(id string) (string, error) {
	args := []string{
		"stop", "-t", STOP_TIMEOUT_STR, id,
	}
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		// probably this is an image, we will just let it through
		if strings.Contains(string(output), "No such container") {
			return id, nil
		}
		return "", &pr.OpError{Op: "stop container", Err: err}
	}
	return id, nil
}

func destroyContainer(id string) error {
	id, err := stopContainer(id)
	if err != nil {
		return err
	}
	exec.Command("docker", "rm", id).Run()
	return nil
}

// - create a container
// - commit it as an image
// - stop the container
func (p *Provisioner) Create(name string) (string, error) {
	id, err := runContainer(BASE_IMAGE, name)
	if err != nil {
		return id, &pr.OpError{Op: "run container", Err: err}
	}
	return id, nil
}

// remove the running container
//
// This does not yet remove all the images that we stored with the container.
// *Probably we dont need to do that ?*
func (p *Provisioner) Destroy(name, id string) error {
	return destroyContainer(id)
}

func (p *Provisioner) Stop(name, boxId, userId string) (id string, err error) {
	return stopContainer(boxId)
}

func (p *Provisioner) Start(name, boxId string, resource *pr.Resource, beforeStart func() error) (string, error) {
	boxId, err := runContainer(boxId, name)
	if err != nil {
		err = &pr.OpError{Op: "start: runContainer " + name, Err: err}
		return boxId, err
	}
	if err := beforeStart(); err != nil {
		err = &pr.OpError{Op: "start: beforeStart " + name, Err: err}
		return boxId, err
	}
	// after setup everything, restart the container
	// FIXME: this is so inefficient that i want to cry
	boxId, err = stopContainer(boxId)
	if err != nil {
		err = &pr.OpError{Op: "start: stopContainer" + name, Err: err}
		return boxId, err
	}
	return runContainer(boxId, name)
}

// FIXME: should not be supported
func (p *Provisioner) GetLxcPath(name, id string) string {
	return path.Join(DOCKER_PATH, "containers", id)
}

// useless here. but we will use it anyway
func (p *Provisioner) GetLxcConfigPath(name, id string) string {
	return path.Join(DOCKER_PATH, "containers", id, "config.lxc")
}

func (p *Provisioner) GetRootfsPath(name, id string) string {
	return p.GetCurrentRoot(name, id)
}

func (p *Provisioner) IsMounted(name, id string) (bool, error) {
	return mount.Mounted(p.GetCurrentRoot(name, id))
}

// detects the correct container path of the box ( which can be either b.Disk
// or /var/lib/lxc ). This is used for the migration process where container
// can be in /var/lib/lxc when we deploy new version of boxer, hence will not
// work with the new lxc path
func (p *Provisioner) GetCurrentRoot(name, id string) string {
	return path.Join(DOCKER_PATH, "aufs", "mnt", id)
}

func Init(root string) (pr.Provisioner, error) {
	return &Provisioner{root: root}, nil
}
