package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type FailoverTestScenario int

const (
	ImmediateFailure FailoverTestScenario = iota
	DuringReplication
	RapidSequential
	PrecisionTimed
)

type ReadTestMode int

const (
	ImmediateRead ReadTestMode = iota
	PostRecoveryRead
)

type TestExecutionPhase int

const (
	PhaseSetup TestExecutionPhase = iota
	PhaseWrite
	PhaseFailure
	PhaseRecovery
	PhaseVerification
	PhaseComplete
)

type LeaderInfo struct {
	PartitionID int
	PodName     string
	PodIndex    int
	Term        int64
	State       string
	LastUpdate  time.Time
}

type TestResult struct {
	TestID            string
	Scenario          FailoverTestScenario
	ReadMode          ReadTestMode
	Key               string
	Value             string
	WriteTime         time.Time
	FailureTime       time.Time
	RecoveryTime      time.Time
	VerificationTime  time.Time
	Success           bool
	Error             string
	Duration          time.Duration
	LeaderBefore      LeaderInfo
	LeaderAfter       LeaderInfo
	ImmediateReadErr  string
	ImmediateReadTime time.Time
}

type EnhancedFailoverTest struct {
	k8sClient      *kubernetes.Clientset
	dynamicClient  dynamic.Interface
	logFile        *os.File
	namespace      string
	testCounter    int64
	results        []TestResult
	resultsMux     sync.RWMutex
	leaderCache    map[int]LeaderInfo
	leaderMux      sync.RWMutex
	partitionCount int
}

func NewEnhancedFailoverTest() (*EnhancedFailoverTest, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	logFileName := getEnv("LOG_FILE", "enhanced-failover-test-results.log")
	namespace := getEnv("NAMESPACE", "default")

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	return &EnhancedFailoverTest{
		k8sClient:      clientset,
		dynamicClient:  dynamicClient,
		logFile:        logFile,
		namespace:      namespace,
		leaderCache:    make(map[int]LeaderInfo),
		partitionCount: 3,
	}, nil
}

func (eft *EnhancedFailoverTest) logMessage(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)
	fmt.Print(logEntry)
	eft.logFile.WriteString(logEntry)
	eft.logFile.Sync()
}

func (eft *EnhancedFailoverTest) updateLeaderInfo(ctx context.Context) error {
	raftGroupGVR := schema.GroupVersionResource{
		Group:    "consensus.atomix.io",
		Version:  "v1beta1",
		Resource: "raftgroups",
	}

	raftGroups, err := eft.dynamicClient.Resource(raftGroupGVR).Namespace(eft.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "atomix.io/store=consensus-store",
	})
	if err != nil {
		return fmt.Errorf("failed to list RaftGroups: %v", err)
	}

	eft.leaderMux.Lock()
	defer eft.leaderMux.Unlock()

	for _, item := range raftGroups.Items {
		groupName := item.GetName()
		partNum, err := strconv.Atoi(strings.Split(groupName, "-")[2])
		if err != nil {
			continue
		}

		status, found, err := unstructured.NestedMap(item.Object, "status")
		if err != nil || !found {
			continue
		}

		leader, found, err := unstructured.NestedMap(status, "leader")
		if err != nil || !found {
			eft.leaderCache[partNum] = LeaderInfo{
				PartitionID: partNum,
				State:       "no-leader",
				LastUpdate:  time.Now(),
			}
			continue
		}

		leaderName, found, err := unstructured.NestedString(leader, "name")
		if err != nil || !found {
			continue
		}

		term, found, err := unstructured.NestedInt64(status, "term")
		if err != nil || !found {
			term = 0
		}

		state, found, err := unstructured.NestedString(status, "state")
		if err != nil || !found {
			state = "unknown"
		}

		podIndex := -1
		if strings.Contains(leaderName, "consensus-store-") {
			if parts := strings.Split(leaderName, "-"); len(parts) >= 4 {
				if idx, err := strconv.Atoi(parts[3]); err == nil {
					podIndex = idx
				}
			}
		}

		eft.leaderCache[partNum] = LeaderInfo{
			PartitionID: partNum,
			PodName:     leaderName,
			PodIndex:    podIndex,
			Term:        term,
			State:       state,
			LastUpdate:  time.Now(),
		}
	}

	return nil
}

