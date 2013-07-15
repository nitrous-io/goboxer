package quota

import (
	"bufio"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

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
	return exec.Command(cmd, args...).Run()
}
