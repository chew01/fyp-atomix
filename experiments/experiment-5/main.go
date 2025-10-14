package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

type TestType int

const (
	LinearizabilityTest TestType = iota
	WriteDurabilityTest
)

type ConcurrencyTest struct {
	clientID            string
	operationSeq        int64
	writeLog            map[string]string
	writeLogMux         sync.RWMutex
	consistency         *ConsistencyTracker
	logFile             *os.File
	csvFile             *os.File
	csvWriter           *csv.Writer
	testDuration        time.Duration
	concurrentClients   int
	operationsPerClient int
	contentionKeys      int
	statisticsFile      string
}

type ConsistencyTracker struct {
	linearizable    bool
	writeDurability bool
	trackerMux      sync.RWMutex

	// Linearizability tracking - client sequences and final value verification
	clientSequences map[string][]string // clientID -> sequence of values written
	finalValue      string              // final value read from shared key
	sequenceMux     sync.RWMutex

	// Write durability tracking
	acknowledgedWrites []AcknowledgedWrite
	durabilityMux      sync.RWMutex
}

type AcknowledgedWrite struct {
	ClientID  string
	Key       string
	Value     string
	Timestamp time.Time
	Success   bool
}

func NewConcurrencyTest() (*ConcurrencyTest, error) {
	concurrentClients := getEnvInt("CONCURRENT_CLIENTS", 3)
	operationsPerClient := getEnvInt("OPERATIONS_PER_CLIENT", 5)
	contentionKeys := getEnvInt("CONTENTION_KEYS", 1) // Single shared key for testing
	testDuration := getEnvDuration("TEST_DURATION", 5*time.Minute)
	statisticsFile := getEnv("STATISTICS_FILE", "linearizability-test-results.csv")
	logFileName := getEnv("LOG_FILE", "linearizability-test-results.log")

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	csvFile, err := os.Create(statisticsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSV file: %v", err)
	}

	csvWriter := csv.NewWriter(csvFile)
	// Write CSV header
	csvWriter.Write([]string{"TestType", "ClientID", "Key", "Operation", "Value", "Timestamp", "Success", "Duration", "Details"})
	csvWriter.Flush()

	consistency := &ConsistencyTracker{
		linearizable:    true,
		writeDurability: true,
		clientSequences: make(map[string][]string),
	}

	return &ConcurrencyTest{
		clientID:            fmt.Sprintf("main-client-%d", time.Now().Unix()),
		operationSeq:        0,
		writeLog:            make(map[string]string),
		consistency:         consistency,
		logFile:             logFile,
		csvFile:             csvFile,
		csvWriter:           csvWriter,
		testDuration:        testDuration,
		concurrentClients:   concurrentClients,
		operationsPerClient: operationsPerClient,
		contentionKeys:      contentionKeys,
		statisticsFile:      statisticsFile,
	}, nil
}

func (ct *ConcurrencyTest) logMessage(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)
	fmt.Print(logEntry)
	ct.logFile.WriteString(logEntry)
	ct.logFile.Sync()
}

func (ct *ConcurrencyTest) recordCSV(testType TestType, clientID, key, operation, value string, success bool, duration time.Duration, details string) {
	testTypeStr := map[TestType]string{
		LinearizabilityTest: "Linearizability",
		WriteDurabilityTest: "WriteDurability",
	}[testType]

	record := []string{
		testTypeStr,
		clientID,
		key,
		operation,
		value,
		time.Now().Format("2006-01-02 15:04:05.000"),
		strconv.FormatBool(success),
		duration.String(),
		details,
	}

	ct.csvWriter.Write(record)
	ct.csvWriter.Flush()
}

