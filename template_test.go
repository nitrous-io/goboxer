package goboxer

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
)

type TemplateSuite struct{}

var _ = Suite(&TemplateSuite{})

func (s *TemplateSuite) TestNetworkTemplate(c *C) {
	box := &Box{
		Id:               1,
		Name:             "mclovins-container",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             10000,
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
	box := &Box{
		Id:               1,
		Name:             "mclovins-container",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             10000,
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
	box := &Box{
		Id:               1,
		Name:             "mclovins-container",
		Runtime:          "django",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             10000,
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
	box := &Box{
		Id:               1,
		Name:             "mclovins-container",
		Runtime:          "csharp",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             10000,
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
