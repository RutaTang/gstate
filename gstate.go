package gstate

import (
	"fmt"
	"sync"
)

// `transitionMeta` will be sent to the `transitionChan` to process the transition.
type transitionMeta struct {
	event     *Event
	happendAt int
	err       error
}

// Finite state machine
type Machine struct {
	stateEventMapTransitionMap map[*State]map[*Event]*Transition
	currentState               *State
	transitionChan             chan transitionMeta
	stateMutex                 sync.Mutex
	waitGroup                  sync.WaitGroup
	processedCount             int

	//machine-global lifecycle functions
	LeaveStateFunc func()
	EnterStateFunc func()
}

func (m *Machine) Start(initialState *State) {
	m.currentState = initialState
	go m.daemon()
}

// This daemon is to process the transition one by one.
// It will block the transition if the previous transition is not finished.
// And it will ignore the transition if the event is not valid.
// Note: this daemon will be runned in a goroutine while the machine is started.
func (m *Machine) daemon() {
	for {
		select {
		case tm := <-m.transitionChan:
			func() {
				m.stateMutex.Lock()
				defer m.stateMutex.Unlock()
				defer m.waitGroup.Done()
				if tm.err != nil {
					return
				}
				if tm.happendAt != m.processedCount {
					return
				}
				if transition, ok := m.stateEventMapTransitionMap[m.currentState][tm.event]; ok && transition.Src == m.currentState {
					// leave any state lifecyle
					if m.LeaveStateFunc != nil {
						m.LeaveStateFunc()
					}
					// leave the current state lifecycle
					if m.currentState.LeaveStateFunc != nil {
						m.currentState.LeaveStateFunc()
					}
					// transition success
					m.currentState = transition.Dst
					m.processedCount++
					// enter the new state lifecycle
					if m.currentState.EnterStateFunc != nil {
						m.currentState.EnterStateFunc()
					}
					// enter any state lifecycle
					if m.EnterStateFunc != nil {
						m.EnterStateFunc()
					}
				}
			}()
		}
	}
}

func (m *Machine) GetCurrentStateName() string {
	return m.currentState.Name
}

// Wait for all the transitions to be finished.
// Note: this function should always be called after sending an event even though there is just one event to make sure the transition is finished.
func (m *Machine) WaitForTransitions() {
	m.waitGroup.Wait()
}

// Send an event to the machine.
// It will send `transitionMeta` to the `transitionChan` to process the transition async.
func (m *Machine) SendEvent(event *Event) error {
	// check if the event is valid
	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()
	transition, ok := m.stateEventMapTransitionMap[m.currentState][event]
	if !ok {
		err := fmt.Errorf("invalid event: %s", event.Name)
		return err
	}
	if transition.Src != m.currentState {
		err := fmt.Errorf("invalid event: %s, current state: %s", event.Name, m.currentState.Name)
		return err
	}
	// send the transitionMeta to the transitionChan
	m.waitGroup.Add(1)
	tm := transitionMeta{event: event, happendAt: m.processedCount}
	done := func() {
		m.transitionChan <- tm
	}
	cancel := func(err error) {
		tm.err = err
		m.transitionChan <- tm
	}
	if transition.TransitionFunc != nil {
		transition.TransitionFunc(done, cancel)
	} else {
		done()
	}
	return nil
}

type Transition struct {
	Src            *State
	Dst            *State
	TransitionFunc func(done func(), cancel func(error))
}

type Event struct {
	Name string
}

type State struct {
	Name           string
	LeaveStateFunc func()
	EnterStateFunc func()
}

type EventTransition struct {
	Event      *Event
	Transition *Transition
}

type EventTransitionGroup []EventTransition

func NewMachine(eventTransitionGroup EventTransitionGroup) *Machine {
	stateEventMapTransitionMap := make(map[*State]map[*Event]*Transition)
	for _, eventTransition := range eventTransitionGroup {
		event := eventTransition.Event
		src := eventTransition.Transition.Src
		transition := eventTransition.Transition
		if _, ok := stateEventMapTransitionMap[src]; !ok {
			stateEventMapTransitionMap[src] = make(map[*Event]*Transition)
		}
		stateEventMapTransitionMap[src][event] = transition
	}
	return &Machine{
		stateEventMapTransitionMap: stateEventMapTransitionMap,
		transitionChan:             make(chan transitionMeta),
	}
}
