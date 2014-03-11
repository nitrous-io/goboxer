package checks

import (
	"fmt"
	. "github.com/motain/gocheck"
	"strings"
)

// "Contains" checker.
type containsChecker struct {
	*CheckerInfo
}

var Contains Checker = &containsChecker{
	&CheckerInfo{Name: "Contains", Params: []string{"value", "search"}},
}

func (checker *containsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	search, ok := params[1].(string)
	if !ok {
		return false, "search must be a string"
	}
	value := params[0]
	valueStr, ok := value.(string)
	if !ok {
		if valueStringer, canString := value.(fmt.Stringer); canString {
			valueStr, ok = valueStringer.String(), true
		}
	}
	if ok {
		return strings.Contains(valueStr, search), ""
	}
	return false, "Value is not a string and does not implement .String()"
}
