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

type ValueUnit struct {
	Value float64
	Unit  string
}

// Return a value unit from a string
//
//	ParseValueUnit("123M")
//	>> ValueUnit{Value: 123, Unit: "M"}
//	ParseValueUnit("0.5M")
//	>> ValueUnit{Value: 0.5, Unit: "M"}
func ParseValueUnit(s string) ValueUnit {
	var i float64
	var u string
	fmt.Sscanf(s, "%f%s", &i, &u)
	return ValueUnit{i, u}
}

// Scale value unit to a number
//
// ConvertValueUnitToByte(ValueUnit(1, "M"))
// >> 1024
func ConvertValueUnitToByte(v ValueUnit) int {
	return int(v.Value * VALUE_UNIT_SCALE[v.Unit])
}

// Scale a value to a given unit
//
// ScaleUnit(512 * 1024, "M")
// >> "512"
func ScaleUnit(v int, u string) float64 {
	return float64(v) / VALUE_UNIT_SCALE[u]
}

var VALUE_UNIT_SCALE map[string]float64

func init() {
	VALUE_UNIT_SCALE = map[string]float64{
		"M": 1024.0,
		"G": 1024.0 * 1024.0,
	}
}