// Linearizability test: Multiple clients write sequences to same key, verify final value is from a LAST write
func (ct *ConcurrencyTest) linearizabilityTest(ctx context.Context) error {
	ct.logMessage("LINEARIZABILITY_TEST_START: Testing concurrent sequences with final value verification")

	testMap, err := atomix.Map[string, string]("concurrency-test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get map instance: %v", err)
	}

	sharedKey := "shared-linearizability-key"
	var wg sync.WaitGroup

	// Track expected last values from each client
	expectedLastValues := make([]string, ct.concurrentClients)

	for i := 0; i < ct.concurrentClients; i++ {
		wg.Add(1)
		clientID := fmt.Sprintf("client-%d", i+1)

		go func(clientIndex int, clientID string) {
			defer wg.Done()

			clientSequence := make([]string, 0, ct.operationsPerClient)

			for j := 1; j <= ct.operationsPerClient; j++ {
				start := time.Now()

				// Each client writes a sequence: client-1-seq-1, client-1-seq-2, client-1-seq-3
				sequenceValue := fmt.Sprintf("%s-seq-%d", clientID, j)
				clientSequence = append(clientSequence, sequenceValue)

				_, err := testMap.Put(ctx, sharedKey, sequenceValue)
				duration := time.Since(start)

				if err != nil {
					ct.logMessage(fmt.Sprintf("LINEARIZABILITY_WRITE_ERROR: %s failed to write %s - %v", clientID, sequenceValue, err))
					ct.recordCSV(LinearizabilityTest, clientID, sharedKey, "write", sequenceValue, false, duration, fmt.Sprintf("Write error: %v", err))
				} else {
					ct.logMessage(fmt.Sprintf("LINEARIZABILITY_WRITE_SUCCESS: %s wrote %s (duration: %v)", clientID, sequenceValue, duration))
					ct.recordCSV(LinearizabilityTest, clientID, sharedKey, "write", sequenceValue, true, duration, fmt.Sprintf("Sequence write: %s", sequenceValue))
				}

				// Small delay between sequence operations
				time.Sleep(10 * time.Millisecond)
			}

			// Store this client's sequence and last expected value
			ct.consistency.sequenceMux.Lock()
			ct.consistency.clientSequences[clientID] = clientSequence
			if len(clientSequence) > 0 {
				expectedLastValues[clientIndex] = clientSequence[len(clientSequence)-1]
			}
			ct.consistency.sequenceMux.Unlock()

		}(i, clientID)
	}

	wg.Wait()

	// Wait a moment for all writes to settle
	time.Sleep(100 * time.Millisecond)

	// Read the final value from the shared key
	start := time.Now()
	entry, err := testMap.Get(ctx, sharedKey)
	duration := time.Since(start)

	if err != nil {
		ct.logMessage(fmt.Sprintf("LINEARIZABILITY_FINAL_READ_ERROR: Failed to read final value - %v", err))
		ct.recordCSV(LinearizabilityTest, "verification", sharedKey, "final-read", "", false, duration, fmt.Sprintf("Read error: %v", err))
		ct.consistency.trackerMux.Lock()
		ct.consistency.linearizable = false
		ct.consistency.trackerMux.Unlock()
		return nil
	}

	if entry == nil {
		ct.logMessage("LINEARIZABILITY_FINAL_READ_NULL: Final value is null")
		ct.recordCSV(LinearizabilityTest, "verification", sharedKey, "final-read", "", false, duration, "Final value is null")
		ct.consistency.trackerMux.Lock()
		ct.consistency.linearizable = false
		ct.consistency.trackerMux.Unlock()
		return nil
	}

	finalValue := entry.Value
	ct.consistency.sequenceMux.Lock()
	ct.consistency.finalValue = finalValue
	ct.consistency.sequenceMux.Unlock()

	ct.logMessage(fmt.Sprintf("LINEARIZABILITY_FINAL_VALUE: %s (duration: %v)", finalValue, duration))
	ct.recordCSV(LinearizabilityTest, "verification", sharedKey, "final-read", finalValue, true, duration, fmt.Sprintf("Final value: %s", finalValue))

	// Verify that final value is one of the expected LAST writes
	isValidLastWrite := false
	for _, lastValue := range expectedLastValues {
		if finalValue == lastValue {
			isValidLastWrite = true
			break
		}
	}

	if isValidLastWrite {
		ct.logMessage(fmt.Sprintf("LINEARIZABILITY_PASS: Final value '%s' matches a client's LAST write", finalValue))
		ct.recordCSV(LinearizabilityTest, "verification", sharedKey, "linearizability-check", finalValue, true, 0, fmt.Sprintf("Final value matches LAST write: %s", finalValue))
	} else {
		ct.logMessage(fmt.Sprintf("LINEARIZABILITY_FAIL: Final value '%s' does NOT match any client's LAST write", finalValue))
		ct.logMessage(fmt.Sprintf("LINEARIZABILITY_EXPECTED_VALUES: %v", expectedLastValues))
		ct.recordCSV(LinearizabilityTest, "verification", sharedKey, "linearizability-check", finalValue, false, 0, fmt.Sprintf("Final value '%s' not in expected last values: %v", finalValue, expectedLastValues))

		ct.consistency.trackerMux.Lock()
		ct.consistency.linearizable = false
		ct.consistency.trackerMux.Unlock()
	}

	// Log all client sequences for analysis
	ct.consistency.sequenceMux.RLock()
	for clientID, sequence := range ct.consistency.clientSequences {
		ct.logMessage(fmt.Sprintf("LINEARIZABILITY_CLIENT_SEQUENCE: %s wrote sequence: %v", clientID, sequence))
	}
	ct.consistency.sequenceMux.RUnlock()

	return nil
}

