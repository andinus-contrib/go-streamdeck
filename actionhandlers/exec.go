package actionhandlers

import (
	streamdeck "github.com/andinus-contrib/go-streamdeck"
	"os/exec"
)

type ExecAction struct {
	Command *exec.Cmd
}

func (action *ExecAction) Pressed(btn streamdeck.Button) {
	action.Command.Start()
}

func NewExecAction(command *exec.Cmd) *ExecAction {
	return &ExecAction{Command: command}
}