func (eft *EnhancedFailoverTest) getLeaderForPartition(partitionID int) (LeaderInfo, bool) {
	eft.leaderMux.RLock()
	defer eft.leaderMux.RUnlock()

	leader, exists := eft.leaderCache[partitionID]
	return leader, exists
}

func (eft *EnhancedFailoverTest) getKeyPartition(key string) int {
	hash := sha256.Sum256([]byte(key))
	return int(hash[0]) % eft.partitionCount
}

func (eft *EnhancedFailoverTest) terminateLeaderPod(ctx context.Context, leader LeaderInfo) error {
	if leader.PodName == "" {
		return fmt.Errorf("no leader pod to terminate")
	}

	podNum, err := strconv.Atoi(strings.Split(leader.PodName, "-")[3])
	if err != nil {
		return fmt.Errorf("failed to parse pod number: %v", err)
	}

	podName := "consensus-store-" + strconv.Itoa(podNum-1)

	eft.logMessage(fmt.Sprintf("FORCED_TERMINATION: Terminating leader pod %s for partition %d", podName, leader.PartitionID))

	err = eft.k8sClient.CoreV1().Pods(eft.namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: new(int64),
	})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %v", podName, err)
	}

	eft.logMessage(fmt.Sprintf("TERMINATION_SUCCESS: Pod %s terminated", podName))
	return nil
}

