// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package version

func osVersion() string {
	series := readSeries(lsbReleaseFile)
	logger.Infof("Release is: %v", series)
	logger.Infof("Read from: %v", lsbReleaseFile)
	return readSeries(lsbReleaseFile)
}
