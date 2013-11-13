package quota

import (
	"fmt"
)

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

// Scale value unit to a number. This will return the value in KB
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
		"K": 1.0,
		"M": 1024.0,
		"G": 1024.0 * 1024.0,
	}
}
