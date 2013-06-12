package goboxer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func Debug(a ...interface{}) {
	fmt.Println(a...)
}

// Convert a struct to its json string representation
func ToJSON(obj interface{}) (string, error) {
	b, err := json.Marshal(obj)
	return string(b), err
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
