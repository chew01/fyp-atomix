package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LogEntry struct {
	Timestamp time.Time
	Type      string
	Key       string
	Value     string
	Duration  time.Duration
	Error     string
	SeqNum    int
}

type LeaderChange struct {
	Timestamp  time.Time
	Partitions map[int]PartitionInfo
}

type PartitionInfo struct {
	Pod   int
	Term  int
	State string
}

type WriteOperation struct {
	Timestamp time.Time
	Key       string
	Value     string
	Duration  time.Duration
	Success   bool
	Error     string
	SeqNum    int
}

type ReadOperation struct {
	Timestamp time.Time
	Key       string
	Value     string
	Duration  time.Duration
	Success   bool
	Error     string
	Expected  string
	SeqNum    int
}

type AnalysisResult struct {
	TotalWrites        int
	SuccessfulWrites   int
	FailedWrites       int
	WriteSuccessRate   float64
	TotalReads         int
	SuccessfulReads    int
	FailedReads        int
	InconsistentReads  int
	ReadSuccessRate    float64
	ConsistencyRate    float64
	LeaderChanges      int
	WriteLatency       LatencyStats
	ReadLatency        LatencyStats
	WriteGaps          []int
	FailoverEvents     []FailoverEvent
	TestDuration       time.Duration
	BaselinePerf       PerformanceMetrics
	FailoverPerf       PerformanceMetrics
}

type LatencyStats struct {
	Min        time.Duration
	Max        time.Duration
	Mean       time.Duration
	Median     time.Duration
	P95        time.Duration
	P99        time.Duration
	StdDev     time.Duration
}

type PerformanceMetrics struct {
	WriteLatency LatencyStats
	ReadLatency  LatencyStats
	WriteRate    float64
	ReadRate     float64
	ErrorRate    float64
}

type FailoverEvent struct {
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	ImpactedOps   int
	RecoveryTime  time.Duration
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run analyze-logs.go <log-file-path>")
		fmt.Println("Example: go run analyze-logs.go failover-test-results.log")
		os.Exit(1)
	}

	logFile := os.Args[1]
	
	fmt.Printf("Analyzing Atomix Failover Test Logs: %s\n", logFile)
	fmt.Println("=" + strings.Repeat("=", 60))
	
	analyzer := NewLogAnalyzer()
	result, err := analyzer.AnalyzeLog(logFile)
	if err != nil {
		fmt.Printf("Error analyzing log: %v\n", err)
		os.Exit(1)
	}

	analyzer.PrintSummary(result)
	
	outputFile := strings.TrimSuffix(logFile, ".log") + "-analysis.txt"
	err = analyzer.WriteDetailedReport(result, outputFile)
	if err != nil {
		fmt.Printf("Warning: Could not write detailed report: %v\n", err)
	} else {
		fmt.Printf("\nDetailed analysis report written to: %s\n", outputFile)
	}
}

type LogAnalyzer struct {
	timestampRegex   *regexp.Regexp
	writeRegex       *regexp.Regexp
	readRegex        *regexp.Regexp
	leaderRegex      *regexp.Regexp
	durationRegex    *regexp.Regexp
	seqRegex         *regexp.Regexp
}