func (eft *EnhancedFailoverTest) waitForLeaderElection(ctx context.Context, partitionID int, originalTerm int64) (LeaderInfo, error) {
	timeout := time.NewTimer(60 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer timeout.Stop()
	defer ticker.Stop()

	eft.logMessage(fmt.Sprintf("LEADER_ELECTION_WAIT: Waiting for new leader on partition %d (original term: %d)", partitionID, originalTerm))

	for {
		select {
		case <-timeout.C:
			return LeaderInfo{}, fmt.Errorf("timeout waiting for leader election on partition %d after 60 seconds", partitionID)
		case <-ctx.Done():
			return LeaderInfo{}, ctx.Err()
		case <-ticker.C:
			err := eft.updateLeaderInfo(ctx)
			if err != nil {
				continue
			}

			leader, exists := eft.getLeaderForPartition(partitionID)
			if exists && leader.State == "Ready" && leader.Term > originalTerm {
				eft.logMessage(fmt.Sprintf("NEW_LEADER_ELECTED: Partition %d, Pod %s, Term %d (waiting for system stabilization...)",
					partitionID, leader.PodName, leader.Term))
				
				time.Sleep(2 * time.Second)
				
				err = eft.updateLeaderInfo(ctx)
				if err != nil {
					eft.logMessage(fmt.Sprintf("LEADER_STABILITY_CHECK_FAILED: Error updating leader info: %v", err))
					continue
				}

				reconfirmedLeader, stillExists := eft.getLeaderForPartition(partitionID)
				if stillExists && reconfirmedLeader.State == "Ready" && reconfirmedLeader.Term == leader.Term && reconfirmedLeader.PodName == leader.PodName {
					eft.logMessage(fmt.Sprintf("LEADER_READY: Partition %d leader %s is stable and ready for reads",
						partitionID, reconfirmedLeader.PodName))
					return reconfirmedLeader, nil
				} else {
					eft.logMessage(fmt.Sprintf("LEADER_INSTABILITY: Leader state changed during stabilization check, continuing to wait..."))
				}
			}
		}
	}
}

func (eft *EnhancedFailoverTest) waitForReadyLeader(ctx context.Context, partitionID int) (LeaderInfo, error) {
	timeout := time.NewTimer(45 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer timeout.Stop()
	defer ticker.Stop()

	eft.logMessage(fmt.Sprintf("READY_LEADER_WAIT: Waiting for ready leader on partition %d", partitionID))
	
	err := eft.updateLeaderInfo(ctx)
	if err == nil {
		leader, exists := eft.getLeaderForPartition(partitionID)
		if exists && leader.State == "Ready" && leader.PodName != "" {
			eft.logMessage(fmt.Sprintf("READY_LEADER_IMMEDIATE: Partition %d already has ready leader %s, Term %d",
				partitionID, leader.PodName, leader.Term))
			return leader, nil
		}
	}

	for {
		select {
		case <-timeout.C:
			return LeaderInfo{}, fmt.Errorf("timeout waiting for ready leader on partition %d after 45 seconds", partitionID)
		case <-ctx.Done():
			return LeaderInfo{}, ctx.Err()
		case <-ticker.C:
			err := eft.updateLeaderInfo(ctx)
			if err != nil {
				continue
			}

			leader, exists := eft.getLeaderForPartition(partitionID)
			if exists && leader.State == "Ready" && leader.PodName != "" {
				eft.logMessage(fmt.Sprintf("READY_LEADER_FOUND: Partition %d has ready leader %s, Term %d",
					partitionID, leader.PodName, leader.Term))
				return leader, nil
			}
		}
	}
}

func (eft *EnhancedFailoverTest) executeImmediateReadTest(ctx context.Context, scenario FailoverTestScenario, delay time.Duration) TestResult {
	testID := fmt.Sprintf("imm-test-%06d", atomic.AddInt64(&eft.testCounter, 1))

	result := TestResult{
		TestID:   testID,
		Scenario: scenario,
		ReadMode: ImmediateRead,
	}

	eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_TEST_START: %s, Scenario: %v, Delay: %v", testID, scenario, delay))

	testMap, err := atomix.Map[string, string]("precision-test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get map instance: %v", err)
		return result
	}

	result.Key = fmt.Sprintf("immediate-key-%s", testID)
	result.Value = fmt.Sprintf("immediate-value-%s-%d", testID, time.Now().UnixNano())

	partitionID := eft.getKeyPartition(result.Key)

	leader, err := eft.waitForReadyLeader(ctx, partitionID)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to find ready leader: %v", err)
		return result
	}
	result.LeaderBefore = leader

	eft.logMessage(fmt.Sprintf("IMMEDIATE_WRITE: %s -> %s (partition %d, leader %s)",
		result.Key, result.Value, partitionID, leader.PodName))

	result.WriteTime = time.Now()
	_, err = testMap.Put(ctx, result.Key, result.Value)
	if err != nil {
		result.Error = fmt.Sprintf("Write failed: %v", err)
		return result
	}

	writeDuration := time.Since(result.WriteTime)
	eft.logMessage(fmt.Sprintf("WRITE_COMPLETE: %s (duration: %v)", testID, writeDuration))

	if delay > 0 {
		eft.logMessage(fmt.Sprintf("IMMEDIATE_DELAY: Waiting %v before termination", delay))
		time.Sleep(delay)
	}

	result.FailureTime = time.Now()
	err = eft.terminateLeaderPod(ctx, leader)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to terminate leader: %v", err)
		return result
	}

	eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_ATTEMPT: Trying immediate read after leader termination"))
	result.ImmediateReadTime = time.Now()

	immediateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	entry, err := testMap.Get(immediateCtx, result.Key)
	if err != nil {
		result.ImmediateReadErr = fmt.Sprintf("Immediate read failed: %v", err)
		eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_FAILED: %s - %s", testID, result.ImmediateReadErr))
	} else if entry == nil {
		result.ImmediateReadErr = "Key not found during immediate read"
		eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_FAILED: %s - Key not found", testID))
	} else if entry.Value != result.Value {
		result.ImmediateReadErr = fmt.Sprintf("Immediate read value mismatch: got '%s', expected '%s'", entry.Value, result.Value)
		eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_FAILED: %s - Value mismatch", testID))
	} else {
		eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_SUCCESS: %s - Data readable immediately after failure", testID))
	}

	newLeader, err := eft.waitForLeaderElection(ctx, partitionID, leader.Term)
	if err != nil {
		result.Error = fmt.Sprintf("Leader election failed: %v", err)
		return result
	}
	result.LeaderAfter = newLeader
	result.RecoveryTime = time.Now()

	recoveryDuration := result.RecoveryTime.Sub(result.FailureTime)
	eft.logMessage(fmt.Sprintf("LEADER_RECOVERY: %s (duration: %v)", testID, recoveryDuration))

	eft.logMessage(fmt.Sprintf("POST_RECOVERY_WAIT: %s - Waiting for system stabilization before verification read", testID))
	time.Sleep(1 * time.Second)

	result.VerificationTime = time.Now()
	
	verificationCtx, verificationCancel := context.WithTimeout(ctx, 10*time.Second)
	defer verificationCancel()

	eft.logMessage(fmt.Sprintf("POST_RECOVERY_READ: %s - Attempting verification read", testID))
	entry, err = testMap.Get(verificationCtx, result.Key)
	if err != nil {
		result.Error = fmt.Sprintf("Post-recovery read failed: %v", err)
		return result
	}

	if entry == nil {
		result.Error = "Key not found during post-recovery verification"
		return result
	}

	if entry.Value != result.Value {
		result.Error = fmt.Sprintf("Post-recovery value mismatch: got '%s', expected '%s'", entry.Value, result.Value)
		return result
	}

	result.Success = true
	result.Duration = time.Since(result.WriteTime)
	
	eft.logMessage(fmt.Sprintf("POST_RECOVERY_READ_SUCCESS: %s - Data verified successfully after recovery", testID))

	immediateStatus := "failed"
	if result.ImmediateReadErr == "" {
		immediateStatus = "success"
	}

	eft.logMessage(fmt.Sprintf("IMMEDIATE_TEST_COMPLETE: %s - Post-recovery: success, Immediate: %s (total duration: %v)",
		testID, immediateStatus, result.Duration))

	return result
}

