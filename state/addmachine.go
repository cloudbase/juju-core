// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"

	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

// MachineTemplate holds attributes that are to be associated
// with a newly created machine.
type MachineTemplate struct {
	// Series is the series to be associated with the new machine.
	Series string

	// Constraints are the constraints to be used when finding
	// an instance for the machine.
	Constraints constraints.Value

	// Jobs holds the jobs to run on the machine's instance.
	// A machine must have at least one job to do.
	// JobManageEnviron can only be part of the jobs
	// when the first (bootstrap) machine is added.
	Jobs []MachineJob

	// NoVote holds whether a machine running
	// a state server should abstain from peer voting.
	// It is ignored if Jobs does not contain JobManageEnviron.
	NoVote bool

	// Addresses holds the addresses to be associated with the
	// new machine.
	Addresses []instance.Address

	// InstanceId holds the instance id to associate with the machine.
	// If this is empty, the provisioner will try to provision the machine.
	// If this is non-empty, the HardwareCharacteristics and Nonce
	// fields must be set appropriately.
	InstanceId instance.Id

	// HardwareCharacteristics holds the h/w characteristics to
	// be associated with the machine.
	HardwareCharacteristics instance.HardwareCharacteristics

	// Nonce holds a unique value that can be used to check
	// if a new instance was really started for this machine.
	// See Machine.SetProvisioned. This must be set if InstanceId is set.
	Nonce string

	// Dirty signifies whether the new machine will be treated
	// as unclean for unit-assignment purposes.
	Dirty bool

	// principals holds the principal units that will
	// associated with the machine.
	principals []string
}

// AddMachineInsideNewMachine creates a new machine within a container
// of the given type inside another new machine. The two given templates
// specify the form of the child and parent respectively.
func (st *State) AddMachineInsideNewMachine(template, parentTemplate MachineTemplate, containerType instance.ContainerType) (*Machine, error) {
	mdoc, ops, err := st.addMachineInsideNewMachineOps(template, parentTemplate, containerType)
	if err != nil {
		return nil, fmt.Errorf("cannot add a new machine: %v", err)
	}
	return st.addMachine(mdoc, ops)
}

// AddMachineInsideMachine adds a machine inside a container of the
// given type on the existing machine with id=parentId.
func (st *State) AddMachineInsideMachine(template MachineTemplate, parentId string, containerType instance.ContainerType) (*Machine, error) {
	mdoc, ops, err := st.addMachineInsideMachineOps(template, parentId, containerType)
	if err != nil {
		return nil, fmt.Errorf("cannot add a new machine: %v", err)
	}
	return st.addMachine(mdoc, ops)
}

