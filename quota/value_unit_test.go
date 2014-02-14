package quota

import (
	. "launchpad.net/gocheck"
)

type ValueUnitSuite struct{}

var _ = Suite(&ValueUnitSuite{})

func (s *ValueUnitSuite) TestParseValueUnit(c *C) {
	var v ValueUnit
	v = ParseValueUnit("123M")
	c.Assert(v.Value, Equals, 123.0)
	c.Assert(v.Unit, Equals, "M")

	v = ParseValueUnit("123")
	c.Assert(v.Value, Equals, 123.0)
	c.Assert(v.Unit, Equals, "K")
}

func (s *ValueUnitSuite) TestConvertValueUnit(c *C) {
	var v ValueUnit
	v = ParseValueUnit("1024K")
	v = ConvertValueUnit(v, "M")
	c.Assert(v.Value, Equals, 1.0)
	c.Assert(v.Unit, Equals, "M")

	v = ParseValueUnit("1M")
	v = ConvertValueUnit(v, "K")
	c.Assert(v.Value, Equals, 1024.0)
	c.Assert(v.Unit, Equals, "K")
}

func (s *ValueUnitSuite) TestSumValueUnitMb(c *C) {
	var a, b, sum ValueUnit
	a = ParseValueUnit("1024K")
	b = ParseValueUnit("1024K")
	sum = SumValueUnitMb(a, b)
	c.Assert(sum.Value, Equals, 2.0)
	c.Assert(sum.Unit, Equals, "M")

	a = ParseValueUnit("1024K")
	b = ParseValueUnit("2M")
	sum = SumValueUnitMb(a, b)
	c.Assert(sum.Value, Equals, 3.0)
	c.Assert(sum.Unit, Equals, "M")
}

func (s *ValueUnitSuite) TestScaleUnit(c *C) {
	c.Assert(ScaleUnit(0, "M"), Equals, 0.0)
	c.Assert(ScaleUnit(1.5*1024, "M"), Equals, 1.5)
	c.Assert(ScaleUnit(2*1024, "M"), Equals, 2.0)
}

func (s *ValueUnitSuite) TestScaleUnitCeil(c *C) {
	c.Assert(ScaleUnitCeil(0, "M"), Equals, 0)
	c.Assert(ScaleUnitCeil(1.5*1024, "M"), Equals, 2)
	c.Assert(ScaleUnitCeil(2*1024, "M"), Equals, 2)
}