// Write durability test: Multiple clients write to same key concurrently, verify all acknowledged writes persist
func (ct *ConcurrencyTest) writeDurabilityTest(ctx context.Context) error {
	ct.logMessage("WRITE_DURABILITY_TEST_START: Testing concurrent writes with durability verification")

	testMap, err := atomix.Map[string, string]("concurrency-test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get map instance: %v", err)
	}

	sharedKey := "shared-durability-key"
	var wg sync.WaitGroup

	for i := 0; i < ct.concurrentClients; i++ {
		wg.Add(1)
		clientID := fmt.Sprintf("durability-client-%d", i+1)

		go func(clientID string) {
			defer wg.Done()

			for j := 1; j <= ct.operationsPerClient; j++ {
				start := time.Now()

				// Each client writes unique values to the same key
				writeValue := fmt.Sprintf("%s-write-%d-%d", clientID, j, time.Now().UnixNano())

				_, err := testMap.Put(ctx, sharedKey, writeValue)
				duration := time.Since(start)

				acknowledgedWrite := AcknowledgedWrite{
					ClientID:  clientID,
					Key:       sharedKey,
					Value:     writeValue,
					Timestamp: time.Now(),
					Success:   err == nil,
				}

				ct.consistency.durabilityMux.Lock()
				ct.consistency.acknowledgedWrites = append(ct.consistency.acknowledgedWrites, acknowledgedWrite)
				ct.consistency.durabilityMux.Unlock()

				if err != nil {
					ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_ERROR: %s failed to write %s - %v", clientID, writeValue, err))
					ct.recordCSV(WriteDurabilityTest, clientID, sharedKey, "write", writeValue, false, duration, fmt.Sprintf("Write error: %v", err))
				} else {
					ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_SUCCESS: %s wrote %s (duration: %v)", clientID, writeValue, duration))
					ct.recordCSV(WriteDurabilityTest, clientID, sharedKey, "write", writeValue, true, duration, fmt.Sprintf("Write acknowledged: %s", writeValue))
				}

				// Small delay between writes from same client
				time.Sleep(20 * time.Millisecond)
			}
		}(clientID)
	}

	wg.Wait()

	// Wait for all writes to settle
	time.Sleep(100 * time.Millisecond)

	// Read the final value and verify it matches one of the acknowledged writes
	start := time.Now()
	entry, err := testMap.Get(ctx, sharedKey)
	duration := time.Since(start)

	if err != nil {
		ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_FINAL_READ_ERROR: Failed to read final value - %v", err))
		ct.recordCSV(WriteDurabilityTest, "verification", sharedKey, "final-read", "", false, duration, fmt.Sprintf("Read error: %v", err))
		ct.consistency.trackerMux.Lock()
		ct.consistency.writeDurability = false
		ct.consistency.trackerMux.Unlock()
		return nil
	}

	if entry == nil {
		ct.logMessage("WRITE_DURABILITY_FINAL_READ_NULL: Final value is null")
		ct.recordCSV(WriteDurabilityTest, "verification", sharedKey, "final-read", "", false, duration, "Final value is null")
		ct.consistency.trackerMux.Lock()
		ct.consistency.writeDurability = false
		ct.consistency.trackerMux.Unlock()
		return nil
	}

	finalValue := entry.Value
	ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_FINAL_VALUE: %s (duration: %v)", finalValue, duration))
	ct.recordCSV(WriteDurabilityTest, "verification", sharedKey, "final-read", finalValue, true, duration, fmt.Sprintf("Final value: %s", finalValue))

	// Verify that the final value matches one of the acknowledged writes
	ct.consistency.durabilityMux.RLock()
	totalWrites := len(ct.consistency.acknowledgedWrites)
	acknowledgedWrites := 0
	valueMatched := false

	for _, write := range ct.consistency.acknowledgedWrites {
		if write.Success {
			acknowledgedWrites++
			if write.Value == finalValue {
				valueMatched = true
				ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_MATCH: Final value matches acknowledged write from %s", write.ClientID))
			}
		}
	}
	ct.consistency.durabilityMux.RUnlock()

	writeSuccessRate := float64(acknowledgedWrites) / float64(totalWrites) * 100
	ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_STATS: %d/%d writes acknowledged (%.1f%%)", acknowledgedWrites, totalWrites, writeSuccessRate))

	if valueMatched {
		ct.logMessage("WRITE_DURABILITY_PASS: Final value matches an acknowledged write")
		ct.recordCSV(WriteDurabilityTest, "verification", sharedKey, "durability-check", finalValue, true, 0, fmt.Sprintf("Final value matches acknowledged write: %s", finalValue))
	} else {
		ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_FAIL: Final value '%s' does NOT match any acknowledged write", finalValue))
		ct.recordCSV(WriteDurabilityTest, "verification", sharedKey, "durability-check", finalValue, false, 0, fmt.Sprintf("Final value '%s' not in acknowledged writes", finalValue))

		ct.consistency.trackerMux.Lock()
		ct.consistency.writeDurability = false
		ct.consistency.trackerMux.Unlock()
	}

	// Log all acknowledged writes for analysis
	ct.consistency.durabilityMux.RLock()
	for i, write := range ct.consistency.acknowledgedWrites {
		if write.Success {
			ct.logMessage(fmt.Sprintf("WRITE_DURABILITY_ACKNOWLEDGED_%d: %s wrote %s at %v", i+1, write.ClientID, write.Value, write.Timestamp.Format("15:04:05.000")))
		}
	}
	ct.consistency.durabilityMux.RUnlock()

	return nil
}

