package store

import (
	"fmt"
	"testing"

	"github.com/square/p2/pkg/health"
	"github.com/square/p2/pkg/types"
)

type FakeHealthChecker struct {
	healthResults chan []*health.Result
}

func (hc *FakeHealthChecker) WatchNodeService(nodename types.NodeName, serviceID string, resultCh chan<- health.Result, errCh chan<- error, quitCh <-chan struct{}) {
	panic("not implemented")
}

func (hc *FakeHealthChecker) WatchService(serviceID string, resultCh chan<- map[types.NodeName]health.Result, errCh chan<- error, quitCh <-chan struct{}) {
	panic("not implemented")
}

func (hc *FakeHealthChecker) Service(serviceID string) (map[types.NodeName]health.Result, error) {
	panic("not implemented")
}

func (hc *FakeHealthChecker) WatchHealth(resultCh chan []*health.Result, errCh chan<- error, quitCh <-chan struct{}) {
	select {
	case result := <-hc.healthResults:
		fmt.Printf("res %v", result)
		resultCh <- result
	case <-quitCh:
		return
	}
}

func NewFakeHealthStore() (healthChecker HealthStore, healthValues chan []*health.Result) {
	healthResults := make(chan []*health.Result, 1) // real clients should use a buffered chan. This is unbuffered to simplify concurrency in this test
	hc := &FakeHealthChecker{
		healthResults: healthResults,
	}
	hs := NewHealthStore(hc)

	return hs, healthResults
}

func TestStartWatchBasic(t *testing.T) {
	hs, healthResults := NewFakeHealthStore()
	quitCh := make(chan struct{})

	go func() {
		hs.StartWatch(quitCh)
	}()

	node := types.NodeName("abc01.sjc1")
	podID1 := types.PodID("podID1")
	podID2 := types.PodID("podID2")

	result := hs.Fetch(podID1, node)
	if result != nil {
		t.Errorf("expected cache to start empty, found %v", result)
	}

	healthResults <- []*health.Result{
		&health.Result{ID: podID1, Node: node},
		&health.Result{ID: podID2, Node: node},
	}

	healthResults <- []*health.Result{
		&health.Result{ID: podID1, Node: node},
		&health.Result{ID: podID2, Node: node},
	}

	result = hs.Fetch(podID1, node)
	if result == nil {
		t.Errorf("expected health store to have %s", podID1)
	}

	result = hs.Fetch(podID2, node)
	if result == nil {
		t.Errorf("expected health store to have %s", podID2)
	}
}