func (eft *EnhancedFailoverTest) executePrecisionFailoverTest(ctx context.Context, scenario FailoverTestScenario, delay time.Duration) TestResult {
	testID := fmt.Sprintf("test-%06d", atomic.AddInt64(&eft.testCounter, 1))

	result := TestResult{
		TestID:   testID,
		Scenario: scenario,
		ReadMode: PostRecoveryRead,
	}

	eft.logMessage(fmt.Sprintf("PRECISION_TEST_START: %s, Scenario: %v, Delay: %v", testID, scenario, delay))

	testMap, err := atomix.Map[string, string]("precision-test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get map instance: %v", err)
		return result
	}

	result.Key = fmt.Sprintf("precision-key-%s", testID)
	result.Value = fmt.Sprintf("precision-value-%s-%d", testID, time.Now().UnixNano())

	partitionID := eft.getKeyPartition(result.Key)

	leader, err := eft.waitForReadyLeader(ctx, partitionID)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to find ready leader: %v", err)
		return result
	}
	result.LeaderBefore = leader

	eft.logMessage(fmt.Sprintf("PRECISION_WRITE: %s -> %s (partition %d, leader %s)",
		result.Key, result.Value, partitionID, leader.PodName))

	result.WriteTime = time.Now()
	_, err = testMap.Put(ctx, result.Key, result.Value)
	if err != nil {
		result.Error = fmt.Sprintf("Write failed: %v", err)
		return result
	}

	writeDuration := time.Since(result.WriteTime)
	eft.logMessage(fmt.Sprintf("WRITE_COMPLETE: %s (duration: %v)", testID, writeDuration))

	if delay > 0 {
		eft.logMessage(fmt.Sprintf("PRECISION_DELAY: Waiting %v before termination", delay))
		time.Sleep(delay)
	}

	result.FailureTime = time.Now()
	err = eft.terminateLeaderPod(ctx, leader)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to terminate leader: %v", err)
		return result
	}

	newLeader, err := eft.waitForLeaderElection(ctx, partitionID, leader.Term)
	if err != nil {
		result.Error = fmt.Sprintf("Leader election failed: %v", err)
		return result
	}
	result.LeaderAfter = newLeader
	result.RecoveryTime = time.Now()

	recoveryDuration := result.RecoveryTime.Sub(result.FailureTime)
	eft.logMessage(fmt.Sprintf("LEADER_RECOVERY: %s (duration: %v)", testID, recoveryDuration))

	eft.logMessage(fmt.Sprintf("POST_RECOVERY_WAIT: %s - Waiting for system stabilization before verification read", testID))
	time.Sleep(1 * time.Second)

	result.VerificationTime = time.Now()
	
	verificationCtx, verificationCancel := context.WithTimeout(ctx, 10*time.Second)
	defer verificationCancel()

	eft.logMessage(fmt.Sprintf("POST_RECOVERY_READ: %s - Attempting verification read", testID))
	entry, err := testMap.Get(verificationCtx, result.Key)
	if err != nil {
		result.Error = fmt.Sprintf("Verification read failed: %v", err)
		return result
	}

	if entry == nil {
		result.Error = "Key not found during verification"
		return result
	}

	if entry.Value != result.Value {
		result.Error = fmt.Sprintf("Value mismatch: got '%s', expected '%s'", entry.Value, result.Value)
		return result
	}

	result.Success = true
	result.Duration = time.Since(result.WriteTime)

	eft.logMessage(fmt.Sprintf("POST_RECOVERY_READ_SUCCESS: %s - Data verified successfully after recovery", testID))
	eft.logMessage(fmt.Sprintf("PRECISION_SUCCESS: %s verified (total duration: %v)", testID, result.Duration))

	return result
}

