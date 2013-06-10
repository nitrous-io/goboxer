package goboxer

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

// exists returns whether the given file or directory exists or not
// mainly useful for testing, so we ignore the error here
func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func getUidAndGid(file string) []int {
	s, err := os.Stat(file)
	if err != nil {
		return []int{-1, -1}
	}
	return []int{
		int(s.Sys().(*syscall.Stat_t).Uid),
		int(s.Sys().(*syscall.Stat_t).Gid),
	}
}

func DoesExist(c *C, checkExists []string) {
	for _, file := range checkExists {
		c.Assert(exists(file), Equals, true, Commentf("%s should exist", file))
	}
}

func DoesNotExist(c *C, checkNotExists []string) {
	for _, file := range checkNotExists {
		c.Assert(exists(file), Equals, false, Commentf("%s should not exist", file))
	}
}

func DoesContain(c *C, checkContainsContent map[string][]string) {
	for key, value := range checkContainsContent {
		file, _ := ioutil.ReadFile(key)
		fileContent := string(file)
		for _, i := range value {
			c.Assert(strings.Contains(fileContent, i), Equals, true,
				Commentf("%s should contain %s but got content %s", key, i,
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
				Commentf("%s should not contain %s but got content %s", key, i,
					fileContent))
		}
	}
}

func DoesHavePermissions(c *C, checkPermissions map[string][]int) {
	for key, value := range checkPermissions {
		uidAndGid := getUidAndGid(key)
		c.Assert(uidAndGid[0], Equals, value[0], Commentf("Uid mismatched: %d", key))
		c.Assert(uidAndGid[1], Equals, value[1], Commentf("Gid mismatched: %d", key))
	}
}

func DestroyTestBox(name, ip string) error {
	scriptPath, _ := filepath.Abs("./scripts/boxer-cleanup-tests")
	stdout, err := exec.Command("sudo", scriptPath, name, ip).CombinedOutput()
	if err != nil {
		Debug("FailedToDestroyTestBox", string(stdout))
	}
	return err
}

func CreateTestBox(args ...string) error {
	scriptPath, _ := filepath.Abs("./scripts/boxer-setup-test-container")
	stdout, err := exec.Command(scriptPath, args...).CombinedOutput()
	if err != nil {
		Debug("FailedToCreateTestBox", string(stdout))
	}
	return err
}
