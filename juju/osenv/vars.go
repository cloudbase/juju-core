// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
    "path"
)

const (
	JujuEnvEnvKey           = "JUJU_ENV"
	JujuHomeEnvKey          = "JUJU_HOME"
	JujuRepositoryEnvKey    = "JUJU_REPOSITORY"
	JujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	JujuContainerTypeEnvKey = "JUJU_CONTAINER_TYPE"
)

var (
    WinBaseDir = "C:/Juju"
    WinTempDir = path.Join(WinBaseDir, "tmp")
    WinLibDir  = path.Join(WinBaseDir, "lib")
    WinLogDir  = path.Join(WinBaseDir, "log")
    WinDataDir = path.Join(WinLibDir, "juju")
    WinBinDir  = path.Join(WinBaseDir, "bin")
)