func (eft *EnhancedFailoverTest) runComprehensiveFailoverTests(ctx context.Context) error {
	eft.logMessage("COMPREHENSIVE_TESTS_START: Enhanced failover testing with immediate and post-recovery read modes")

	scenarios := []struct {
		scenario FailoverTestScenario
		name     string
		delays   []time.Duration
	}{
		{ImmediateFailure, "Immediate Failure", []time.Duration{0}},
		{DuringReplication, "During Replication", []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}},
		{PrecisionTimed, "Precision Timed", []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 75 * time.Millisecond}},
		{RapidSequential, "Rapid Sequential", []time.Duration{0, 0, 0}},
	}

	totalTests := 0
	successfulTests := 0

	for _, scenario := range scenarios {
		eft.logMessage(fmt.Sprintf("SCENARIO_START: %s", scenario.name))

		for i, delay := range scenario.delays {
			if scenario.scenario == RapidSequential && i > 0 {
				time.Sleep(500 * time.Millisecond)
			}

			eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_MODE: Testing immediate read capability for %s with delay %v", scenario.name, delay))
			immediateResult := eft.executeImmediateReadTest(ctx, scenario.scenario, delay)

			eft.resultsMux.Lock()
			eft.results = append(eft.results, immediateResult)
			eft.resultsMux.Unlock()

			totalTests++
			if immediateResult.Success {
				successfulTests++
			}

			immediateStatus := "failed"
			if immediateResult.ImmediateReadErr == "" {
				immediateStatus = "success"
			}

			eft.logMessage(fmt.Sprintf("IMMEDIATE_TEST_RESULT: %s - Post-recovery: %v, Immediate: %s, Error: %s",
				immediateResult.TestID, immediateResult.Success, immediateStatus, immediateResult.Error))

			time.Sleep(3 * time.Second)

			eft.logMessage(fmt.Sprintf("POST_RECOVERY_MODE: Testing post-recovery read for %s with delay %v", scenario.name, delay))
			postRecoveryResult := eft.executePrecisionFailoverTest(ctx, scenario.scenario, delay)

			eft.resultsMux.Lock()
			eft.results = append(eft.results, postRecoveryResult)
			eft.resultsMux.Unlock()

			totalTests++
			if postRecoveryResult.Success {
				successfulTests++
			}

			eft.logMessage(fmt.Sprintf("POST_RECOVERY_RESULT: %s - Success: %v, Error: %s",
				postRecoveryResult.TestID, postRecoveryResult.Success, postRecoveryResult.Error))

			time.Sleep(2 * time.Second)
		}

		eft.logMessage(fmt.Sprintf("SCENARIO_COMPLETE: %s", scenario.name))
		time.Sleep(5 * time.Second)
	}

	eft.logMessage(fmt.Sprintf("COMPREHENSIVE_TESTS_COMPLETE: %d/%d tests successful (%.1f%%)",
		successfulTests, totalTests, float64(successfulTests)/float64(totalTests)*100))

	return eft.generateEnhancedReport()
}

