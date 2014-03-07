package lxc

import (
	pr "github.com/action-io/goboxer/provisioner"
	. "github.com/motain/gocheck"
	"io/ioutil"
	"os"
	"strings"
)

type TemplateSuite struct{}

var _ = Suite(&TemplateSuite{})

func DoesContain(c *C, checkContainsContent map[string][]string) {
	for key, value := range checkContainsContent {
		file, _ := ioutil.ReadFile(key)
		fileContent := string(file)
		for _, i := range value {
			c.Assert(strings.Contains(fileContent, i), Equals, true,
				Commentf("%s should contain '%s' but got content '%s'", key, i,
					fileContent))
		}
	}
}

func DoesNotContain(c *C, checkNotContainsContent map[string][]string) {
	for key, value := range checkNotContainsContent {
		file, _ := ioutil.ReadFile(key)
		fileContent := string(file)
		for _, i := range value {
			c.Assert(strings.Contains(fileContent, i), Equals, false,
				Commentf("%s should not contain '%s' but got content '%s'", key, i,
					fileContent))
		}
	}
}

func (s *TemplateSuite) TestNetworkTemplate(c *C) {
	box := pr.Config{
		ContainerId:      1,
		ContainerName:    "mclovins-container",
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
		ContainerName:    "mclovins-container",
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
		ContainerName:    "mclovins-container",
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
		ContainerName:    "mclovins-container",
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
