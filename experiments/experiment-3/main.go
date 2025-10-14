package main

import (
	"context"
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

type FailoverTest struct {
	writeSeq      int64
	writeLog      map[string]string
	writeLogMux   sync.RWMutex
	leaderStatus  string
	leaderMux     sync.RWMutex
	k8sClient     *kubernetes.Clientset
	dynamicClient dynamic.Interface
	logFile       *os.File
	writeInterval time.Duration
	readInterval  time.Duration
	testDuration  time.Duration
	namespace     string
}

func NewFailoverTest() (*FailoverTest, error) {
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

	writeInterval := getEnvDuration("WRITE_INTERVAL", 1*time.Second)
	readInterval := getEnvDuration("READ_INTERVAL", 2*time.Second)
	testDuration := getEnvDuration("TEST_DURATION", 10*time.Minute)
	logFileName := getEnv("LOG_FILE", "failover-test-results.log")
	namespace := getEnv("NAMESPACE", "default")

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	return &FailoverTest{
		writeSeq:      0,
		writeLog:      make(map[string]string),
		k8sClient:     clientset,
		dynamicClient: dynamicClient,
		logFile:       logFile,
		writeInterval: writeInterval,
		readInterval:  readInterval,
		testDuration:  testDuration,
		namespace:     namespace,
	}, nil
}

func (ft *FailoverTest) logMessage(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)
	fmt.Print(logEntry)
	ft.logFile.WriteString(logEntry)
	ft.logFile.Sync()
}

func (ft *FailoverTest) continuousWriter(ctx context.Context) {
	ticker := time.NewTicker(ft.writeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			testMap, err := atomix.Map[string, string]("test-map").Codec(generic.Scalar[string]()).Get(ctx)
			if err != nil {
				ft.logMessage(fmt.Sprintf("WRITE_ERROR: Failed to get map instance: %v", err))
				continue
			}

			seq := atomic.AddInt64(&ft.writeSeq, 1)
			key := fmt.Sprintf("seq-%06d", seq)
			value := fmt.Sprintf("value-%06d-%d", seq, time.Now().Unix())

			start := time.Now()
			_, err = testMap.Put(ctx, key, value)
			duration := time.Since(start)

			if err != nil {
				ft.logMessage(fmt.Sprintf("WRITE_FAILED: %s -> %s (duration: %v, error: %v)", key, value, duration, err))
			} else {
				ft.writeLogMux.Lock()
				ft.writeLog[key] = value
				ft.writeLogMux.Unlock()
				ft.logMessage(fmt.Sprintf("WRITE_SUCCESS: %s -> %s (duration: %v)", key, value, duration))
			}
		}
	}
}

func (ft *FailoverTest) continuousReader(ctx context.Context) {
	ticker := time.NewTicker(ft.readInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ft.writeLogMux.RLock()
			if len(ft.writeLog) == 0 {
				ft.writeLogMux.RUnlock()
				continue
			}

			keys := make([]string, 0, len(ft.writeLog))
			for key := range ft.writeLog {
				keys = append(keys, key)
			}
			ft.writeLogMux.RUnlock()

			if len(keys) > 0 {
				testMap, err := atomix.Map[string, string]("test-map").Codec(generic.Scalar[string]()).Get(ctx)
				if err != nil {
					ft.logMessage(fmt.Sprintf("READ_ERROR: Failed to get map instance: %v", err))
					continue
				}

				recentKey := keys[len(keys)-1]
				expectedValue := ft.writeLog[recentKey]

				start := time.Now()
				entry, err := testMap.Get(ctx, recentKey)
				duration := time.Since(start)

				if err != nil {
					ft.logMessage(fmt.Sprintf("READ_FAILED: %s (duration: %v, error: %v)", recentKey, duration, err))
				} else if entry == nil {
					ft.logMessage(fmt.Sprintf("READ_FAILED: %s -> key not found (duration: %v)", recentKey, duration))
				} else if entry.Value != expectedValue {
					ft.logMessage(fmt.Sprintf("READ_INCONSISTENT: %s -> got '%s', expected '%s' (duration: %v)", recentKey, entry.Value, expectedValue, duration))
				} else {
					ft.logMessage(fmt.Sprintf("READ_SUCCESS: %s -> %s (duration: %v)", recentKey, entry.Value, duration))
				}
			}
		}
	}
}