func (eft *EnhancedFailoverTest) runPrecisionFailoverTests(ctx context.Context) error {
	eft.logMessage("PRECISION_TESTS_START: Enhanced failover testing with precise timing (post-recovery mode only)")

	scenarios := []struct {
		scenario FailoverTestScenario
		name     string
		delays   []time.Duration
	}{
		{ImmediateFailure, "Immediate Failure", []time.Duration{0}},
		{DuringReplication, "During Replication", []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}},
		{PrecisionTimed, "Precision Timed", []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 75 * time.Millisecond}},
		{RapidSequential, "Rapid Sequential", []time.Duration{0, 0, 0}},
	}

	totalTests := 0
	successfulTests := 0

	for _, scenario := range scenarios {
		eft.logMessage(fmt.Sprintf("SCENARIO_START: %s", scenario.name))

		for i, delay := range scenario.delays {
			if scenario.scenario == RapidSequential && i > 0 {
				time.Sleep(500 * time.Millisecond)
			}

			result := eft.executePrecisionFailoverTest(ctx, scenario.scenario, delay)

			eft.resultsMux.Lock()
			eft.results = append(eft.results, result)
			eft.resultsMux.Unlock()

			totalTests++
			if result.Success {
				successfulTests++
			}

			eft.logMessage(fmt.Sprintf("TEST_RESULT: %s - Success: %v, Error: %s",
				result.TestID, result.Success, result.Error))

			time.Sleep(2 * time.Second)
		}

		eft.logMessage(fmt.Sprintf("SCENARIO_COMPLETE: %s", scenario.name))
		time.Sleep(5 * time.Second)
	}

	eft.logMessage(fmt.Sprintf("PRECISION_TESTS_COMPLETE: %d/%d tests successful (%.1f%%)",
		successfulTests, totalTests, float64(successfulTests)/float64(totalTests)*100))

	return eft.generateDetailedReport()
}

