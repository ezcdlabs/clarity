package clock_test

import (
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/clock"
)

func TestFake_TimerDoesNotFireWithoutAdvance(t *testing.T) {
	fake := clock.NewFake()
	ch := fake.After(time.Hour)

	select {
	case <-ch:
		t.Fatal("timer fired without Advance being called")
	default:
	}
}

func TestFake_TimerFiresAfterAdvancePastDeadline(t *testing.T) {
	fake := clock.NewFake()
	ch := fake.After(5 * time.Second)

	fake.Advance(5 * time.Second)

	select {
	case <-ch:
	default:
		t.Fatal("timer did not fire after Advance past deadline")
	}
}

func TestFake_TimerDoesNotFireBeforeDeadline(t *testing.T) {
	fake := clock.NewFake()
	ch := fake.After(5 * time.Second)

	fake.Advance(4 * time.Second)

	select {
	case <-ch:
		t.Fatal("timer fired before deadline was reached")
	default:
	}
}

func TestFake_MultipleTimers_AllFireWhenAdvancedPast(t *testing.T) {
	fake := clock.NewFake()
	ch1 := fake.After(1 * time.Second)
	ch2 := fake.After(2 * time.Second)
	ch3 := fake.After(10 * time.Second)

	fake.Advance(5 * time.Second)

	for _, ch := range []<-chan time.Time{ch1, ch2} {
		select {
		case <-ch:
		default:
			t.Fatal("expected timer to fire after Advance past its deadline")
		}
	}
	select {
	case <-ch3:
		t.Fatal("timer with deadline beyond Advance should not have fired")
	default:
	}
}

func TestFake_TimerAdded_SignalsOnRegistration(t *testing.T) {
	fake := clock.NewFake()

	select {
	case <-fake.TimerAdded():
		t.Fatal("TimerAdded should not signal before any timer is registered")
	default:
	}

	fake.After(time.Second)

	select {
	case <-fake.TimerAdded():
	default:
		t.Fatal("TimerAdded should signal after a timer is registered")
	}
}
