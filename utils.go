package goboxer

import (
	"encoding/json"
	"fmt"
)

func Debug(a ...interface{}) {
	fmt.Println(a...)
}

// Convert a struct to its json string representation
func ToJSON(obj interface{}) (string, error) {
	b, err := json.Marshal(obj)
	return string(b), err
}
