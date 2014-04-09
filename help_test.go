package goboxer

import (
	"fmt"
	. "github.com/motain/gocheck"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/action-io/goboxer/utils"
)

const ScratchDir = "/tmp/goboxer-tests" // where file artifacts are placed

// Hook up gocheck into go test runner.
func Test(t *testing.T) { TestingT(t) }

// "Contains" checker.
type containsChecker struct {
	*CheckerInfo
}

var Contains Checker = &containsChecker{
	&CheckerInfo{Name: "Contains", Params: []string{"value", "search"}},
}

func (checker *containsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	search, ok := params[1].(string)
	if !ok {
		return false, "search must be a string"
	}
	value := params[0]
	valueStr, ok := value.(string)
	if !ok {
		if valueStringer, canString := value.(fmt.Stringer); canString {
			valueStr, ok = valueStringer.String(), true
		}
	}
	if ok {
		return strings.Contains(valueStr, search), ""
	}
	return false, "Value is not a string and does not implement .String()"
}

// exists returns whether the given file or directory exists or not
// mainly useful for testing, so we ignore the error here
func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
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

// check if []string contains the specified string
func SliceInclude(slice []string, target string) bool {
	for _, i := range slice {
		if i == target {
			return true
		}
	}
	return false
}

// a RetryFn is tried multiple times by RetryEvery
type RetryFn func() bool

// RetryEvery runs RetryFn every interval until it returns true or stopAfter
// has exceeded.  It is assumed that given RetryFn will complete running before interval
func RetryEvery(interval time.Duration, stopAfter time.Duration, fn RetryFn) {
	if fn() {
		return
	}
	trying := true
	go func() {
		for _ = range time.Tick(interval) {
			if !trying || fn() {
				return
			}
		}
	}()
	select {
	case <-time.After(stopAfter):
		trying = false
		fmt.Printf("RetryEvery timed out after %s\n", stopAfter)
		return
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

func DoesHavePermissions(c *C, checkPermissions map[string][]int) {
	for key, value := range checkPermissions {
		uidAndGid := getUidAndGid(key)
		c.Assert(uidAndGid[0], Equals, value[0], Commentf("Uid mismatched: %d", key))
		c.Assert(uidAndGid[1], Equals, value[1], Commentf("Gid mismatched: %d", key))
	}
}

func AssertIptablesOutput(c *C, str string, assertion bool) {
	output, _ := exec.Command("iptables", "-L", "-n", "-t", "nat").Output()
	c.Assert(strings.Contains(string(output), "192.168.254.2:22"), Equals,
		assertion)
}

func MakeRepo(box *Box, repoPath string, scm string) error {
	fullPath := filepath.Join(box.RootFS(), repoPath)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return err
	}
	makeRepo := func(scmCommand string) error {
		_, err := utils.MultiRun(fullPath,
			exec.Command(scmCommand, "init"),
			exec.Command("touch", fmt.Sprintf(".%signore", scmCommand)),
			exec.Command(scmCommand, "add", fmt.Sprintf(".%signore", scmCommand)),
			exec.Command(scmCommand, "commit", "-m", "Initial Commit"),
			exec.Command("chroot", box.RootFS(), "chown", "-R", fmt.Sprintf("%s:%s", box.Uid(), box.Gid()), repoPath))
		return err
	}
	switch scm {
	case ScmGit:
		return makeRepo("git")
	case ScmMercurial:
		return makeRepo("hg")
	case ScmUnsupported:
		return nil
	default:
		panic("Switch not exhaustive")
	}
	panic("Switch should return")
}

func MakeTargz(box *Box, archivePath string) error {
	archiveParent := filepath.Join(ScratchDir, "maketargz")
	os.MkdirAll(archiveParent, 0755)
	defer os.RemoveAll(archiveParent)
	os.MkdirAll(filepath.Dir(archivePath), 0755)
	_, err := utils.MultiRun(archiveParent,
		exec.Command("mkdir", "myrepo-master"),
		exec.Command("touch", "myrepo-master/README.md"),
		exec.Command("tar", "zcf", archivePath, "myrepo-master"))
	return err
}

// verify the container config with expected values
func verifyLxcConfig(c *C, containerPath, container string, configs map[string]string) {
	args := []string{}
	if len(containerPath) > 0 {
		args = append(args, "--lxcpath", containerPath)
	}
	for key, value := range configs {
		cmdArgs := append(args, "-n", container, key)
		cmd := exec.Command("lxc-cgroup", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			c.Assert(value, Equals, string(output))
		}
	}
}
