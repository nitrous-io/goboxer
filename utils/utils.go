package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Convert a struct to its json string representation
func ToJSON(obj interface{}) (string, error) {
	b, err := json.Marshal(obj)
	return string(b), err
}

// UnionSlices returns a union of 2 string slices.
func UnionSlices(first []string, second []string) []string {
	for _, elem := range second {
		if !Contains(first, elem) {
			first = append(first, elem)
		}
	}
	return first
}

// Contains returns true if the given needle (a string) exists in the haystack
// (a slice).
func Contains(haystack []string, needle string) bool {
	for _, elem := range haystack {
		if elem == needle {
			return true
		}
	}
	return false
}

// append string content to a file
func Appendln(location, content string) error {
	f, err := os.OpenFile(location, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n" + content)
	return err
}

// Append the content to location unless the location file already had the
// content
func AppendlnUnlessContain(location, content string) error {
	isContain, err := FileContains(location, content)
	if os.IsNotExist(err) {
		if _, err = os.Create(location); err != nil {
			return err
		}
		return Appendln(location, content)
	} else if err != nil {
		return err
	} else if !isContain {
		return Appendln(location, content)
	}
	return nil
}

// check if content of the file as the current content
func FileContains(location, content string) (bool, error) {
	fileContent, err := ioutil.ReadFile(location)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(fileContent), content), nil
}

func DeleteLineContains(file, pattern string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	src := []string{}
	for {
		line, err := reader.ReadString('\n')
		if !strings.Contains(line, pattern) && line != "\n" {
			src = append(src, line)
		}
		if err != nil {
			break
		}
	}
	return ioutil.WriteFile(file, []byte(strings.Join(src, "")), 0)
}

func RunCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	if stdout, err := command.CombinedOutput(); err != nil {
		fmt.Printf("Command %v failed with output %s\n", command, string(stdout))
		return err
	}
	return nil
}

// exec the command with timeout
// - if the command fails to start, returns its error
// - if the command finishes before timeout, returns its error
// - if the command times out, returns the timeout error
func Exec(cmd *exec.Cmd, timeout time.Duration) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%v: invalid cmd, %v", cmd, err)
	}
	c := make(chan error, 1)
	go func() {
		c <- cmd.Wait()
	}()
	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return fmt.Errorf("%v: timed out", cmd)
	case err := <-c:
		return err
	}
}

// PipeCommands pipes the stdout of each command to the stdin of the next
// command. Returns the output of the final command and the first error
// encountered (if any).
// Stderr is ignored.
func PipeCommands(commands ...*exec.Cmd) ([]byte, error) {
	last := len(commands) - 1

	for i, command := range commands[:last] {
		var err error
		if commands[i+1].Stdin, err = command.StdoutPipe(); err != nil {
			return nil, err
		}
		if err := command.Start(); err != nil {
			return nil, err
		}
	}

	output, err := commands[last].Output()
	if err != nil {
		return nil, err
	}

	for _, command := range commands[:last] {
		if err := command.Wait(); err != nil {
			return nil, err
		}
	}

	return output, nil
}

// MultiRun runs multiple cmds in working dir wd.  On error, returns the output
// and error of the problem command, otherwise returns the output of the last
// command and nil.
func MultiRun(wd string, commands ...*exec.Cmd) ([]byte, error) {
	var (
		output []byte
		err    error
	)
	if err = os.MkdirAll(wd, 0755); err != nil {
		return []byte{}, err
	}
	for _, cmd := range commands {
		cmd.Dir = wd
		output, err = cmd.CombinedOutput()
		if err != nil {
			return output, fmt.Errorf("Error running %v in %s: %s", cmd, cmd.Dir, err)
		}
	}
	return output, err
}
