// docker provisioner
package docker

import (
	"os/exec"
	"path"
	"strings"

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

// run the image as a new container with provided name
// returns the container id
func runContainer(imageId, name string) (string, error) {
	args := []string{
		"run", "-d", "-name", name, imageId,
	}
	output, err := exec.Command("docker", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// commit and return the image id
func commitContainer(id string) (string, error) {
	args := []string{
		"commit", id,
	}
	output, err := exec.Command("docker", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// - stop the container
// - commit it as a new image
func stopContainer(id string) (string, error) {
	args := []string{
		"stop", id, "-t", STOP_TIMEOUT_STR,
	}
	if err := exec.Command("docker", args...).Run(); err != nil {
		return id, err
	}
	return commitContainer(id)
}

// we ignore the committed image here
// TODO: maybe we should remove the committed image as well.
func destroyContainer(id string) error {
	if _, err := stopContainer(id); err != nil {
		return err
	}
	args := []string{
		"rm", id, "-f",
	}
	return exec.Command("docker", args...).Run()
}

// - create a container
// - commit it as an image
// - stop the container
func (p *Provisioner) Create(name string) (string, error) {
	id, err := runContainer(BASE_IMAGE, name)
	if err != nil {
		return id, err
	}
	return stopContainer(id)
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

func (p *Provisioner) Start(name, boxId string, beforeStart func() error) (string, error) {
	boxId, err := runContainer(name, boxId)
	if err != nil {
		return boxId, err
	}
	boxId, err = stopContainer(boxId)
	if err != nil {
		return boxId, err
	}
	if err := beforeStart(); err != nil {
		err = &pr.OpError{Op: "beforeStart callback " + name, Err: err}
		return boxId, err
	}
	return runContainer(name, boxId)
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
	return path.Join(DOCKER_PATH, "containers", id, "root")
}

func Init(root string) (pr.Provisioner, error) {
	return &Provisioner{root: root}, nil
}
