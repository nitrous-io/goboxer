package checks

import (
	"fmt"
	. "github.com/motain/gocheck"
	"os"
)

// "Exists" checker.
// check if the file or directory exists
type existsChecker struct {
	*CheckerInfo
}

var Exists Checker = &existsChecker{
	&CheckerInfo{Name: "Exists", Params: []string{"value"}},
}

func (checker *existsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	value := params[0]
	valueStr, ok := value.(string)
	if !ok {
		if valueStringer, canString := value.(fmt.Stringer); canString {
			valueStr, ok = valueStringer.String(), true
		}
	}
	if ok {
		return exists(valueStr), ""
	}
	return false, "Value is not a string and does not implement .String()"
}

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
