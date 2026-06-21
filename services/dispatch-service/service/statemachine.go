package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
)

// validTransitions defines every legal (from -> set of to) transition for the
// driver state machine.
func validTransitions() map[model.DriverState]map[model.DriverState]bool {
	return map[model.DriverState]map[model.DriverState]bool{
		model.DriverOffline:    {model.DriverIdle: true},
		model.DriverIdle:       {model.DriverAssigned: true, model.DriverOffline: true},
		model.DriverAssigned:   {model.DriverToHub: true, model.DriverIdle: true, model.DriverOffline: true},
		model.DriverToHub:      {model.DriverPicking: true, model.DriverIdle: true},
		model.DriverPicking:    {model.DriverDelivering: true, model.DriverIdle: true},
		model.DriverDelivering: {model.DriverCompleted: true, model.DriverIdle: true},
		model.DriverCompleted:  {model.DriverIdle: true, model.DriverOffline: true},
	}
}

// DriverStateMachine manages driver lifecycle state transitions with full
// event sourcing. Every transition is validated and recorded.
type DriverStateMachine struct {
	mu          sync.RWMutex
	drivers     map[string]*model.Driver
	events      []model.StateTransitionEvent
	transitions map[model.DriverState]map[model.DriverState]bool
}

// NewDriverStateMachine creates a new state machine with the default
// transition rules.
func NewDriverStateMachine() *DriverStateMachine {
	return &DriverStateMachine{
		drivers:     make(map[string]*model.Driver),
		events:      make([]model.StateTransitionEvent, 0),
		transitions: validTransitions(),
	}
}

// AddDriver registers a driver with the state machine.
func (sm *DriverStateMachine) AddDriver(d *model.Driver) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.drivers[d.ID] = d
}

// GetDriver returns a driver by ID. The second return value indicates whether
// the driver was found.
func (sm *DriverStateMachine) GetDriver(id string) (*model.Driver, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	d, ok := sm.drivers[id]
	return d, ok
}

// GetDriversByState returns all drivers currently in the given state.
func (sm *DriverStateMachine) GetDriversByState(state model.DriverState) []*model.Driver {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var result []*model.Driver
	for _, d := range sm.drivers {
		if d.Status == state {
			result = append(result, d)
		}
	}
	return result
}

// AvailableDrivers returns all drivers in the IDLE state.
func (sm *DriverStateMachine) AvailableDrivers() []*model.Driver {
	return sm.GetDriversByState(model.DriverIdle)
}

// AllDrivers returns all registered drivers.
func (sm *DriverStateMachine) AllDrivers() []*model.Driver {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]*model.Driver, 0, len(sm.drivers))
	for _, d := range sm.drivers {
		result = append(result, d)
	}
	return result
}

// Transition attempts to move a driver from its current state to `to`. It
// returns an error if the transition is invalid or the driver is unknown. On
// success a StateTransitionEvent is appended to the event log.
func (sm *DriverStateMachine) Transition(driverID string, to model.DriverState, reason string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	driver, ok := sm.drivers[driverID]
	if !ok {
		return fmt.Errorf("driver %s not found", driverID)
	}

	from := driver.Status
	validTo, ok := sm.transitions[from]
	if !ok || !validTo[to] {
		return fmt.Errorf("invalid state transition: from %s to %s", from, to)
	}

	driver.Status = to
	sm.events = append(sm.events, model.StateTransitionEvent{
		DriverID:  driverID,
		FromState: from,
		ToState:   to,
		Timestamp: time.Now(),
		Reason:    reason,
	})
	return nil
}

// Events returns a copy of all recorded state transition events.
func (sm *DriverStateMachine) Events() []model.StateTransitionEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]model.StateTransitionEvent, len(sm.events))
	copy(result, sm.events)
	return result
}

// UpdateLocation sets a driver's current geographic coordinates.
func (sm *DriverStateMachine) UpdateLocation(driverID string, lat, lng float64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	driver, ok := sm.drivers[driverID]
	if !ok {
		return fmt.Errorf("driver %s not found", driverID)
	}
	driver.Location = model.Location{Lat: lat, Lng: lng}
	return nil
}