// AddMachine adds a machine with the given series and jobs.
// It is deprecated and around for testing purposes only.
func (st *State) AddMachine(series string, jobs ...MachineJob) (*Machine, error) {
	ms, err := st.AddMachines(MachineTemplate{
		Series: series,
		Jobs:   jobs,
	})
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddOne machine adds a new machine configured according to the
// given template.
func (st *State) AddOneMachine(template MachineTemplate) (*Machine, error) {
	ms, err := st.AddMachines(template)
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddMachines adds new machines configured according to the
// given templates.
func (st *State) AddMachines(templates ...MachineTemplate) (_ []*Machine, err error) {
	defer utils.ErrorContextf(&err, "cannot add a new machine")
	var ms []*Machine
	env, err := st.Environment()
	if err != nil {
		return nil, err
	} else if env.Life() != Alive {
		return nil, fmt.Errorf("environment is no longer alive")
	}
	var ops []txn.Op
	var mdocs []*machineDoc
	for _, template := range templates {
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, err
		}
		mdocs = append(mdocs, mdoc)
		ms = append(ms, newMachine(st, mdoc))
		ops = append(ops, addOps...)
	}
	ssOps, err := st.maintainStateServersOps(mdocs, nil)
	if err != nil {
		return nil, err
	}
	ops = append(ops, ssOps...)
	ops = append(ops, env.assertAliveOp())
	if err := st.runTransaction(ops); err != nil {
		return nil, onAbort(err, fmt.Errorf("environment is no longer alive"))
	}
	return ms, nil
}

func (st *State) addMachine(mdoc *machineDoc, ops []txn.Op) (*Machine, error) {
	env, err := st.Environment()
	if err != nil {
		return nil, err
	} else if env.Life() != Alive {
		return nil, fmt.Errorf("environment is no longer alive")
	}
	ops = append([]txn.Op{env.assertAliveOp()}, ops...)
	if err := st.runTransaction(ops); err != nil {
		enverr := env.Refresh()
		if (enverr == nil && env.Life() != Alive) || errors.IsNotFoundError(enverr) {
			return nil, fmt.Errorf("environment is no longer alive")
		} else if enverr != nil {
			err = enverr
		}
		return nil, err
	}
	return newMachine(st, mdoc), nil
}

// effectiveMachineTemplate verifies that the given template is
// valid and combines it with values from the state
// to produce a resulting template that more accurately
// represents the data that will be inserted into the state.
func (st *State) effectiveMachineTemplate(p MachineTemplate, allowStateServer bool) (MachineTemplate, error) {
	if p.Series == "" {
		return MachineTemplate{}, fmt.Errorf("no series specified")
	}
	cons, err := st.EnvironConstraints()
	if err != nil {
		return MachineTemplate{}, err
	}
	p.Constraints = p.Constraints.WithFallbacks(cons)
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same
	// constraints.Value struct so here we clear the container
	// value. Provisioning ignores the container value but clearing
	// it avoids potential confusion.
	p.Constraints.Container = nil

	if len(p.Jobs) == 0 {
		return MachineTemplate{}, fmt.Errorf("no jobs specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range p.Jobs {
		if jset[j] {
			return MachineTemplate{}, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	if jset[JobManageEnviron] {
		if !allowStateServer {
			return MachineTemplate{}, errStateServerNotAllowed
		}
	}

	if p.InstanceId != "" {
		if p.Nonce == "" {
			return MachineTemplate{}, fmt.Errorf("cannot add a machine with an instance id and no nonce")
		}
	} else if p.Nonce != "" {
		return MachineTemplate{}, fmt.Errorf("cannot specify a nonce without an instance id")
	}
	return p, nil
}

// addMachineOps returns operations to add a new top level machine
// based on the given template. It also returns the machine document
// that will be inserted.
func (st *State) addMachineOps(template MachineTemplate) (*machineDoc, []txn.Op, error) {
	template, err := st.effectiveMachineTemplate(template, true)
	if err != nil {
		return nil, nil, err
	}
	if template.InstanceId == "" {
		if err := st.precheckInstance(template.Series, template.Constraints); err != nil {
			return nil, nil, err
		}
	}
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	mdoc := machineDocForTemplate(template, strconv.Itoa(seq))
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(mdoc, template.Constraints)...)
	ops = append(ops, st.insertNewContainerRefOp(mdoc.Id))
	if template.InstanceId != "" {
		ops = append(ops, txn.Op{
			C:      st.instanceData.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: &instanceData{
				Id:         mdoc.Id,
				InstanceId: template.InstanceId,
				Arch:       template.HardwareCharacteristics.Arch,
				Mem:        template.HardwareCharacteristics.Mem,
				RootDisk:   template.HardwareCharacteristics.RootDisk,
				CpuCores:   template.HardwareCharacteristics.CpuCores,
				CpuPower:   template.HardwareCharacteristics.CpuPower,
				Tags:       template.HardwareCharacteristics.Tags,
			},
		})
	}
	return mdoc, ops, nil
}

// supportsContainerType reports whether the machine supports the given
// container type. If the machine's supportedContainers attribute is
// set, this decision can be made right here, otherwise we assume that
// everything will be ok and later on put the container into an error
// state if necessary.
func (m *Machine) supportsContainerType(ctype instance.ContainerType) bool {
	supportedContainers, ok := m.SupportedContainers()
	if !ok {
		// We don't know yet, so we report that we support the container.
		return true
	}
	for _, ct := range supportedContainers {
		if ct == ctype {
			return true
		}
	}
	return false
}

// addMachineInsideMachineOps returns operations to add a machine inside
// a container of the given type on an existing machine.
func (st *State) addMachineInsideMachineOps(template MachineTemplate, parentId string, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	if template.InstanceId != "" {
		return nil, nil, fmt.Errorf("cannot specify instance id for a new container")
	}
	template, err := st.effectiveMachineTemplate(template, false)
	if err != nil {
		return nil, nil, err
	}
	if containerType == "" {
		return nil, nil, fmt.Errorf("no container type specified")
	}

	// If a parent machine is specified, make sure it exists
	// and can support the requested container type.
	parent, err := st.Machine(parentId)
	if err != nil {
		return nil, nil, err
	}
	if !parent.supportsContainerType(containerType) {
		return nil, nil, fmt.Errorf("machine %s cannot host %s containers", parentId, containerType)
	}
	newId, err := st.newContainerId(parentId, containerType)
	if err != nil {
		return nil, nil, err
	}
	mdoc := machineDocForTemplate(template, newId)
	mdoc.ContainerType = string(containerType)
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(mdoc, template.Constraints)...)
	ops = append(ops,
		// Update containers record for host machine.
		st.addChildToContainerRefOp(parentId, mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(mdoc.Id),
	)
	return mdoc, ops, nil
}

// newContainerId returns a new id for a machine within the machine
// with id parentId and the given container type.
func (st *State) newContainerId(parentId string, containerType instance.ContainerType) (string, error) {
	seq, err := st.sequence(fmt.Sprintf("machine%s%sContainer", parentId, containerType))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%d", parentId, containerType, seq), nil
}

// addMachineInsideNewMachineOps returns operations to create a new
// machine within a container of the given type inside another
// new machine. The two given templates specify the form
// of the child and parent respectively.
func (st *State) addMachineInsideNewMachineOps(template, parentTemplate MachineTemplate, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	if template.InstanceId != "" || parentTemplate.InstanceId != "" {
		return nil, nil, fmt.Errorf("cannot specify instance id for a new container")
	}
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	parentTemplate, err = st.effectiveMachineTemplate(parentTemplate, false)
	if err != nil {
		return nil, nil, err
	}
	if containerType == "" {
		return nil, nil, fmt.Errorf("no container type specified")
	}
	if parentTemplate.InstanceId == "" {
		if err := st.precheckInstance(parentTemplate.Series, parentTemplate.Constraints); err != nil {
			return nil, nil, err
		}
	}

	parentDoc := machineDocForTemplate(parentTemplate, strconv.Itoa(seq))
	newId, err := st.newContainerId(parentDoc.Id, containerType)
	if err != nil {
		return nil, nil, err
	}
	template, err = st.effectiveMachineTemplate(template, false)
	if err != nil {
		return nil, nil, err
	}
	mdoc := machineDocForTemplate(template, newId)
	mdoc.ContainerType = string(containerType)
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(parentDoc, parentTemplate.Constraints)...)
	ops = append(ops, st.insertNewMachineOps(mdoc, template.Constraints)...)
	ops = append(ops,
		// The host machine doesn't exist yet, create a new containers record.
		st.insertNewContainerRefOp(mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(parentDoc.Id, mdoc.Id),
	)
	return mdoc, ops, nil
}

func machineDocForTemplate(template MachineTemplate, id string) *machineDoc {
	return &machineDoc{
		Id:         id,
		Series:     template.Series,
		Jobs:       template.Jobs,
		Clean:      !template.Dirty,
		Principals: template.principals,
		Life:       Alive,
		InstanceId: template.InstanceId,
		Nonce:      template.Nonce,
		Addresses:  instanceAddressesToAddresses(template.Addresses),
		NoVote:     template.NoVote,
	}
}

// insertNewMachineOps returns operations to insert the given machine
// document and its associated constraints into the database.
func (st *State) insertNewMachineOps(mdoc *machineDoc, cons constraints.Value) []txn.Op {
	return []txn.Op{
		{
			C:      st.machines.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: mdoc,
		},
		createConstraintsOp(st, machineGlobalKey(mdoc.Id), cons),
		createStatusOp(st, machineGlobalKey(mdoc.Id), statusDoc{
			Status: params.StatusPending,
		}),
	}
}

func hasJob(jobs []MachineJob, job MachineJob) bool {
	for _, j := range jobs {
		if j == job {
			return true
		}
	}
	return false
}

var errStateServerNotAllowed = fmt.Errorf("state server jobs specified without calling EnsureAvailability")

// maintainStateServersOps returns a set of operations that will maintain
// the state server information when the given machine documents
// are added to the machines collection. If currentInfo is nil,
// there can be only one machine document and it must have
// id 0 (this is a special case to allow adding the bootstrap machine)
func (st *State) maintainStateServersOps(mdocs []*machineDoc, currentInfo *StateServerInfo) ([]txn.Op, error) {
	var newIds, newVotingIds []string
	for _, doc := range mdocs {
		if !hasJob(doc.Jobs, JobManageEnviron) {
			continue
		}
		newIds = append(newIds, doc.Id)
		if !doc.NoVote {
			newVotingIds = append(newVotingIds, doc.Id)
		}
	}
	if len(newIds) == 0 {
		return nil, nil
	}
	if currentInfo == nil {
		// Allow bootstrap machine only.
		if len(mdocs) != 1 || mdocs[0].Id != "0" {
			return nil, errStateServerNotAllowed
		}
		var err error
		currentInfo, err = st.StateServerInfo()
		if err != nil {
			return nil, fmt.Errorf("cannot get state server info: %v", err)
		}
		if len(currentInfo.MachineIds) > 0 || len(currentInfo.VotingMachineIds) > 0 {
			return nil, fmt.Errorf("state servers already exist")
		}
	}
	ops := []txn.Op{{
		C:  st.stateServers.Name,
		Id: environGlobalKey,
		Assert: D{{
			"$and", []D{
				{{"machineids", D{{"$size", len(currentInfo.MachineIds)}}}},
				{{"votingmachineids", D{{"$size", len(currentInfo.VotingMachineIds)}}}},
			},
		}},
		Update: D{
			{"$addToSet", D{{"machineids", D{{"$each", newIds}}}}},
			{"$addToSet", D{{"votingmachineids", D{{"$each", newVotingIds}}}}},
		},
	}}
	return ops, nil
}

// EnsureAvailability adds state server machines as necessary to make
// the number of live state servers equal to numStateServers. The given
// constraints and series will be attached to any new machines.
//
// TODO(rog):
// If any current state servers are down, they will be
// removed from the current set of voting replica set
// peers (although the machines themselves will remain
// and they will still remain part of the replica set).
// Once a machine's voting status has been removed,
// the machine itself may be removed.
func (st *State) EnsureAvailability(numStateServers int, cons constraints.Value, series string) error {
	if numStateServers%2 != 1 || numStateServers <= 0 {
		return fmt.Errorf("number of state servers must be odd and greater than zero")
	}
	if numStateServers > replicaset.MaxPeers {
		return fmt.Errorf("state server count is too large (allowed %d)", replicaset.MaxPeers)
	}
	info, err := st.StateServerInfo()
	if err != nil {
		return err
	}
	if len(info.VotingMachineIds) == numStateServers {
		// TODO(rog) #1271504 2014-01-22
		// Find machines which are down, set
		// their NoVote flag and add new machines to
		// replace them.
		return nil
	}
	if len(info.VotingMachineIds) > numStateServers {
		return fmt.Errorf("cannot reduce state server count")
	}
	mdocs := make([]*machineDoc, 0, numStateServers-len(info.MachineIds))
	var ops []txn.Op
	for i := len(info.MachineIds); i < numStateServers; i++ {
		template := MachineTemplate{
			Series: series,
			Jobs: []MachineJob{
				JobHostUnits,
				JobManageEnviron,
			},
			Constraints: cons,
		}
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return err
		}
		mdocs = append(mdocs, mdoc)
		ops = append(ops, addOps...)
	}
	ssOps, err := st.maintainStateServersOps(mdocs, info)
	if err != nil {
		return fmt.Errorf("cannot prepare machine add operations: %v", err)
	}
	ops = append(ops, ssOps...)
	err = st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("failed to create new state server machines: %v", err)
	}
	return nil
}
