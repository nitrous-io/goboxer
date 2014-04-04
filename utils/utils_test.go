package utils

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/motain/gocheck"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestExecWithTimeout(c *C) {
	now := time.Now()

	// Times out before command completes.
	err := Exec(exec.Command("sleep", "30"), 5*time.Millisecond)
	c.Assert(err, NotNil)
	c.Assert(time.Now().Sub(now) >= 5*time.Millisecond, Equals, true)
	c.Assert(time.Now().Sub(now) < 30*time.Second, Equals, true)

	// Command completes without timing out.
	err = Exec(exec.Command("sleep", "0.01"), 50*time.Millisecond)
	c.Assert(err, IsNil)

	// Command raises an error.
	err = Exec(exec.Command("invalidcmd"), 1*time.Second)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(fmt.Sprintf("%v", err), "invalid cmd"), Equals, true)
	c.Assert(time.Now().Sub(now) >= 1*time.Second, Equals, false)
}

func (s *UtilSuite) TestPipeCommands(c *C) {
	cmd1 := exec.Command("echo", "this is a test")
	cmd2 := exec.Command("grep", "test")
	cmd3 := exec.Command("wc", "--chars")
	output, err := PipeCommands(cmd1, cmd2, cmd3)

	c.Assert(err, IsNil)
	c.Assert(string(output), Equals, "15\n")
}

func (s *UtilSuite) TestShellEscape(c *C) {
	tests := []struct {
		input   string
		escaped string
	}{
		{"", "''"},
		{"https://github.com/rails/rails`touch foo`", "https://github.com/rails/rails\\`touch\\ foo\\`"},
		{"https://github.com/nitrous-io/autoparts || rm -rf", "https://github.com/nitrous-io/autoparts\\ \\|\\|\\ rm\\ -rf"},
		{"https://github.com/nitrous-io/auto\nparts", "https://github.com/nitrous-io/auto'\n'parts"},
	}
	for _, t := range tests {
		c.Assert(ShellEscape(t.input), Equals, t.escaped)
	}
}
