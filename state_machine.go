package goboxer

import (
	"fmt"
	"time"
)

// A handler is a function that accept an object and does something. It will
// return an error when it fails to perform the work. In that case, the parent
// should rollback the state
type StateHandler func(c *BoxCommand) error

// A State consist of a name, a function that does work when we advance the
// state, and a function that does work when we want to rollback
type State struct {
	Name     string
	Advance  StateHandler
	Rollback StateHandler
}

type BoxCommand struct {
	Name         string
	Boxer        *Boxer
	Box          *Box
	States       []State
	CurrentIndex int
}

type StateError struct {
	Name   string
	When   time.Time
	Detail error
}

func (e *StateError) Error() string {
	return fmt.Sprintf("State Error: %+v - %+v - %+v", e.Name, e.When, e.Detail)
}

func NewStateError(name string, err error) StateError {
	return StateError{Name: name, When: time.Now(), Detail: err}
}

type CommandError struct {
	AdvanceStack  []StateError
	RollbackStack []StateError
}

// check if the command completes successfully
func (e *CommandError) Success() bool {
	return len(e.AdvanceStack) == 0 && len(e.RollbackStack) == 0
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("CommandError: %+v-%+v", e.AdvanceStack, e.RollbackStack)
}

func advanceState(command *BoxCommand, errStack *CommandError) {
	state := command.States[command.CurrentIndex]
	err := state.Advance(command)
	if err != nil {
		stateError := NewStateError(state.Name, err)
		errStack.AdvanceStack = append(errStack.AdvanceStack, stateError)
		// if the rollback stack has errors, we will not try to re-rollback
		if len(errStack.RollbackStack) == 0 {
			rollbackState(command, errStack)
		}
	} else {
		nextIndex := command.CurrentIndex + 1
		if nextIndex < len(command.States) {
			command.CurrentIndex = nextIndex
			advanceState(command, errStack)
		}
	}
}

func rollbackState(command *BoxCommand, errStack *CommandError) {
	state := command.States[command.CurrentIndex]
	err := state.Rollback(command)
	if err != nil {
		stateError := NewStateError(state.Name, err)
		// if the advance stack has errors, we will not try to re-advance
		errStack.RollbackStack = append(errStack.RollbackStack, stateError)
		if len(errStack.AdvanceStack) == 0 {
			advanceState(command, errStack)
		}
	} else {
		previousIndex := command.CurrentIndex - 1
		if previousIndex >= 0 {
			command.CurrentIndex = previousIndex
			rollbackState(command, errStack)
		}
	}
}

func Advance(command *BoxCommand) *CommandError {
	errStack := &CommandError{}
	advanceState(command, errStack)
	return errStack
}

func Rollback(command *BoxCommand) *CommandError {
	errStack := &CommandError{}
	rollbackState(command, errStack)
	return errStack
}
