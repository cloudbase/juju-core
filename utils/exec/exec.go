// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.util.exec")

// Parameters for RunCommands.  Commands contains one or more commands to be
// executed using '/bin/bash -s'.  If WorkingDir is set, this is passed
// through to bash.  Similarly if the Environment is specified, this is used
// for executing the command.
type RunParams struct {
	Commands    string
	WorkingDir  string
	Environment []string
}

// ExecResponse contains the return code and output generated by executing a
// command.
type ExecResponse struct {
	Code   int
	Stdout []byte
	Stderr []byte
}