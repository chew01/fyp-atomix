package leadership

import (
	"context"
	"log"
	"prototype/controller/device"
	"reflect"
	"sync"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
	"github.com/atomix/go-sdk/pkg/primitive/election"
)

type activeElection struct {
	cancel   context.CancelFunc
	election election.Election
}

type ElectionManager struct {
	ctx      context.Context
	hostname string
	mu       sync.Mutex
	active   map[string]activeElection
}

func NewElectionManager(ctx context.Context, hostname string) *ElectionManager {
	return &ElectionManager{
		ctx:      ctx,
		hostname: hostname,
		active:   make(map[string]activeElection),
	}
}

func (m *ElectionManager) StartElection(deviceID string, dev *device.Device) {
	m.mu.Lock()
	if _, exists := m.active[deviceID]; exists {
		// Election already running
		return
	}
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(m.ctx)
	e, err := atomix.LeaderElection("election-" + dev.ID).
		CandidateID(m.hostname).
		Get(ctx)
	if err != nil {
		log.Printf("[Election] (%s) Failed to create election: %v", dev.ID, err)
		cancel()
		return
	}

	m.mu.Lock()
	m.active[deviceID] = activeElection{
		cancel:   cancel,
		election: e,
	}
	m.mu.Unlock()

	go m.runElection(ctx, dev, e)
}

func (m *ElectionManager) StopElection(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ae, exists := m.active[deviceID]; exists {
		ae.cancel()
		if _, err := ae.election.Evict(m.ctx, m.hostname); err != nil {
			log.Printf("[Leadership] Failed to evict %s from election %s: %v", m.hostname, deviceID, err)
		}
		delete(m.active, deviceID)
	}
}

func (m *ElectionManager) StopAllElectionsForHostname(hostname string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for deviceID, ae := range m.active {
		log.Printf("[Leadership] Evicting %s from election %s", hostname, deviceID)
		if _, err := ae.election.Evict(m.ctx, hostname); err != nil {
			log.Printf("[Leadership] Failed to evict %s from election %s: %v", hostname, deviceID, err)
		}
	}
}

func (m *ElectionManager) runElection(ctx context.Context, dev *device.Device, e election.Election) {
	electionName := e.Name()

	// Join election
	if _, err := e.Enter(ctx); err != nil {
		log.Printf("[Leadership] (%s) Failed to enter election: %v", electionName, err)
		return
	}
	log.Printf("[Leadership] (%s) Entered election", electionName)

	// Watch election
	stream, err := e.Watch(ctx)
	if err != nil {
		log.Printf("[Leadership] (%s) Failed to watch election: %v", electionName, err)
		return
	}

	// Distributed config map
	configMap, err := atomix.Map[string, string]("config").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Printf("[Leadership] (%s) Error accessing config map: %v", electionName, err)
		return
	}
	defer configMap.Close(ctx)

	var cache *election.Term
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Leadership] (%s) Stopping election", electionName)
			return
		default:
		}

		term, err := stream.Next()
		if err != nil {
			log.Printf("[Leadership] (%s) Error in election stream: %v", electionName, err)
			time.Sleep(time.Second)
			continue
		}

		if cache == nil || cache.ID != term.ID {
			log.Printf("[Leadership] (%s) New term: %d", electionName, term.ID)
		}
		if cache == nil || !reflect.DeepEqual(cache.Candidates, term.Candidates) {
			log.Printf("[Leadership] (%s) Candidates: %v", electionName, term.Candidates)
		}

		if cache == nil || cache.Leader != term.Leader {
			if term.Leader == e.CandidateID() {
				log.Printf("[Leadership] (%s) ✅ I am leader (term %d)", electionName, term.ID)
				value := "leader " + m.hostname
				_, _ = configMap.Put(ctx, electionName, value)
				config := map[string]string{"flow": "allow all", "version": time.Now().Format(time.RFC3339)}
				dev.ApplyConfig(ctx, config)
			} else {
				log.Printf("[Leadership] (%s) ℹ️ Current leader: %s", electionName, term.Leader)
			}
		}
		cache = term
	}
}
