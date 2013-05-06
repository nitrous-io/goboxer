package goboxer

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func debug(a ...interface{}) {
	fmt.Println(a...)
}

// exists returns whether the given file or directory exists or not
// mainly useful for testing, so we ignore the error here
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	return err == nil, err
}

// check if content of the file contain a certain string
func FileContainsString(path string, content string) (bool, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return false, err
	}
	if !strings.Contains(string(file), content) {
		return false, fmt.Errorf("file %s does not contain %s", path, content)
	}
	return true, nil
}

func GetUid(file string) (int, error) {
	s, err := os.Stat(file)
	if err != nil {
		return -1, err
	}
	return int(s.Sys().(*syscall.Stat_t).Uid), nil
}

func GetGid(file string) (int, error) {
	s, err := os.Stat(file)
	if err != nil {
		return -1, err
	}
	return int(s.Sys().(*syscall.Stat_t).Gid), nil
}

func DestroyTestBox(name, ip string) error {
	return exec.Command("sudo", "./scripts/boxer-cleanup-tests", name, ip).Run()
}
