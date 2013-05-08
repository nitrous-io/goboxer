package boxer

import (
	"errors"
	"github.com/action-io/goboxer/box"
	"github.com/bmizerany/assert"
	"testing"
)

func startAdvance(m *BoxCommand) error  { return nil }
func startRollback(m *BoxCommand) error { return nil }
func endAdvance(m *BoxCommand) error    { return nil }
func endRollback(m *BoxCommand) error   { return nil }
func endAdvanceErr(m *BoxCommand) error { return errors.New("advance failed") }

func NewTestCommand() *BoxCommand {
	startState := State{Name: "start", Advance: startAdvance, Rollback: startRollback}
	endState := State{Name: "end", Advance: endAdvance, Rollback: endRollback}
	states := []State{startState, endState}
	return &BoxCommand{
		Name:         "TestCommand",
		Boxer:        &Boxer{},
		Box:          &box.Box{},
		States:       states,
		CurrentIndex: 0,
	}
}

func TestAutoAdvance(t *testing.T) {
	m := NewTestCommand()
	assert.Equal(t, m.States[m.CurrentIndex].Name, "start")
	Advance(m)
	assert.Equal(t, m.States[m.CurrentIndex].Name, "end")
}

func TestAutoRollback(t *testing.T) {
	finalState := State{Name: "final", Advance: endAdvanceErr, Rollback: endRollback}
	m := NewTestCommand()
	m.States = append(m.States, finalState)
	Advance(m)
	assert.Equal(t, m.States[m.CurrentIndex].Name, "start")
}