func (eft *EnhancedFailoverTest) generateEnhancedReport() error {
	eft.resultsMux.RLock()
	defer eft.resultsMux.RUnlock()

	eft.logMessage("=== COMPREHENSIVE TEST REPORT: IMMEDIATE vs POST-RECOVERY READS ===")

	type ScenarioModeStats struct {
		total                int
		success              int
		avgDuration          time.Duration
		immediateReadSuccess int
		immediateReadTotal   int
	}

	scenarioModeStats := make(map[FailoverTestScenario]map[ReadTestMode]*ScenarioModeStats)

	for _, result := range eft.results {
		if scenarioModeStats[result.Scenario] == nil {
			scenarioModeStats[result.Scenario] = make(map[ReadTestMode]*ScenarioModeStats)
		}
		if scenarioModeStats[result.Scenario][result.ReadMode] == nil {
			scenarioModeStats[result.Scenario][result.ReadMode] = &ScenarioModeStats{}
		}

		stats := scenarioModeStats[result.Scenario][result.ReadMode]
		stats.total++
		if result.Success {
			stats.success++
			stats.avgDuration += result.Duration
		}

		if result.ReadMode == ImmediateRead {
			stats.immediateReadTotal++
			if result.ImmediateReadErr == "" {
				stats.immediateReadSuccess++
			}
		}
	}

	scenarioNames := map[FailoverTestScenario]string{
		ImmediateFailure:  "Immediate Failure",
		DuringReplication: "During Replication",
		RapidSequential:   "Rapid Sequential",
		PrecisionTimed:    "Precision Timed",
	}

	readModeNames := map[ReadTestMode]string{
		ImmediateRead:    "Immediate Read",
		PostRecoveryRead: "Post-Recovery Read",
	}

	eft.logMessage("=== SCENARIO COMPARISON SUMMARY ===")
	for scenario, modeStats := range scenarioModeStats {
		scenarioName := scenarioNames[scenario]
		eft.logMessage(fmt.Sprintf("SCENARIO: %s", scenarioName))

		for mode, stats := range modeStats {
			modeName := readModeNames[mode]
			successRate := float64(stats.success) / float64(stats.total) * 100
			var avgDuration time.Duration
			if stats.success > 0 {
				avgDuration = stats.avgDuration / time.Duration(stats.success)
			}

			if mode == ImmediateRead {
				immediateSuccessRate := float64(stats.immediateReadSuccess) / float64(stats.immediateReadTotal) * 100
				eft.logMessage(fmt.Sprintf("  %s: %d/%d post-recovery success (%.1f%%), %d/%d immediate success (%.1f%%), Avg Duration: %v",
					modeName, stats.success, stats.total, successRate, stats.immediateReadSuccess, stats.immediateReadTotal, immediateSuccessRate, avgDuration))
			} else {
				eft.logMessage(fmt.Sprintf("  %s: %d/%d success (%.1f%%), Avg Duration: %v",
					modeName, stats.success, stats.total, successRate, avgDuration))
			}
		}
		eft.logMessage("")
	}

	eft.logMessage("=== READ MODE ANALYSIS ===")
	immediateReadTests := 0
	immediateReadSuccess := 0
	immediateDataSuccess := 0
	postRecoveryTests := 0
	postRecoverySuccess := 0

	for _, result := range eft.results {
		if result.ReadMode == ImmediateRead {
			immediateReadTests++
			if result.Success {
				immediateReadSuccess++
			}
			if result.ImmediateReadErr == "" {
				immediateDataSuccess++
			}
		} else {
			postRecoveryTests++
			if result.Success {
				postRecoverySuccess++
			}
		}
	}

	immediatePostRecoveryRate := float64(immediateReadSuccess) / float64(immediateReadTests) * 100
	immediateDataRate := float64(immediateDataSuccess) / float64(immediateReadTests) * 100
	postRecoveryRate := float64(postRecoverySuccess) / float64(postRecoveryTests) * 100

	eft.logMessage(fmt.Sprintf("IMMEDIATE_READ_ANALYSIS: %d/%d tests with post-recovery success (%.1f%%), %d/%d with immediate data access (%.1f%%)",
		immediateReadSuccess, immediateReadTests, immediatePostRecoveryRate, immediateDataSuccess, immediateReadTests, immediateDataRate))

	eft.logMessage(fmt.Sprintf("POST_RECOVERY_ANALYSIS: %d/%d tests successful (%.1f%%)",
		postRecoverySuccess, postRecoveryTests, postRecoveryRate))

	eft.logMessage(fmt.Sprintf("COMPARATIVE_ANALYSIS: Data durability proven in %.1f%% of immediate tests and %.1f%% of post-recovery tests",
		immediatePostRecoveryRate, postRecoveryRate))

	eft.logMessage("=== INDIVIDUAL TEST DETAILS ===")
	for _, result := range eft.results {
		scenarioName := scenarioNames[result.Scenario]
		modeName := readModeNames[result.ReadMode]
		recoveryTime := result.RecoveryTime.Sub(result.FailureTime)

		if result.ReadMode == ImmediateRead {
			immediateStatus := "failed"
			if result.ImmediateReadErr == "" {
				immediateStatus = "success"
			}
			eft.logMessage(fmt.Sprintf("TEST_DETAIL: %s (%s - %s) - Post-recovery: %v, Immediate: %s, Duration: %v, Recovery: %v",
				result.TestID, scenarioName, modeName, result.Success, immediateStatus, result.Duration, recoveryTime))
			if result.ImmediateReadErr != "" {
				eft.logMessage(fmt.Sprintf("  IMMEDIATE_ERROR: %s", result.ImmediateReadErr))
			}
		} else {
			eft.logMessage(fmt.Sprintf("TEST_DETAIL: %s (%s - %s) - Success: %v, Duration: %v, Recovery: %v",
				result.TestID, scenarioName, modeName, result.Success, result.Duration, recoveryTime))
		}

		if !result.Success {
			eft.logMessage(fmt.Sprintf("  POST_RECOVERY_ERROR: %s", result.Error))
		}

		eft.logMessage(fmt.Sprintf("  LEADERS: Before: %s (P%d), After: %s (P%d)",
			result.LeaderBefore.PodName, result.LeaderBefore.PartitionID,
			result.LeaderAfter.PodName, result.LeaderAfter.PartitionID))
	}

	return nil
}

