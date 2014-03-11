package lxc

import (
	"github.com/action-io/goboxer/mount"
	pr "github.com/action-io/goboxer/provisioner"
	"path"
)

// populate data.RootPath if successfully mount the box
func mountBox(p *Provisioner, name, id string) (string, error) {
	baseContainerPath := path.Join(p.root, "base_container_template")
	deltaPath := path.Join(p.root, name+"_delta")
	boxId := name + "-" + id
	boxPath := path.Join(p.root, boxId)
	if err := mount.Union("aufs", boxPath, deltaPath, baseContainerPath); err != nil {
		return "", &pr.OpError{Op: "lxc: overlayfs/aufs mount", Err: err}
	}
	// FIXME: MountBind to make it similar to boxer. Ideally we probably
	// want to symlink as it doesnt make any difference and seems to be
	// more stable. Less mountpoint is better
	err := mount.MountBind(boxPath, path.Join(LXC_PATH, boxId))
	if err != nil {
		return "", &pr.OpError{Op: "lxc: overlayfs/aufs mount-bind", Err: err}
	}
	return path.Join(boxPath, "rootfs"), nil
}

func umountBox(p *Provisioner, name, id string) error {
	boxId := name + "-" + id
	boxPath := path.Join(p.root, boxId)
	lxcPath := path.Join(LXC_PATH, boxId)
	// unmount both /var/lib/lxc/mclovins-container-1-1234 and
	// /lxc/disk8/mclovins-container-1-1234
	// but ignore errors here because they are not important for the next start
	// since we always re-generate the mountpoints
	mount.Unmount("aufs", lxcPath)
	mount.Unmount("aufs", boxPath)
	return nil
}
