// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"encoding/base64"

	"launchpad.net/goyaml"
	"launchpad.net/juju-core/version"
)

var format_1_16 = formatter_1_16{}

// formatter_1_16 is the formatter for the 1.16 format.
type formatter_1_16 struct {
}

// Ensure that the formatter_1_16 struct implements the formatter interface.
var _ formatter = formatter_1_16{}

// format_1_16Serialization holds information for a given agent.
type format_1_16Serialization struct {
	Tag               string
	Nonce             string
	UpgradedToVersion *version.Number `yaml:"upgradedToVersion"`

	CACert         string
	StateAddresses []string `yaml:",omitempty"`
	StatePassword  string   `yaml:",omitempty"`

	APIAddresses []string `yaml:",omitempty"`
	APIPassword  string   `yaml:",omitempty"`

	OldPassword string
	Values      map[string]string

	// Only state server machines have these next three items
	StateServerCert string `yaml:",omitempty"`
	StateServerKey  string `yaml:",omitempty"`
	APIPort         int    `yaml:",omitempty"`
}

func init() {
	registerFormat(format_1_16)
}

const legacyFormatFilename = "format"

// legacyFormatPrefix is the prefix of the legacy format file.
const legacyFormatPrefix = "format "

// decode64 makes sure that for an empty string we have a nil slice, not an
// empty slice, which is what the base64 DecodeString function returns.
func decode64(value string) (result []byte, err error) {
	if value != "" {
		result, err = base64.StdEncoding.DecodeString(value)
	}
	return
}

func (formatter_1_16) version() string {
	return "1.16"
}

func (formatter_1_16) unmarshal(data []byte) (*configInternal, error) {
	var format format_1_16Serialization
	if err := goyaml.Unmarshal(data, &format); err != nil {
		return nil, err
	}
	caCert, err := decode64(format.CACert)
	if err != nil {
		return nil, err
	}
	stateServerCert, err := decode64(format.StateServerCert)
	if err != nil {
		return nil, err
	}
	stateServerKey, err := decode64(format.StateServerKey)
	if err != nil {
		return nil, err
	}
	if format.UpgradedToVersion == nil {
		// Assume it's 1.16.0.
		upgradedToVersion := version.MustParse("1.16.0")
		format.UpgradedToVersion = &upgradedToVersion
	}
	config := &configInternal{
		tag:               format.Tag,
		nonce:             format.Nonce,
		upgradedToVersion: *format.UpgradedToVersion,
		caCert:            caCert,
		oldPassword:       format.OldPassword,
		stateServerCert:   stateServerCert,
		stateServerKey:    stateServerKey,
		apiPort:           format.APIPort,
		values:            format.Values,
	}
	if len(format.StateAddresses) > 0 {
		config.stateDetails = &connectionDetails{
			format.StateAddresses,
			format.StatePassword,
		}
	}
	if len(format.APIAddresses) > 0 {
		config.apiDetails = &connectionDetails{
			format.APIAddresses,
			format.APIPassword,
		}
	}
	return config, nil
}