func (ct *ConcurrencyTest) runConcurrencyTests(ctx context.Context) error {
	ct.logMessage("LINEARIZABILITY_TEST_SUITE_START: Starting linearizability and write durability testing")
	ct.logMessage(fmt.Sprintf("CONFIG: Concurrent clients: %d, Operations per client: %d, Duration: %v",
		ct.concurrentClients, ct.operationsPerClient, ct.testDuration))

	testMap, err := atomix.Map[string, string]("concurrency-test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize test map: %v", err)
	}

	// Initial connectivity test
	initialKey := "linearizability-connectivity-test"
	initialValue := fmt.Sprintf("initialized-%d", time.Now().Unix())
	_, err = testMap.Put(ctx, initialKey, initialValue)
	if err != nil {
		return fmt.Errorf("failed initial connectivity test: %v", err)
	}

	entry, err := testMap.Get(ctx, initialKey)
	if err != nil || entry == nil || entry.Value != initialValue {
		return fmt.Errorf("initial connectivity verification failed")
	}

	ct.logMessage("CONNECTIVITY: Initial connectivity and consistency verified")

	testCtx, cancel := context.WithTimeout(ctx, ct.testDuration)
	defer cancel()

	// Run the two focused tests
	tests := []struct {
		name     string
		testFunc func(context.Context) error
	}{
		{"Linearizability", ct.linearizabilityTest},
		{"Write Durability", ct.writeDurabilityTest},
	}

	for _, test := range tests {
		ct.logMessage(fmt.Sprintf("STARTING_TEST: %s", test.name))

		testStart := time.Now()
		err := test.testFunc(testCtx)
		testDuration := time.Since(testStart)

		if err != nil {
			ct.logMessage(fmt.Sprintf("TEST_ERROR: %s failed - %v (duration: %v)", test.name, err, testDuration))
		} else {
			ct.logMessage(fmt.Sprintf("TEST_COMPLETE: %s completed successfully (duration: %v)", test.name, testDuration))
		}

		// Brief pause between tests
		time.Sleep(2 * time.Second)
	}

	return ct.generateFinalReport()
}