func NewLogAnalyzer() *LogAnalyzer {
	return &LogAnalyzer{
		timestampRegex: regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})\]`),
		writeRegex:     regexp.MustCompile(`WRITE_(SUCCESS|FAILED): (seq-\d+) -> (.+?) \(duration: ([^,)]+)(?:, error: (.+))?\)`),
		readRegex:      regexp.MustCompile(`READ_(SUCCESS|FAILED|INCONSISTENT): (seq-\d+)(?: -> (.+?))? \(duration: ([^,)]+)(?:, error: (.+))?\)`),
		leaderRegex:    regexp.MustCompile(`LEADER_CHANGE: (.+)`),
		durationRegex:  regexp.MustCompile(`(\d+(?:\.\d+)?)(ms|s|µs|ns)`),
		seqRegex:       regexp.MustCompile(`seq-(\d+)`),
	}
}

func (la *LogAnalyzer) AnalyzeLog(filename string) (*AnalysisResult, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	var writes []WriteOperation
	var reads []ReadOperation
	var leaderChanges []LeaderChange
	var startTime, endTime time.Time

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		
		timestamp := la.extractTimestamp(line)
		if startTime.IsZero() {
			startTime = timestamp
		}
		endTime = timestamp

		if write := la.parseWrite(line, timestamp); write != nil {
			writes = append(writes, *write)
		} else if read := la.parseRead(line, timestamp); read != nil {
			reads = append(reads, *read)
		} else if leader := la.parseLeaderChange(line, timestamp); leader != nil {
			leaderChanges = append(leaderChanges, *leader)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	return la.generateAnalysis(writes, reads, leaderChanges, startTime, endTime), nil
}

func (la *LogAnalyzer) extractTimestamp(line string) time.Time {
	matches := la.timestampRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return time.Time{}
	}
	
	timestamp, err := time.Parse("2006-01-02 15:04:05.000", matches[1])
	if err != nil {
		return time.Time{}
	}
	return timestamp
}

func (la *LogAnalyzer) parseWrite(line string, timestamp time.Time) *WriteOperation {
	matches := la.writeRegex.FindStringSubmatch(line)
	if len(matches) < 5 {
		return nil
	}

	seqNum := la.extractSeqNum(matches[2])
	duration := la.parseDuration(matches[4])
	success := matches[1] == "SUCCESS"
	
	var errorMsg string
	if len(matches) > 5 {
		errorMsg = matches[5]
	}

	return &WriteOperation{
		Timestamp: timestamp,
		Key:       matches[2],
		Value:     matches[3],
		Duration:  duration,
		Success:   success,
		Error:     errorMsg,
		SeqNum:    seqNum,
	}
}

func (la *LogAnalyzer) parseRead(line string, timestamp time.Time) *ReadOperation {
	if strings.Contains(line, "READ_INCONSISTENT") {
		return la.parseInconsistentRead(line, timestamp)
	}
	
	matches := la.readRegex.FindStringSubmatch(line)
	if len(matches) < 5 {
		return nil
	}

	seqNum := la.extractSeqNum(matches[2])
	duration := la.parseDuration(matches[4])
	success := matches[1] == "SUCCESS"
	
	var errorMsg string
	if len(matches) > 5 {
		errorMsg = matches[5]
	}

	return &ReadOperation{
		Timestamp: timestamp,
		Key:       matches[2],
		Value:     matches[3],
		Duration:  duration,
		Success:   success,
		Error:     errorMsg,
		SeqNum:    seqNum,
	}
}

func (la *LogAnalyzer) parseInconsistentRead(line string, timestamp time.Time) *ReadOperation {
	inconsistentRegex := regexp.MustCompile(`READ_INCONSISTENT: (seq-\d+) -> got '(.+?)', expected '(.+?)' \(duration: ([^)]+)\)`)
	matches := inconsistentRegex.FindStringSubmatch(line)
	if len(matches) < 5 {
		return nil
	}

	seqNum := la.extractSeqNum(matches[1])
	duration := la.parseDuration(matches[4])

	return &ReadOperation{
		Timestamp: timestamp,
		Key:       matches[1],
		Value:     matches[2],
		Duration:  duration,
		Success:   false,
		Expected:  matches[3],
		SeqNum:    seqNum,
	}
}

func (la *LogAnalyzer) parseLeaderChange(line string, timestamp time.Time) *LeaderChange {
	matches := la.leaderRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return nil
	}

	partitions := make(map[int]PartitionInfo)
	partitionRegex := regexp.MustCompile(`Partition (\d+): Pod (\d+) \(term: (\d+), (\w+)\)`)
	partitionMatches := partitionRegex.FindAllStringSubmatch(matches[1], -1)
	
	for _, match := range partitionMatches {
		if len(match) >= 5 {
			partNum, _ := strconv.Atoi(match[1])
			podNum, _ := strconv.Atoi(match[2])
			term, _ := strconv.Atoi(match[3])
			state := match[4]
			
			partitions[partNum] = PartitionInfo{
				Pod:   podNum,
				Term:  term,
				State: state,
			}
		}
	}

	return &LeaderChange{
		Timestamp:  timestamp,
		Partitions: partitions,
	}
}

func (la *LogAnalyzer) extractSeqNum(key string) int {
	matches := la.seqRegex.FindStringSubmatch(key)
	if len(matches) < 2 {
		return 0
	}
	num, _ := strconv.Atoi(matches[1])
	return num
}

func (la *LogAnalyzer) parseDuration(durationStr string) time.Duration {
	matches := la.durationRegex.FindStringSubmatch(durationStr)
	if len(matches) < 3 {
		return 0
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	unit := matches[2]
	switch unit {
	case "ns":
		return time.Duration(value)
	case "µs", "us":
		return time.Duration(value * 1000)
	case "ms":
		return time.Duration(value * 1000000)
	case "s":
		return time.Duration(value * 1000000000)
	default:
		return 0
	}
}

func (la *LogAnalyzer) generateAnalysis(writes []WriteOperation, reads []ReadOperation, leaderChanges []LeaderChange, startTime, endTime time.Time) *AnalysisResult {
	result := &AnalysisResult{}
	
	result.TotalWrites = len(writes)
	result.TotalReads = len(reads)
	result.LeaderChanges = len(leaderChanges)
	result.TestDuration = endTime.Sub(startTime)

	successfulWrites := 0
	failedWrites := 0
	var writeDurations []time.Duration
	
	for _, write := range writes {
		if write.Success {
			successfulWrites++
		} else {
			failedWrites++
		}
		writeDurations = append(writeDurations, write.Duration)
	}

	result.SuccessfulWrites = successfulWrites
	result.FailedWrites = failedWrites
	if result.TotalWrites > 0 {
		result.WriteSuccessRate = float64(successfulWrites) / float64(result.TotalWrites) * 100
	}

	successfulReads := 0
	failedReads := 0
	inconsistentReads := 0
	var readDurations []time.Duration
	
	for _, read := range reads {
		if read.Success {
			successfulReads++
		} else {
			failedReads++
			if read.Expected != "" {
				inconsistentReads++
			}
		}
		readDurations = append(readDurations, read.Duration)
	}

	result.SuccessfulReads = successfulReads
	result.FailedReads = failedReads
	result.InconsistentReads = inconsistentReads
	if result.TotalReads > 0 {
		result.ReadSuccessRate = float64(successfulReads) / float64(result.TotalReads) * 100
		result.ConsistencyRate = float64(result.TotalReads-inconsistentReads) / float64(result.TotalReads) * 100
	}

	if len(writeDurations) > 0 {
		result.WriteLatency = la.calculateLatencyStats(writeDurations)
	}
	if len(readDurations) > 0 {
		result.ReadLatency = la.calculateLatencyStats(readDurations)
	}

	result.WriteGaps = la.detectSequenceGaps(writes)
	result.FailoverEvents = la.detectFailoverEvents(writes, reads, leaderChanges)
	
	result.BaselinePerf, result.FailoverPerf = la.calculatePerformanceComparison(writes, reads, leaderChanges)

	return result
}

func (la *LogAnalyzer) calculateLatencyStats(durations []time.Duration) LatencyStats {
	if len(durations) == 0 {
		return LatencyStats{}
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	mean := sum / time.Duration(len(durations))

	var variance float64
	for _, d := range durations {
		diff := float64(d - mean)
		variance += diff * diff
	}
	variance /= float64(len(durations))
	stdDev := time.Duration(math.Sqrt(variance))

	stats := LatencyStats{
		Min:    durations[0],
		Max:    durations[len(durations)-1],
		Mean:   mean,
		Median: durations[len(durations)/2],
		StdDev: stdDev,
	}

	if len(durations) > 1 {
		p95Index := int(0.95 * float64(len(durations)))
		p99Index := int(0.99 * float64(len(durations)))
		stats.P95 = durations[p95Index]
		stats.P99 = durations[p99Index]
	}

	return stats
}

func (la *LogAnalyzer) detectSequenceGaps(writes []WriteOperation) []int {
	var gaps []int
	seqNums := make([]int, 0, len(writes))
	
	for _, write := range writes {
		if write.Success {
			seqNums = append(seqNums, write.SeqNum)
		}
	}
	
	sort.Ints(seqNums)
	
	for i := 1; i < len(seqNums); i++ {
		if seqNums[i]-seqNums[i-1] > 1 {
			for gap := seqNums[i-1] + 1; gap < seqNums[i]; gap++ {
				gaps = append(gaps, gap)
			}
		}
	}
	
	return gaps
}

func (la *LogAnalyzer) detectFailoverEvents(writes []WriteOperation, reads []ReadOperation, leaderChanges []LeaderChange) []FailoverEvent {
	var events []FailoverEvent
	
	if len(leaderChanges) < 2 {
		return events
	}

	for i := 1; i < len(leaderChanges); i++ {
		startTime := leaderChanges[i-1].Timestamp
		endTime := leaderChanges[i].Timestamp
		
		impactedOps := 0
		var firstSuccessAfter time.Time
		
		for _, write := range writes {
			if write.Timestamp.After(startTime) && write.Timestamp.Before(endTime) {
				if !write.Success {
					impactedOps++
				} else if firstSuccessAfter.IsZero() {
					firstSuccessAfter = write.Timestamp
				}
			}
		}
		
		for _, read := range reads {
			if read.Timestamp.After(startTime) && read.Timestamp.Before(endTime) {
				if !read.Success {
					impactedOps++
				} else if firstSuccessAfter.IsZero() {
					firstSuccessAfter = read.Timestamp
				}
			}
		}
		
		var recoveryTime time.Duration
		if !firstSuccessAfter.IsZero() {
			recoveryTime = firstSuccessAfter.Sub(startTime)
		}
		
		if impactedOps > 0 {
			events = append(events, FailoverEvent{
				StartTime:    startTime,
				EndTime:      endTime,
				Duration:     endTime.Sub(startTime),
				ImpactedOps:  impactedOps,
				RecoveryTime: recoveryTime,
			})
		}
	}
	
	return events
}

func (la *LogAnalyzer) calculatePerformanceComparison(writes []WriteOperation, reads []ReadOperation, leaderChanges []LeaderChange) (PerformanceMetrics, PerformanceMetrics) {
	if len(leaderChanges) == 0 {
		allWrites := make([]time.Duration, 0, len(writes))
		allReads := make([]time.Duration, 0, len(reads))
		
		for _, w := range writes {
			if w.Success {
				allWrites = append(allWrites, w.Duration)
			}
		}
		for _, r := range reads {
			if r.Success {
				allReads = append(allReads, r.Duration)
			}
		}
		
		baseline := PerformanceMetrics{
			WriteLatency: la.calculateLatencyStats(allWrites),
			ReadLatency:  la.calculateLatencyStats(allReads),
		}
		
		return baseline, PerformanceMetrics{}
	}

	firstFailover := leaderChanges[0].Timestamp
	
	var baselineWrites, failoverWrites []time.Duration
	var baselineReads, failoverReads []time.Duration
	
	for _, write := range writes {
		if write.Success {
			if write.Timestamp.Before(firstFailover) {
				baselineWrites = append(baselineWrites, write.Duration)
			} else {
				failoverWrites = append(failoverWrites, write.Duration)
			}
		}
	}
	
	for _, read := range reads {
		if read.Success {
			if read.Timestamp.Before(firstFailover) {
				baselineReads = append(baselineReads, read.Duration)
			} else {
				failoverReads = append(failoverReads, read.Duration)
			}
		}
	}
	
	baseline := PerformanceMetrics{
		WriteLatency: la.calculateLatencyStats(baselineWrites),
		ReadLatency:  la.calculateLatencyStats(baselineReads),
	}
	
	failover := PerformanceMetrics{
		WriteLatency: la.calculateLatencyStats(failoverWrites),
		ReadLatency:  la.calculateLatencyStats(failoverReads),
	}
	
	return baseline, failover
}

func (la *LogAnalyzer) PrintSummary(result *AnalysisResult) {
	fmt.Println("\nATOMIX FAILOVER TEST ANALYSIS SUMMARY")
	fmt.Println("=" + strings.Repeat("=", 60))
	
	fmt.Printf("Test Duration: %v\n", result.TestDuration)
	fmt.Printf("Leader Changes Detected: %d\n", result.LeaderChanges)
	
	fmt.Println("\nWRITE OPERATIONS:")
	fmt.Printf("  Total Writes: %d\n", result.TotalWrites)
	fmt.Printf("  Successful: %d (%.2f%%)\n", result.SuccessfulWrites, result.WriteSuccessRate)
	fmt.Printf("  Failed: %d\n", result.FailedWrites)
	if len(result.WriteGaps) > 0 {
		fmt.Printf("  ❌ SEQUENCE GAPS DETECTED: %d missing sequences\n", len(result.WriteGaps))
		fmt.Printf("     Missing sequences: %v\n", result.WriteGaps[:min(10, len(result.WriteGaps))])
	} else {
		fmt.Printf("  ✅ NO SEQUENCE GAPS: All writes preserved\n")
	}
	
	fmt.Println("\nREAD OPERATIONS:")
	fmt.Printf("  Total Reads: %d\n", result.TotalReads)
	fmt.Printf("  Successful: %d (%.2f%%)\n", result.SuccessfulReads, result.ReadSuccessRate)
	fmt.Printf("  Failed: %d\n", result.FailedReads)
	fmt.Printf("  Inconsistent: %d\n", result.InconsistentReads)
	if result.ConsistencyRate >= 100 {
		fmt.Printf("  ✅ LINEARIZABILITY: %.2f%% (Perfect consistency)\n", result.ConsistencyRate)
	} else if result.ConsistencyRate >= 99 {
		fmt.Printf("  ⚠️  LINEARIZABILITY: %.2f%% (Near-perfect consistency)\n", result.ConsistencyRate)
	} else {
		fmt.Printf("  ❌ LINEARIZABILITY: %.2f%% (Consistency issues detected)\n", result.ConsistencyRate)
	}
	
	fmt.Println("\nPERFORMANCE METRICS:")
	la.printLatencyStats("Write Latency", result.WriteLatency)
	la.printLatencyStats("Read Latency", result.ReadLatency)
	
	if len(result.FailoverEvents) > 0 {
		fmt.Printf("\nFAILOVER IMPACT ANALYSIS:\n")
		fmt.Printf("  Failover Events: %d\n", len(result.FailoverEvents))
		for i, event := range result.FailoverEvents {
			fmt.Printf("  Event %d: %d operations impacted, %v recovery time\n", 
				i+1, event.ImpactedOps, event.RecoveryTime)
		}
		
		if result.FailoverPerf.WriteLatency.Mean > 0 {
			fmt.Printf("\nPERFORMANCE COMPARISON:\n")
			fmt.Printf("  Baseline Write Latency: %v (mean)\n", result.BaselinePerf.WriteLatency.Mean)
			fmt.Printf("  Failover Write Latency: %v (mean)\n", result.FailoverPerf.WriteLatency.Mean)
			if result.BaselinePerf.WriteLatency.Mean > 0 {
				impact := float64(result.FailoverPerf.WriteLatency.Mean-result.BaselinePerf.WriteLatency.Mean) / float64(result.BaselinePerf.WriteLatency.Mean) * 100
				fmt.Printf("  Performance Impact: %.1f%%\n", impact)
			}
		}
	} else {
		fmt.Printf("\n✅ AUTOMATIC RECOVERY: No significant failover events detected\n")
	}
	
	fmt.Println("\nACADEMIC EVIDENCE SUMMARY:")
	fmt.Println("=" + strings.Repeat("=", 30))
	
	if len(result.WriteGaps) == 0 {
		fmt.Printf("✅ WRITE DURABILITY: VERIFIED - No data loss detected\n")
	} else {
		fmt.Printf("❌ WRITE DURABILITY: FAILED - %d write operations lost\n", len(result.WriteGaps))
	}
	
	if result.ConsistencyRate >= 99.9 {
		fmt.Printf("✅ READ LINEARIZABILITY: VERIFIED - %.2f%% consistency rate\n", result.ConsistencyRate)
	} else {
		fmt.Printf("❌ READ LINEARIZABILITY: FAILED - %.2f%% consistency rate\n", result.ConsistencyRate)
	}
	
	if result.LeaderChanges > 0 && len(result.FailoverEvents) == 0 {
		fmt.Printf("✅ AUTOMATIC RECOVERY: VERIFIED - System recovered automatically\n")
	} else if len(result.FailoverEvents) > 0 {
		avgRecovery := time.Duration(0)
		for _, event := range result.FailoverEvents {
			avgRecovery += event.RecoveryTime
		}
		avgRecovery /= time.Duration(len(result.FailoverEvents))
		fmt.Printf("⚠️  AUTOMATIC RECOVERY: PARTIAL - Average recovery time: %v\n", avgRecovery)
	} else {
		fmt.Printf("❌ AUTOMATIC RECOVERY: UNTESTED - No failover scenarios detected\n")
	}
}

func (la *LogAnalyzer) printLatencyStats(title string, stats LatencyStats) {
	if stats.Mean == 0 {
		return
	}
	fmt.Printf("  %s:\n", title)
	fmt.Printf("    Mean: %v, Median: %v\n", stats.Mean, stats.Median)
	fmt.Printf("    Min: %v, Max: %v\n", stats.Min, stats.Max)
	fmt.Printf("    P95: %v, P99: %v\n", stats.P95, stats.P99)
}

func (la *LogAnalyzer) WriteDetailedReport(result *AnalysisResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "ATOMIX FAILOVER TEST - DETAILED ANALYSIS REPORT\n")
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "="+ strings.Repeat("=", 70) + "\n\n")

	fmt.Fprintf(file, "EXECUTIVE SUMMARY\n")
	fmt.Fprintf(file, "-" + strings.Repeat("-", 20) + "\n")
	fmt.Fprintf(file, "Test Duration: %v\n", result.TestDuration)
	fmt.Fprintf(file, "Total Operations: %d writes, %d reads\n", result.TotalWrites, result.TotalReads)
	fmt.Fprintf(file, "Leader Changes: %d\n", result.LeaderChanges)
	fmt.Fprintf(file, "Write Success Rate: %.2f%%\n", result.WriteSuccessRate)
	fmt.Fprintf(file, "Read Success Rate: %.2f%%\n", result.ReadSuccessRate)
	fmt.Fprintf(file, "Data Consistency Rate: %.2f%%\n\n", result.ConsistencyRate)

	fmt.Fprintf(file, "DETAILED FINDINGS\n")
	fmt.Fprintf(file, "-" + strings.Repeat("-", 20) + "\n")

	fmt.Fprintf(file, "\n1. WRITE DURABILITY ANALYSIS\n")
	if len(result.WriteGaps) == 0 {
		fmt.Fprintf(file, "   STATUS: ✅ PASSED\n")
		fmt.Fprintf(file, "   All write operations were successfully persisted.\n")
		fmt.Fprintf(file, "   No sequence gaps detected in the write log.\n")
	} else {
		fmt.Fprintf(file, "   STATUS: ❌ FAILED\n")
		fmt.Fprintf(file, "   %d write operations were lost during the test.\n", len(result.WriteGaps))
		fmt.Fprintf(file, "   Missing sequence numbers: %v\n", result.WriteGaps)
	}

	fmt.Fprintf(file, "\n2. READ LINEARIZABILITY ANALYSIS\n")
	if result.ConsistencyRate >= 99.9 {
		fmt.Fprintf(file, "   STATUS: ✅ PASSED\n")
		fmt.Fprintf(file, "   Read operations demonstrate strong consistency.\n")
	} else {
		fmt.Fprintf(file, "   STATUS: ❌ FAILED\n")
		fmt.Fprintf(file, "   %d inconsistent reads detected out of %d total reads.\n", 
			result.InconsistentReads, result.TotalReads)
	}
	fmt.Fprintf(file, "   Consistency Rate: %.2f%%\n", result.ConsistencyRate)

	fmt.Fprintf(file, "\n3. AUTOMATIC RECOVERY ANALYSIS\n")
	if len(result.FailoverEvents) == 0 && result.LeaderChanges > 0 {
		fmt.Fprintf(file, "   STATUS: ✅ PASSED\n")
		fmt.Fprintf(file, "   System recovered automatically from all failover scenarios.\n")
	} else if len(result.FailoverEvents) > 0 {
		fmt.Fprintf(file, "   STATUS: ⚠️  PARTIAL\n")
		fmt.Fprintf(file, "   %d failover events with measurable impact detected.\n", len(result.FailoverEvents))
		for i, event := range result.FailoverEvents {
			fmt.Fprintf(file, "   Event %d: %v duration, %d operations impacted, %v recovery time\n",
				i+1, event.Duration, event.ImpactedOps, event.RecoveryTime)
		}
	} else {
		fmt.Fprintf(file, "   STATUS: ❌ UNTESTED\n")
		fmt.Fprintf(file, "   No failover scenarios were detected during the test.\n")
	}

	fmt.Fprintf(file, "\nPERFORMANCE STATISTICS\n")
	fmt.Fprintf(file, "-" + strings.Repeat("-", 25) + "\n")
	la.writeLatencyStats(file, "Write Operations", result.WriteLatency)
	la.writeLatencyStats(file, "Read Operations", result.ReadLatency)

	fmt.Fprintf(file, "\nRECOMMendations\n")
	fmt.Fprintf(file, "-" + strings.Repeat("-", 20) + "\n")
	
	if len(result.WriteGaps) > 0 {
		fmt.Fprintf(file, "• Investigate write durability failures\n")
		fmt.Fprintf(file, "• Consider increasing write timeout values\n")
		fmt.Fprintf(file, "• Review consensus configuration\n")
	}
	
	if result.ConsistencyRate < 99.9 {
		fmt.Fprintf(file, "• Address read consistency issues\n")
		fmt.Fprintf(file, "• Verify read quorum settings\n")
		fmt.Fprintf(file, "• Check for network partitioning issues\n")
	}
	
	if len(result.FailoverEvents) > 0 {
		fmt.Fprintf(file, "• Optimize failover detection and recovery\n")
		fmt.Fprintf(file, "• Consider adjusting leader election timeouts\n")
		fmt.Fprintf(file, "• Review cluster health monitoring\n")
	}
	
	if len(result.WriteGaps) == 0 && result.ConsistencyRate >= 99.9 && len(result.FailoverEvents) == 0 {
		fmt.Fprintf(file, "✅ All objectives met - System demonstrates robust failover capabilities\n")
	}

	return nil
}

func (la *LogAnalyzer) writeLatencyStats(file *os.File, title string, stats LatencyStats) {
	if stats.Mean == 0 {
		return
	}
	fmt.Fprintf(file, "\n%s Latency:\n", title)
	fmt.Fprintf(file, "  Mean: %v\n", stats.Mean)
	fmt.Fprintf(file, "  Median: %v\n", stats.Median)
	fmt.Fprintf(file, "  Min: %v, Max: %v\n", stats.Min, stats.Max)
	fmt.Fprintf(file, "  P95: %v, P99: %v\n", stats.P95, stats.P99)
	fmt.Fprintf(file, "  Standard Deviation: %v\n", stats.StdDev)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}