func (eft *EnhancedFailoverTest) generateDetailedReport() error {
	eft.resultsMux.RLock()
	defer eft.resultsMux.RUnlock()

	eft.logMessage("=== DETAILED TEST REPORT ===")

	scenarioStats := make(map[FailoverTestScenario]struct {
		total       int
		success     int
		avgDuration time.Duration
	})

	for _, result := range eft.results {
		stats := scenarioStats[result.Scenario]
		stats.total++
		if result.Success {
			stats.success++
			stats.avgDuration += result.Duration
		}
		scenarioStats[result.Scenario] = stats
	}

	scenarioNames := map[FailoverTestScenario]string{
		ImmediateFailure:  "Immediate Failure",
		DuringReplication: "During Replication",
		RapidSequential:   "Rapid Sequential",
		PrecisionTimed:    "Precision Timed",
	}

	for scenario, stats := range scenarioStats {
		name := scenarioNames[scenario]
		successRate := float64(stats.success) / float64(stats.total) * 100
		var avgDuration time.Duration
		if stats.success > 0 {
			avgDuration = stats.avgDuration / time.Duration(stats.success)
		}

		eft.logMessage(fmt.Sprintf("SCENARIO_SUMMARY: %s - %d/%d successful (%.1f%%), Avg Duration: %v",
			name, stats.success, stats.total, successRate, avgDuration))
	}

	eft.logMessage("=== INDIVIDUAL TEST DETAILS ===")
	for _, result := range eft.results {
		scenarioName := scenarioNames[result.Scenario]
		recoveryTime := result.RecoveryTime.Sub(result.FailureTime)

		eft.logMessage(fmt.Sprintf("TEST_DETAIL: %s (%s) - Success: %v, Duration: %v, Recovery: %v",
			result.TestID, scenarioName, result.Success, result.Duration, recoveryTime))

		if !result.Success {
			eft.logMessage(fmt.Sprintf("  ERROR: %s", result.Error))
		}

		eft.logMessage(fmt.Sprintf("  LEADERS: Before: %s (P%d), After: %s (P%d)",
			result.LeaderBefore.PodName, result.LeaderBefore.PartitionID,
			result.LeaderAfter.PodName, result.LeaderAfter.PartitionID))
	}

	return nil
}

func (eft *EnhancedFailoverTest) Close() {
	if eft.logFile != nil {
		eft.logFile.Close()
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	enhancedTest, err := NewEnhancedFailoverTest()
	if err != nil {
		log.Fatalf("Failed to initialize enhanced failover test: %v", err)
	}
	defer enhancedTest.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		enhancedTest.logMessage("INTERRUPT: Received interrupt signal, shutting down gracefully...")
		cancel()
	}()

	testMap, err := atomix.Map[string, string]("precision-test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		enhancedTest.logMessage(fmt.Sprintf("FATAL: Failed to initialize test map: %v", err))
		os.Exit(1)
	}

	initialKey := "connectivity-test"
	initialValue := fmt.Sprintf("initialized-%d", time.Now().Unix())
	_, err = testMap.Put(ctx, initialKey, initialValue)
	if err != nil {
		enhancedTest.logMessage(fmt.Sprintf("FATAL: Failed initial connectivity test: %v", err))
		os.Exit(1)
	}

	entry, err := testMap.Get(ctx, initialKey)
	if err != nil || entry == nil || entry.Value != initialValue {
		enhancedTest.logMessage("FATAL: Initial connectivity verification failed")
		os.Exit(1)
	}

	enhancedTest.logMessage("CONNECTIVITY: Initial connectivity and consistency verified")

	testMode := getEnv("TEST_MODE", "comprehensive")

	switch testMode {
	case "precision":
		enhancedTest.logMessage("TEST_MODE: Running precision failover tests (post-recovery only)")
		if err := enhancedTest.runPrecisionFailoverTests(ctx); err != nil {
			enhancedTest.logMessage(fmt.Sprintf("FATAL: Precision tests failed: %v", err))
			os.Exit(1)
		}
		enhancedTest.logMessage("EXPERIMENT_COMPLETE: Precision failover testing completed successfully")
	case "comprehensive":
		enhancedTest.logMessage("TEST_MODE: Running comprehensive failover tests (immediate + post-recovery)")
		if err := enhancedTest.runComprehensiveFailoverTests(ctx); err != nil {
			enhancedTest.logMessage(fmt.Sprintf("FATAL: Comprehensive tests failed: %v", err))
			os.Exit(1)
		}
		enhancedTest.logMessage("EXPERIMENT_COMPLETE: Comprehensive failover testing with immediate and post-recovery reads completed successfully")
	default:
		enhancedTest.logMessage(fmt.Sprintf("FATAL: Invalid TEST_MODE '%s'. Valid options: 'precision', 'comprehensive'", testMode))
		os.Exit(1)
	}
}
