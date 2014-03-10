package lxc

import (
	. "github.com/motain/gocheck"
	"io/ioutil"
	"strings"
	"testing"
)

// Hook up gocheck into go test runner.
func Test(t *testing.T) { TestingT(t) }

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
