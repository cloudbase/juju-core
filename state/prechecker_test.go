// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type PrecheckerSuite struct {
	ConnSuite
	prechecker mockPrechecker
}

var _ = gc.Suite(&PrecheckerSuite{})

type mockPrechecker struct {
	precheckInstanceError       error
	precheckInstanceSeries      string
	precheckInstanceConstraints constraints.Value
}

func (p *mockPrechecker) PrecheckInstance(series string, cons constraints.Value) error {
	p.precheckInstanceSeries = series
	p.precheckInstanceConstraints = cons
	return p.precheckInstanceError
}

func (s *PrecheckerSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.prechecker = mockPrechecker{}
	s.policy.getPrechecker = func(*config.Config) (state.Prechecker, error) {
		return &s.prechecker, nil
	}
}

func (s *PrecheckerSuite) TestPrecheckInstance(c *gc.C) {
	// PrecheckInstance should be called with the specified
	// series, and the specified constraints merged with the
	// environment constraints, when attempting to create an
	// instance.
	envCons := constraints.MustParse("mem=4G")
	template, err := s.addOneMachine(c, envCons)
	c.Assert(err, gc.IsNil)
	c.Assert(s.prechecker.precheckInstanceSeries, gc.Equals, template.Series)
	cons := template.Constraints.WithFallbacks(envCons)
	c.Assert(s.prechecker.precheckInstanceConstraints, gc.DeepEquals, cons)
}

func (s *PrecheckerSuite) TestPrecheckErrors(c *gc.C) {
	// Ensure that AddOneMachine fails when PrecheckInstance returns an error.
	s.prechecker.precheckInstanceError = fmt.Errorf("no instance for you")
	_, err := s.addOneMachine(c, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, ".*no instance for you")

	// If the policy's Prechecker method fails, that will be returned first.
	s.policy.getPrechecker = func(*config.Config) (state.Prechecker, error) {
		return nil, fmt.Errorf("no prechecker for you")
	}
	_, err = s.addOneMachine(c, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, ".*no prechecker for you")
}

func (s *PrecheckerSuite) TestPrecheckPrecheckerUnimplemented(c *gc.C) {
	var precheckerErr error
	s.policy.getPrechecker = func(*config.Config) (state.Prechecker, error) {
		return nil, precheckerErr
	}
	_, err := s.addOneMachine(c, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: policy returned nil prechecker without an error")
	precheckerErr = errors.NewNotImplementedError("Prechecker")
	_, err = s.addOneMachine(c, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (s *PrecheckerSuite) TestPrecheckNoPolicy(c *gc.C) {
	s.policy.getPrechecker = func(*config.Config) (state.Prechecker, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}
	state.SetPolicy(s.State, nil)
	_, err := s.addOneMachine(c, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (s *PrecheckerSuite) addOneMachine(c *gc.C, envCons constraints.Value) (state.MachineTemplate, error) {
	err := s.State.SetEnvironConstraints(envCons)
	c.Assert(err, gc.IsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cpu-cores=4")
	template := state.MachineTemplate{
		Series:      "precise",
		Constraints: extraCons,
		Jobs:        oneJob,
	}
	_, err = s.State.AddOneMachine(template)
	return template, err
}

func (s *PrecheckerSuite) TestPrecheckInstanceInjectMachine(c *gc.C) {
	template := state.MachineTemplate{
		InstanceId: instance.Id("bootstrap"),
		Series:     "precise",
		Nonce:      state.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	}
	_, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)
	// PrecheckInstance should not have been called, as we've
	// injected a machine with an existing instance.
	c.Assert(s.prechecker.precheckInstanceSeries, gc.Equals, "")
}

func (s *PrecheckerSuite) TestPrecheckContainerNewMachine(c *gc.C) {
	// Attempting to add a container to a new machine should cause
	// PrecheckInstance to be called.
	template := state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
	c.Assert(err, gc.IsNil)
	c.Assert(s.prechecker.precheckInstanceSeries, gc.Equals, template.Series)
}
