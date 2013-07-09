package quota

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"strings"
	"testing"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

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