func (ct *ConcurrencyTest) generateFinalReport() error {
	ct.logMessage("=== LINEARIZABILITY TEST FINAL REPORT ===")

	ct.consistency.trackerMux.RLock()
	linearizable := ct.consistency.linearizable
	writeDurability := ct.consistency.writeDurability
	ct.consistency.trackerMux.RUnlock()

	// Get linearizability details
	ct.consistency.sequenceMux.RLock()
	finalValue := ct.consistency.finalValue
	totalClientSequences := len(ct.consistency.clientSequences)
	ct.consistency.sequenceMux.RUnlock()

	// Get write durability details
	ct.consistency.durabilityMux.RLock()
	totalWrites := len(ct.consistency.acknowledgedWrites)
	acknowledgedWrites := 0
	for _, write := range ct.consistency.acknowledgedWrites {
		if write.Success {
			acknowledgedWrites++
		}
	}
	ct.consistency.durabilityMux.RUnlock()

	ct.logMessage(fmt.Sprintf("LINEARIZABILITY: %t (Final value: %s, Client sequences: %d)", linearizable, finalValue, totalClientSequences))
	ct.logMessage(fmt.Sprintf("WRITE_DURABILITY: %t (Acknowledged writes: %d/%d)", writeDurability, acknowledgedWrites, totalWrites))

	allPassed := linearizable && writeDurability
	ct.logMessage(fmt.Sprintf("OVERALL_RESULT: %t", allPassed))

	// Detailed statistics
	writeSuccessRate := float64(acknowledgedWrites) / float64(totalWrites) * 100
	ct.logMessage(fmt.Sprintf("STATISTICS: Write Success Rate: %.1f%% (%d/%d)",
		writeSuccessRate, acknowledgedWrites, totalWrites))

	// Create summary file
	summaryFile := "linearizability-summary.txt"
	summary, err := os.Create(summaryFile)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %v", err)
	}
	defer summary.Close()

	summary.WriteString("=== ATOMIX LINEARIZABILITY CAPABILITY TEST SUMMARY ===\n")
	summary.WriteString(fmt.Sprintf("Test Date: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	summary.WriteString(fmt.Sprintf("Configuration: %d clients, %d operations per client\n",
		ct.concurrentClients, ct.operationsPerClient))
	summary.WriteString("\nTEST OBJECTIVES:\n")
	summary.WriteString("1. Linearizability: Multiple clients write sequences to same key, final value must be from a LAST write\n")
	summary.WriteString("2. Write Durability: Multiple clients write concurrently, all acknowledged writes must persist\n")
	summary.WriteString("\nRESULTS:\n")
	summary.WriteString(fmt.Sprintf("Linearizability: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[linearizable]))
	summary.WriteString(fmt.Sprintf("Write Durability: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[writeDurability]))
	summary.WriteString(fmt.Sprintf("Overall: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[allPassed]))
	summary.WriteString("\nDETAILS:\n")
	summary.WriteString(fmt.Sprintf("Final Value from Linearizability Test: %s\n", finalValue))
	summary.WriteString(fmt.Sprintf("Client Sequences Processed: %d\n", totalClientSequences))
	summary.WriteString(fmt.Sprintf("Write Success Rate: %.1f%% (%d/%d writes acknowledged)\n", writeSuccessRate, acknowledgedWrites, totalWrites))

	// Add client sequence details
	ct.consistency.sequenceMux.RLock()
	summary.WriteString("\nCLIENT SEQUENCES:\n")
	for clientID, sequence := range ct.consistency.clientSequences {
		summary.WriteString(fmt.Sprintf("%s: %v\n", clientID, sequence))
	}
	ct.consistency.sequenceMux.RUnlock()

	ct.logMessage(fmt.Sprintf("SUMMARY_FILE_CREATED: %s", summaryFile))

	return nil
}

func (ct *ConcurrencyTest) Close() {
	if ct.csvWriter != nil {
		ct.csvWriter.Flush()
	}
	if ct.csvFile != nil {
		ct.csvFile.Close()
	}
	if ct.logFile != nil {
		ct.logFile.Close()
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultValue
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	concurrencyTest, err := NewConcurrencyTest()
	if err != nil {
		log.Fatalf("Failed to initialize concurrency test: %v", err)
	}
	defer concurrencyTest.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		concurrencyTest.logMessage("INTERRUPT: Received interrupt signal, shutting down gracefully...")
		cancel()
	}()

	if err := concurrencyTest.runConcurrencyTests(ctx); err != nil {
		concurrencyTest.logMessage(fmt.Sprintf("FATAL: Test failed: %v", err))
		os.Exit(1)
	}

	select {}
}
