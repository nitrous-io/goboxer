package lxc

import (
	"fmt"
	pr "github.com/action-io/goboxer/provisioner"
	"github.com/action-io/goboxer/utils"
	"os"
	"path"
	"strconv"
)

type task func(config lxcConfig) error

type tasks []task

func (ts tasks) execute(config lxcConfig) error {
	for _, t := range ts {
		err := t(config)
		if err != nil {
			return err
		}
	}
	return nil
}

var defaultProvisioningTasks = tasks{
	setEtcAction,
	setPremiumStatus,
	setHostname,
	setHosts,
	setNetworkInterface,
	setDefaultKeyPair,
	setPermission,
}

func setWorkspace(config lxcConfig) error {
	workspacePath := path.Join(config.RootFS, "home", "action", "workspace")
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return &pr.OpError{Op: "set workspace", Err: err}
	}
	return nil
}

func setReadme(config lxcConfig) error {
	readmePath := path.Join(config.RootFS, "home", "action", "README.md")
	readme, err := os.Create(readmePath)
	if err != nil {
		return &pr.OpError{Op: "set readme", Err: err}
	}
	defer readme.Close()
	return RuntimeTemplates.Get(config.Runtime).Execute(readme, config)
}

func setEtcAction(config lxcConfig) error {
	actionPath := path.Join(config.RootFS, "etc", "action")
	if err := os.MkdirAll(actionPath, 0755); err != nil {
		return &pr.OpError{Op: "mkdir etc action", Err: err}
	}
	// set box_id
	boxIdFile, err := os.Create(path.Join(actionPath, "box_id"))
	if err != nil {
		return &pr.OpError{Op: "create boxid", Err: err}
	}
	defer boxIdFile.Close()
	if _, err = boxIdFile.WriteString(strconv.Itoa(config.ContainerId)); err != nil {
		return &pr.OpError{Op: "write to boxid", Err: err}
	}
	// set box_user
	boxUserFile, err := os.Create(path.Join(actionPath, "box_user"))
	if err != nil {
		return &pr.OpError{Op: "create box_user", Err: err}
	}
	defer boxUserFile.Close()
	_, err = boxUserFile.WriteString("action")
	if err != nil {
		return &pr.OpError{Op: "write to box_user", Err: err}
	}
	return nil
}

func setPremiumStatus(config lxcConfig) error {
	actionPath := path.Join(config.RootFS, "etc", "action")
	premiumPath := path.Join(actionPath, "premium")
	if config.Premium {
		if err := os.MkdirAll(actionPath, 0755); err != nil {
			return err
		}
		premiumFile, err := os.Create(premiumPath)
		if err != nil {
			return err
		}
		defer premiumFile.Close()
		_, err = premiumFile.WriteString("1")
		return err
	} else {
		return os.RemoveAll(premiumPath)
	}
}

func setHostname(config lxcConfig) error {
	hostNamePath := path.Join(config.RootFS, "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.WriteString(config.ContainerName)
	return err
}

func setHosts(config lxcConfig) error {
	hostsPath := path.Join(config.RootFS, "etc", "hosts")
	hosts, err := os.Create(hostsPath)
	if err != nil {
		return err
	}
	defer hosts.Close()
	return HostsTemplateCompiled.Execute(hosts, config)
}

func setNetworkInterface(config lxcConfig) error {
	networkConfigPath := path.Join(config.RootFS, "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, config)
}

func setDefaultKeyPair(config lxcConfig) error {
	sshPath := path.Join(config.RootFS, "home", "action", ".ssh")
	sshKeyFile := path.Join(sshPath, "id_rsa")
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		if err := utils.RunCommand("/bin/sh", "-c", sshKeygen); err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

// set correct permission for user's home folder so he/she can write onto it
func setPermission(config lxcConfig) error {
	uid := config.Uid
	gid := config.Uid // same as uid
	permission := fmt.Sprintf("%s:%s", uid, gid)
	if err := utils.RunCommand("chroot", config.RootFS, "usermod", "-u", uid, "action"); err != nil {
		return err
	}
	if err := utils.RunCommand("chroot", config.RootFS, "groupmod", "-g", gid, "action"); err != nil {
		return err
	}
	return utils.RunCommand("chroot", config.RootFS, "chown", "-R", permission, "/home/action")
}
