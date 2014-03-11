package lxc

import (
	pr "github.com/action-io/goboxer/provisioner"
	. "github.com/motain/gocheck"
	"io/ioutil"
	"os"
)

type TemplateSuite struct{}

var _ = Suite(&TemplateSuite{})

func (s *TemplateSuite) TestNetworkTemplate(c *C) {
	box := pr.Config{
		ContainerId:      1,
		ContainerName:    "mclovins-container-1",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
	}
	tmpFile, _ := ioutil.TempFile("/tmp", "network-template")
	defer os.Remove(tmpFile.Name())
	NetworkConfigCompiled.Execute(tmpFile, box)

	checkContainsContent := map[string][]string{}
	checkContainsContent[tmpFile.Name()] = []string{
		"address 192.168.254.2",
		"gateway 192.168.254.1",
	}
	DoesContain(c, checkContainsContent)
}

func (s *TemplateSuite) TestHostsTemplate(c *C) {
	box := pr.Config{
		ContainerId:      1,
		ContainerName:    "mclovins-container-1",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
	}
	tmpFile, _ := ioutil.TempFile("/tmp", "hosts")
	defer os.Remove(tmpFile.Name())
	HostsTemplateCompiled.Execute(tmpFile, box)

	checkContainsContent := map[string][]string{}
	checkContainsContent[tmpFile.Name()] = []string{
		"127.0.1.1 mclovins-container-1",
	}
	DoesContain(c, checkContainsContent)
}

func (s *TemplateSuite) TestReadmeTemplateWhichHasRuntimeMapping(c *C) {
	box := pr.Config{
		ContainerId:      1,
		ContainerName:    "mclovins-container-1",
		Runtime:          "django",
		Template:         "test_container_template",
		Memory:           "256M",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
	}
	tmpFile, _ := ioutil.TempFile("/tmp", "readme")
	defer os.Remove(tmpFile.Name())
	RuntimeTemplates.Get(box.Runtime).Execute(tmpFile, box)

	checkContainsContent := map[string][]string{}
	checkContainsContent[tmpFile.Name()] = []string{
		"Welcome to Nitrous.IO",
		"## Python",
	}
	DoesContain(c, checkContainsContent)
}

func (s *TemplateSuite) TestReadmeTemplateWhichDoesntHaveRuntimeMapping(c *C) {
	box := pr.Config{
		ContainerId:      1,
		ContainerName:    "mclovins-container-1",
		Runtime:          "csharp",
		Template:         "test_container_template",
		Memory:           "256M",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
	}
	tmpFile, _ := ioutil.TempFile("/tmp", "readme")
	defer os.Remove(tmpFile.Name())
	RuntimeTemplates.Get(box.Runtime).Execute(tmpFile, box)

	checkContainsContent := map[string][]string{}
	checkContainsContent[tmpFile.Name()] = []string{
		"Welcome to Nitrous.IO",
	}

	checkDoesNotContainContent := map[string][]string{}
	checkDoesNotContainContent[tmpFile.Name()] = []string{
		"Test Template",
	}

	DoesContain(c, checkContainsContent)
	DoesNotContain(c, checkDoesNotContainContent)
}
