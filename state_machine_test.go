package goboxer

import (
	"errors"
	. "launchpad.net/gocheck"
)

type StateMachineSuite struct{}

var _ = Suite(&StateMachineSuite{})

func startAdvance(m *BoxCommand) error  { return nil }
func startRollback(m *BoxCommand) error { return nil }
func endAdvance(m *BoxCommand) error    { return nil }
func endRollback(m *BoxCommand) error   { return nil }
func endAdvanceErr(m *BoxCommand) error { return errors.New("failed") }

func newTestCommand() *BoxCommand {
	return &BoxCommand{
		Name:  "TestCommand",
		Boxer: &Boxer{},
		Box:   &Box{},
		States: []State{
			State{Name: "start", Advance: startAdvance, Rollback: startRollback},
			State{Name: "end", Advance: endAdvance, Rollback: endRollback},
		},
	}
}

func (s *StateMachineSuite) TestMergeState(c *C) {
	a := []State{
		State{Name: "start", Advance: startAdvance, Rollback: startRollback},
	}
	b := []State{
		State{Name: "end", Advance: endAdvance, Rollback: endRollback},
	}
	res := MergeStates(a, b)
	c.Assert(len(res), Equals, 2)
	c.Assert(res[0].Name, Equals, "start")
	c.Assert(res[1].Name, Equals, "end")
}

func (s *StateMachineSuite) TestAutoAdvance(c *C) {
	m := newTestCommand()
	c.Assert(m.States[m.CurrentIndex].Name, Equals, "start")
	err := Advance(m)
	c.Assert(err.Success(), Equals, true)
	c.Assert(m.States[m.CurrentIndex].Name, Equals, "end")
}

func (s *StateMachineSuite) TestAutoRollback(c *C) {
	finalState := State{Name: "final", Advance: endAdvanceErr, Rollback: endRollback}
	m := newTestCommand()
	m.States = append(m.States, finalState)
	err := Advance(m)
	c.Assert(err, NotNil, Commentf("failed to return error"))
	c.Assert(err.AdvanceStack[0].Name, Equals, "final")
	c.Assert(m.States[m.CurrentIndex].Name, Equals, "start")
}

func (s *StateMachineSuite) TestLimitNumberOfRevert(c *C) {
	rollbackFail := State{Name: "stage-one", Advance: startAdvance, Rollback: endAdvanceErr}
	advanceFail := State{Name: "stage-two", Advance: endAdvanceErr, Rollback: endRollback}

	m := newTestCommand()
	m.States = append(m.States, rollbackFail, advanceFail)
	err := Advance(m)
	c.Assert(err, NotNil, Commentf("failed to return error"))
	c.Assert(err.AdvanceStack[0].Name, Equals, "stage-two")
	c.Assert(err.RollbackStack[0].Name, Equals, "stage-one")
	c.Assert(m.States[m.CurrentIndex].Name, Equals, "stage-one")
}
