package once

import (
	"sync"
	"testing"
)

func TestDoneReportsCompletion(t *testing.T) {
	var once Once
	if once.Done() {
		t.Fatal("a fresh Once reports done")
	}
	once.Do(func() {})
	if !once.Done() {
		t.Fatal("an executed Once is not reported done")
	}
}

func TestResetAllowsReexecution(t *testing.T) {
	var once Once
	calls := 0
	f := func() { calls++ }

	once.Do(f)
	once.Do(f)
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 before the reset", calls)
	}

	once.Reset()
	if once.Done() {
		t.Fatal("a reset Once reports done")
	}

	once.Do(f)
	if calls != 2 {
		t.Fatalf("calls = %d after the reset, want 2", calls)
	}
	if !once.Done() {
		t.Fatal("the reset Once is done again after reexecution")
	}
}

func TestDoneConcurrentWithExecution(t *testing.T) {
	var once Once
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			once.Do(func() {})
		}()
		go func() {
			defer wg.Done()
			once.Done()
		}()
	}
	wg.Wait()
	if !once.Done() {
		t.Fatal("the Once is not done after concurrent execution")
	}
}
