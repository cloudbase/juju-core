// Copyright 2011, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"bytes"
	"text/template"

	yaml "launchpad.net/goyaml"
)

// Config represents a set of cloud-init configuration options.
type Config struct {
	attrs map[string]interface{}
}

// New returns a new Config with no options set.
func New() *Config {
	return &Config{make(map[string]interface{})}
}

// Render returns the cloud-init configuration as a YAML file.
func (cfg *Config) Render() ([]byte, error) {
	data, err := yaml.Marshal(cfg.attrs)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

// Render returns the cloud-init configuration as a YAML file.
func (cfg *Config) RenderWin() ([]byte, error) {
	// data, err := yaml.Marshal(cfg.attrs)
	winCmds := cfg.attrs["powershell"]
	var script []byte
	header := "#ps1_sysnative\r\n"
	script = append(script, header...)
	for _, value := range winCmds.([]*command) {
            script = append(script, value.literal...)
            
    }
	return script, nil
}

func (cfg *Config) set(opt string, yes bool, value interface{}) {
	if yes {
		cfg.attrs[opt] = value
	} else {
		delete(cfg.attrs, opt)
	}
}

// AptSource is an apt(8) source, comprising a source location,
// with an optional Key, and optional apt_preferences(5).
type AptSource struct {
	Source string          `yaml:"source"`
	Key    string          `yaml:"key,omitempty"`
	Prefs  *AptPreferences `yaml:"-"`
}

// AptPreferences is a set of apt_preferences(5) compatible
// preferences for an apt source. It can be used to override the
// default priority for the source. Path where the file will be
// created (usually in /etc/apt/preferences.d/).
type AptPreferences struct {
	Path        string
	Explanation string
	Package     string
	Pin         string
	PinPriority int
}

// FileContents generates an apt_preferences(5) file from the fields
// in prefs.
func (prefs *AptPreferences) FileContents() string {
	const prefTemplate = `
Explanation: {{.Explanation}}
Package: {{.Package}}
Pin: {{.Pin}}
Pin-Priority: {{.PinPriority}}
`
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(prefTemplate[1:]))
	err := t.Execute(&buf, prefs)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

// command represents a shell command.
type command struct {
	literal string
	args    []string
}

// GetYAML implements yaml.Getter
func (t *command) GetYAML() (tag string, value interface{}) {
	if t.args != nil {
		return "", t.args
	}
	return "", t.literal
}

type SSHKeyType string

const (
	RSAPrivate SSHKeyType = "rsa_private"
	RSAPublic  SSHKeyType = "rsa_public"
	DSAPrivate SSHKeyType = "dsa_private"
	DSAPublic  SSHKeyType = "dsa_public"
)