func (ft *FailoverTest) leaderMonitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	raftGroupGVR := schema.GroupVersionResource{
		Group:    "consensus.atomix.io",
		Version:  "v1beta1",
		Resource: "raftgroups",
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			raftGroups, err := ft.dynamicClient.Resource(raftGroupGVR).Namespace(ft.namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "atomix.io/store=consensus-store",
			})
			if err != nil {
				ft.logMessage(fmt.Sprintf("LEADER_ERROR: Failed to list RaftGroups: %v", err))
				continue
			}

			var leaderInfo []string

			for _, item := range raftGroups.Items {
				groupName := item.GetName()

				partNum, err := strconv.Atoi(strings.Split(groupName, "-")[2])
				if err != nil {
					ft.logMessage(fmt.Sprintf("LEADER_ERROR: Failed to convert group name to int: %v", err))
				}

				status, found, err := unstructured.NestedMap(item.Object, "status")
				if err != nil || !found {
					ft.logMessage(fmt.Sprintf("LEADER_ERROR: Failed to get status for RaftGroup %s: %v", groupName, err))
					continue
				}

				leader, found, err := unstructured.NestedMap(status, "leader")
				if err != nil || !found {
					leaderInfo = append(leaderInfo, fmt.Sprintf("Partition %d: No Leader", partNum))
					continue
				}

				leaderName, found, err := unstructured.NestedString(leader, "name")
				if err != nil || !found {
					leaderInfo = append(leaderInfo, fmt.Sprintf("%s:unknown-leader", groupName))
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

				podNum, err := strconv.Atoi(strings.Split(leaderName, "-")[3])
				if err != nil {
					ft.logMessage(fmt.Sprintf("LEADER_ERROR: Failed to get pod number: %v", err))
				}
				leaderInfo = append(leaderInfo, fmt.Sprintf("Partition %d: Pod %d (term: %d, %s)", partNum, podNum-1, term, state))
			}

			newStatus := fmt.Sprintf("%s", strings.Join(leaderInfo, ", "))

			ft.leaderMux.Lock()
			if ft.leaderStatus != newStatus {
				ft.logMessage(fmt.Sprintf("LEADER_CHANGE: %s", newStatus))
				ft.leaderStatus = newStatus
			}
			ft.leaderMux.Unlock()
		}
	}
}

func (ft *FailoverTest) runTest(ctx context.Context) error {
	ft.logMessage("STARTING Atomix Failover Capability Test")
	ft.logMessage(fmt.Sprintf("CONFIG: Write interval: %v, Read interval: %v, Test duration: %v", ft.writeInterval, ft.readInterval, ft.testDuration))

	testMap, err := atomix.Map[string, string]("test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize test map: %v", err)
	}

	initialKey := "test-connectivity"
	initialValue := fmt.Sprintf("initialized-%d", time.Now().Unix())
	_, err = testMap.Put(ctx, initialKey, initialValue)
	if err != nil {
		return fmt.Errorf("failed initial connectivity test: %v", err)
	}

	entry, err := testMap.Get(ctx, initialKey)
	if err != nil {
		return fmt.Errorf("failed initial read test: %v", err)
	}

	if entry == nil {
		return fmt.Errorf("initial read test failed: key not found")
	}

	if entry.Value != initialValue {
		return fmt.Errorf("initial read consistency test failed: got '%s', expected '%s'", entry.Value, initialValue)
	}

	ft.logMessage("CONNECTIVITY: Initial connectivity and consistency verified")

	var wg sync.WaitGroup

	testCtx, cancel := context.WithTimeout(ctx, ft.testDuration)
	defer cancel()

	wg.Add(3)
	go func() {
		defer wg.Done()
		ft.continuousWriter(testCtx)
	}()

	go func() {
		defer wg.Done()
		ft.continuousReader(testCtx)
	}()

	go func() {
		defer wg.Done()
		ft.leaderMonitor(testCtx)
	}()

	wg.Wait()

	ft.writeLogMux.RLock()
	totalWrites := len(ft.writeLog)
	ft.writeLogMux.RUnlock()

	ft.logMessage(fmt.Sprintf("COMPLETED: Test finished. Total successful writes: %d, Final sequence: %d", totalWrites, atomic.LoadInt64(&ft.writeSeq)))

	return nil
}

func (ft *FailoverTest) Close() {
	if ft.logFile != nil {
		ft.logFile.Close()
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
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

	failoverTest, err := NewFailoverTest()
	if err != nil {
		log.Fatalf("Failed to initialize failover test: %v", err)
	}
	defer failoverTest.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		failoverTest.logMessage("INTERRUPT: Received interrupt signal, shutting down gracefully...")
		cancel()
	}()

	if err := failoverTest.runTest(ctx); err != nil {
		failoverTest.logMessage(fmt.Sprintf("FATAL: Test failed: %v", err))
		os.Exit(1)
	}
}
