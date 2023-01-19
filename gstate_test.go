package gstate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RutaTang/gstate"
)

func TestBasic(t *testing.T) {
	e1 := gstate.Event{Name: "start"}
	e2 := gstate.Event{Name: "stop"}
	s1 := gstate.State{Name: "closed"}
	s2 := gstate.State{Name: "open"}

	eventTransitionMap := &gstate.EventTransitionGroup{
		&gstate.EventTransition{
			Event: &e1,
			Transition: &gstate.Transition{
				Src: &s1,
				Dst: &s2,
			},
		},
		&gstate.EventTransition{
			Event: &e2,
			Transition: &gstate.Transition{
				Src: &s2,
				Dst: &s1,
			},
		},
	}

	m := gstate.NewMachine(eventTransitionMap)
	m.Start(&s1)
	if m.GetCurrentStateName() != "closed" {
		t.Error("Expected 'closed', got ", m.GetCurrentStateName())
	}
	m.SendEvent(&e1)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "open" {
		t.Error("Expected 'open', got ", m.GetCurrentStateName())
	}
	m.SendEvent(&e2)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "closed" {
		t.Error("Expected 'closed', got ", m.GetCurrentStateName())
	}
}

func TestOnEventToManyTransitions(t *testing.T) {
	//states
	water := gstate.State{Name: "water"}
	ice := gstate.State{Name: "ice"}
	gas := gstate.State{Name: "gas"}

	//events
	down := gstate.Event{Name: "down"}
	up := gstate.Event{Name: "up"}

	eventTransitionMap := &gstate.EventTransitionGroup{
		&gstate.EventTransition{
			Event: &down,
			Transition: &gstate.Transition{
				Src: &water,
				Dst: &ice,
			},
		},
		&gstate.EventTransition{
			Event: &down,
			Transition: &gstate.Transition{
				Src: &gas,
				Dst: &water,
			},
		},
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &ice,
				Dst: &water,
			},
		},
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &water,
				Dst: &gas,
			},
		},
	}

	m := gstate.NewMachine(eventTransitionMap)
	m.Start(&ice)
	if m.GetCurrentStateName() != "ice" {
		t.Error("Expected 'ice', got ", m.GetCurrentStateName())
	}

	m.SendEvent(&up)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "water" {
		t.Error("Expected 'water', got ", m.GetCurrentStateName())
	}

	m.SendEvent(&up)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "gas" {
		t.Error("Expected 'gas', got ", m.GetCurrentStateName())
	}

	m.SendEvent(&down)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "water" {
		t.Error("Expected 'water', got ", m.GetCurrentStateName())
	}

	m.SendEvent(&down)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "ice" {
		t.Error("Expected 'ice', got ", m.GetCurrentStateName())
	}
}

func TestTransitionWithAsyncFunction(t *testing.T) {
	//states
	water := gstate.State{Name: "water"}
	ice := gstate.State{Name: "ice"}

	//events
	down := gstate.Event{Name: "down"}
	up := gstate.Event{Name: "up"}

	eventTransitionMap := &gstate.EventTransitionGroup{
		&gstate.EventTransition{
			Event: &down,
			Transition: &gstate.Transition{
				Src: &water,
				Dst: &ice,
				TransitionFunc: func(done func(), cancel func(err error)) {
					go func() {
						time.Sleep(1 * time.Second)
						done()
					}()
				},
			},
		},
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &ice,
				Dst: &water,
			},
		},
	}

	m := gstate.NewMachine(eventTransitionMap)
	m.Start(&water)

	m.SendEvent(&down)
	now := time.Now()
	m.WaitForTransitions()
	elapsed := time.Since(now)
	if elapsed < 1*time.Second {
		t.Error("Expected to wait for 1 second, got ", elapsed)
	}
	if m.GetCurrentStateName() != "ice" {
		t.Error("Expected 'ice', got ", m.GetCurrentStateName())
	}
}

func TestCancelTransition(t *testing.T) {
	//states
	water := gstate.State{Name: "water"}
	ice := gstate.State{Name: "ice"}

	//events
	up := gstate.Event{Name: "up"}

	eventTransitionMap := &gstate.EventTransitionGroup{
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &ice,
				Dst: &water,
				TransitionFunc: func(done func(), cancel func(err error)) {
					go func() {
						cancel(errors.New("transition cancelled"))
					}()
				},
			},
		},
	}

	m := gstate.NewMachine(eventTransitionMap)
	m.Start(&ice)

	m.SendEvent(&up)
	m.WaitForTransitions()
	if m.GetCurrentStateName() != "ice" {
		t.Error("Expected 'ice', got ", m.GetCurrentStateName())
	}
}

func TestStateLifeCycle(t *testing.T) {
	//states
	water := gstate.State{Name: "water",
		EnterStateFunc: func() {
			time.Sleep(1 * time.Second)
		},
	}
	ice := gstate.State{Name: "ice",
		LeaveStateFunc: func() {
			time.Sleep(1 * time.Second)
		},
	}

	//events
	up := gstate.Event{Name: "up"}

	eventTransitionMap := &gstate.EventTransitionGroup{
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &ice,
				Dst: &water,
			},
		},
	}

	m := gstate.NewMachine(eventTransitionMap)
	m.Start(&ice)

	m.SendEvent(&up)
	now := time.Now()
	m.WaitForTransitions()
	elapsed := time.Since(now)
	if elapsed < 2*time.Second {
		t.Error("Expected to wait for 2 second, got ", elapsed)
	}
	if m.GetCurrentStateName() != "water" {
		t.Error("Expected 'water', got ", m.GetCurrentStateName())
	}
}

func TestStateDaemon(t *testing.T) {

	done := make(chan struct{})
	//states
	gas := gstate.State{Name: "gas"}
	water := gstate.State{Name: "water", DaemonFunc: func(ctx context.Context, state *gstate.State) {
		for {
			select {
			case <-ctx.Done():
				done <- struct{}{}
				return
			}
		}
	}}
	ice := gstate.State{Name: "ice"}

	//events
	up := gstate.Event{Name: "up"}

	eventTransitionMap := &gstate.EventTransitionGroup{
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &ice,
				Dst: &water,
			},
		},
		&gstate.EventTransition{
			Event: &up,
			Transition: &gstate.Transition{
				Src: &water,
				Dst: &gas,
			},
		},
	}

	m := gstate.NewMachine(eventTransitionMap)
	m.Start(&ice)

	m.SendEvent(&up)
	m.WaitForTransitions()

	m.SendEvent(&up)
	m.WaitForTransitions()

	if m.GetCurrentStateName() != "gas" {
		t.Error("Expected 'gas', got ", m.GetCurrentStateName())
	}

	// should stop state daemon in time
	select {
	case <-done:
		break
	case <-time.After(2 * time.Second):
		t.Error("Expected daemon to be stopped")
	}

}
