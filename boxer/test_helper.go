package boxer

import (
	"fmt"
	"github.com/bmizerany/assert"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// exists returns whether the given file or directory exists or not
// mainly useful for testing, so we ignore the error here
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

func DoesExist(t *testing.T, checkExists []string) {
	for _, file := range checkExists {
		assert.Equal(t, exists(file), true, "Failed to create "+file)
	}
}

func DoesContain(t *testing.T, checkContainsContent map[string][]string) {
	for key, value := range checkContainsContent {
		file, _ := ioutil.ReadFile(key)
		fileContent := string(file)
		for _, i := range value {
			assert.Equal(t, strings.Contains(fileContent, i), true, key+" does not contain "+i)
		}
	}
}

func DoesHavePermissions(t *testing.T, checkPermissions map[string][]int) {
	for key, value := range checkPermissions {
		uidAndGid := getUidAndGid(key)
		assert.Equal(t, uidAndGid[0], value[0], fmt.Sprintf("Uid mismatched: %d", key))
		assert.Equal(t, uidAndGid[1], value[1], fmt.Sprintf("Gid mismatched: %d", key))
	}
}

func DestroyTestBox(name, ip string) error {
	scriptPath, _ := filepath.Abs("../scripts/boxer-cleanup-tests")
	return exec.Command("sudo", scriptPath, name, ip).Run()
}
