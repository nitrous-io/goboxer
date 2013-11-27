package quota

import (
	"fmt"
)

type ValueUnit struct {
	Value float64
	Unit  string
}

// roundup the value unit to int
func (v ValueUnit) ToString() string {
	return fmt.Sprintf("%d%s", int(v.Value), v.Unit)
}

// return the sum of value unit in megabytes
func SumValueUnit(a, b ValueUnit) ValueUnit {
	if a.Unit == b.Unit {
		return ConvertValueUnit(ValueUnit{
			Value: a.Value + b.Value,
			Unit:  a.Unit,
		}, "M")
	}
	au := ScaleUnit(ConvertValueUnitToByte(a), "M")
	bu := ScaleUnit(ConvertValueUnitToByte(b), "M")
	return ValueUnit{
		Value: au + bu,
		Unit:  "M",
	}
}

func ConvertValueUnit(v ValueUnit, m string) ValueUnit {
	au := ScaleUnit(ConvertValueUnitToByte(v), m)
	return ValueUnit{
		Value: au,
		Unit:  m,
	}
}

// Return a value unit from a string. If the string doesnt have unit, we assume
// that it will be K
//
//	ParseValueUnit("123M")
//	>> ValueUnit{Value: 123, Unit: "M"}
//	ParseValueUnit("0.5M")
//	>> ValueUnit{Value: 0.5, Unit: "M"}
//	ParseValueUnit("0.5")
//	>> ValueUnit{Value: 0.5, Unit: "K"}
func ParseValueUnit(s string) ValueUnit {
	var i float64
	var u string
	fmt.Sscanf(s, "%f%s", &i, &u)
	if len(u) == 0 {
		u = "K"
	}
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
