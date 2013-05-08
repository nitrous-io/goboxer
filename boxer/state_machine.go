package boxer

import (
	"github.com/action-io/goboxer/box"
	"github.com/action-io/goboxer/utils"
)

// A handler is a function that accept an object and does something. It will
// return an error when it fails to perform the work. In that case, the parent
// should rollback the state
type Handler func(c *BoxCommand) error

// A State consist of a name, a function that does work when we advance the
// state, and a function that does work when we want to rollback
type State struct {
	Name     string
	Advance  Handler
	Rollback Handler
}

type BoxCommand struct {
	Name         string
	Boxer        *Boxer
	Box          *box.Box
	States       []State
	CurrentIndex int
}

func Advance(command *BoxCommand) {
	state := command.States[command.CurrentIndex]
	err := state.Advance(command)
	if err != nil {
		utils.Debug("err", command.Name, command.CurrentIndex, err)
		Rollback(command)
	} else {
		nextIndex := command.CurrentIndex + 1
		if nextIndex < len(command.States) {
			command.CurrentIndex = nextIndex
			Advance(command)
		}
	}
}

func Rollback(command *BoxCommand) {
	state := command.States[command.CurrentIndex]
	err := state.Rollback(command)
	if err != nil {
		Advance(command)
	} else {
		previousIndex := command.CurrentIndex - 1
		if previousIndex >= 0 {
			command.CurrentIndex = previousIndex
			Rollback(command)
		}
	}
}
