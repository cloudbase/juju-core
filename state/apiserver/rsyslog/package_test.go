// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	stdtesting "testing"

	"launchpad.net/juju-core/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
