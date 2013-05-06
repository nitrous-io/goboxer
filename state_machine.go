package goboxer

// A handler is a function that accept an object and does something. It will
// return an error when it fails to perform the work. In that case, the parent
// should rollback the state
type Handler func(m StateMachine) error

// A State consist of a name, a function that does work when we advance the
// state, and a function that does work when we want to rollback
type State struct {
	Name     string
	Advance  Handler
	Rollback Handler
}

type StateMachine interface {
	GetStateName() string
	GetStates() []State
	GetCurrentIndex() int
	SetCurrentIndex(i int)
	GetState() State
}

func Advance(machine StateMachine) {
	currentIndex := machine.GetCurrentIndex()
	state := machine.GetState()
	err := state.Advance(machine)
	if err != nil {
		Rollback(machine)
	} else {
		nextIndex := currentIndex + 1
		if nextIndex < len(machine.GetStates()) {
			machine.SetCurrentIndex(nextIndex)
			Advance(machine)
		}
	}
}

func Rollback(machine StateMachine) {
	currentIndex := machine.GetCurrentIndex()
	state := machine.GetState()
	err := state.Rollback(machine)
	if err != nil {
		Advance(machine)
	} else {
		previousIndex := currentIndex - 1
		if previousIndex >= 0 {
			machine.SetCurrentIndex(previousIndex)
			Rollback(machine)
		}
	}
}
