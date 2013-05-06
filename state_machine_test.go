package goboxer

import (
	"errors"
	"testing"
)

type TestMachine struct {
	States       []State
	CurrentIndex int
}

func startAdvance(m StateMachine) error  { return nil }
func startRollback(m StateMachine) error { return nil }
func endAdvance(m StateMachine) error    { return nil }
func endRollback(m StateMachine) error   { return nil }
func endAdvanceErr(m StateMachine) error { return errors.New("advance failed") }

func NewTestMachine() *TestMachine {
	startState := State{Name: "start", Advance: startAdvance, Rollback: startRollback}
	endState := State{Name: "end", Advance: endAdvance, Rollback: endRollback}
	states := []State{startState, endState}
	return &TestMachine{States: states, CurrentIndex: 0}
}

func (t *TestMachine) GetState() State       { return t.States[t.CurrentIndex] }
func (t *TestMachine) GetStateName() string  { return t.States[t.CurrentIndex].Name }
func (t *TestMachine) GetStates() []State    { return t.States }
func (t *TestMachine) GetCurrentIndex() int  { return t.CurrentIndex }
func (t *TestMachine) SetCurrentIndex(i int) { t.CurrentIndex = i }

func TestAutoAdvance(t *testing.T) {
	m := NewTestMachine()
	if m.GetStateName() != "start" {
		t.Errorf("Expect state %s but got %s", "start", m.GetStateName())
	}
	Advance(StateMachine(m))
	if m.GetStateName() != "end" {
		t.Errorf("Expect state %s but got %s", "end", m.GetStateName())
	}
}

func TestAutoRollback(t *testing.T) {
	finalState := State{Name: "final", Advance: endAdvanceErr, Rollback: endRollback}
	m := NewTestMachine()
	m.States = append(m.States, finalState)
	Advance(StateMachine(m))
	if m.GetStateName() != "start" {
		t.Errorf("Expect state %s but got %s", "start", m.GetStateName())
	}
}
