package goboxer

import (
	"encoding/json"
	"fmt"
	"os"
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
