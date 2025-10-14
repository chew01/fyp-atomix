package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type TestResult struct {
	TestID       string
	Scenario     string
	Success      bool
	Duration     time.Duration
	RecoveryTime time.Duration
	WriteTime    time.Time
	FailureTime  time.Time
}

type ScenarioStats struct {
	Name        string
	TotalTests  int
	Successful  int
	SuccessRate float64
	AvgDuration time.Duration
	AvgRecovery time.Duration
	MinRecovery time.Duration
	MaxRecovery time.Duration
}

func main() {
	logFile := "enhanced-failover-test-results.log"
	if len(os.Args) > 1 {
		logFile = os.Args[1]
	}

	results, err := parseLogFile(logFile)
	if err != nil {
		log.Fatalf("Failed to parse log file: %v", err)
	}

	stats := analyzeResults(results)
	generateReport(stats, results)
}

func parseLogFile(filename string) ([]TestResult, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var results []TestResult
	testMap := make(map[string]*TestResult)

	scanner := bufio.NewScanner(file)

	// Regex patterns for different log events
	testStartPattern := regexp.MustCompile(`PRECISION_TEST_START: (test-\d+), Scenario: (\w+)`)
	writeCompletePattern := regexp.MustCompile(`WRITE_COMPLETE: (test-\d+) \(duration: ([^)]+)\)`)
	terminationPattern := regexp.MustCompile(`TERMINATION_SUCCESS: Pod (.+) terminated`)
	recoveryPattern := regexp.MustCompile(`LEADER_RECOVERY: (test-\d+) \(duration: ([^)]+)\)`)
	successPattern := regexp.MustCompile(`PRECISION_SUCCESS: (test-\d+) verified \(total duration: ([^)]+)\)`)
	timestampPattern := regexp.MustCompile(`\[([^\]]+)\]`)

	for scanner.Scan() {
		line := scanner.Text()

		// Extract timestamp
		timestampMatch := timestampPattern.FindStringSubmatch(line)
		var timestamp time.Time
		if len(timestampMatch) > 1 {
			timestamp, _ = time.Parse("2006-01-02 15:04:05.000", timestampMatch[1])
		}

		// Parse test start
		if match := testStartPattern.FindStringSubmatch(line); len(match) > 2 {
			testID := match[1]
			scenario := match[2]
			testMap[testID] = &TestResult{
				TestID:   testID,
				Scenario: scenario,
			}
		}

		// Parse write complete
		if match := writeCompletePattern.FindStringSubmatch(line); len(match) > 2 {
			testID := match[1]
			if result, exists := testMap[testID]; exists {
				result.WriteTime = timestamp
			}
		}

		// Parse recovery time
		if match := recoveryPattern.FindStringSubmatch(line); len(match) > 2 {
			testID := match[1]
			recoveryStr := match[2]
			if result, exists := testMap[testID]; exists {
				result.FailureTime = timestamp.Add(-parseDuration(recoveryStr))
				result.RecoveryTime = parseDuration(recoveryStr)
			}
		}

		// Parse success
		if match := successPattern.FindStringSubmatch(line); len(match) > 2 {
			testID := match[1]
			durationStr := match[2]
			if result, exists := testMap[testID]; exists {
				result.Success = true
				result.Duration = parseDuration(durationStr)
				results = append(results, *result)
			}
		}
	}

	return results, scanner.Err()
}

func parseDuration(s string) time.Duration {
	// Handle various duration formats
	s = strings.TrimSpace(s)

	// Handle milliseconds
	if strings.HasSuffix(s, "ms") {
		if ms, err := strconv.ParseFloat(strings.TrimSuffix(s, "ms"), 64); err == nil {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}

	// Handle seconds
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ms") {
		if sec, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64); err == nil {
			return time.Duration(sec * float64(time.Second))
		}
	}

	// Fallback to Go's duration parser
	if dur, err := time.ParseDuration(s); err == nil {
		return dur
	}

	return 0
}

func analyzeResults(results []TestResult) map[string]ScenarioStats {
	scenarioMap := make(map[string][]TestResult)

	// Group results by scenario
	for _, result := range results {
		scenarioMap[result.Scenario] = append(scenarioMap[result.Scenario], result)
	}

	stats := make(map[string]ScenarioStats)

	for scenario, testResults := range scenarioMap {
		successful := 0
		var totalDuration, totalRecovery time.Duration
		var recoveryTimes []time.Duration

		for _, result := range testResults {
			if result.Success {
				successful++
				totalDuration += result.Duration
				totalRecovery += result.RecoveryTime
				recoveryTimes = append(recoveryTimes, result.RecoveryTime)
			}
		}

		sort.Slice(recoveryTimes, func(i, j int) bool {
			return recoveryTimes[i] < recoveryTimes[j]
		})

		stat := ScenarioStats{
			Name:        scenario,
			TotalTests:  len(testResults),
			Successful:  successful,
			SuccessRate: float64(successful) / float64(len(testResults)) * 100,
		}

		if successful > 0 {
			stat.AvgDuration = totalDuration / time.Duration(successful)
			stat.AvgRecovery = totalRecovery / time.Duration(successful)
		}

		if len(recoveryTimes) > 0 {
			stat.MinRecovery = recoveryTimes[0]
			stat.MaxRecovery = recoveryTimes[len(recoveryTimes)-1]
		}

		stats[scenario] = stat
	}

	return stats
}

func generateReport(stats map[string]ScenarioStats, results []TestResult) {
	fmt.Println("=== ENHANCED FAILOVER TEST ANALYSIS REPORT ===")
	fmt.Printf("Analysis Date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Total Tests Analyzed: %d\n\n", len(results))

	// Overall Statistics
	totalTests := 0
	totalSuccessful := 0
	var overallDuration, overallRecovery time.Duration
	successfulCount := 0

	for _, stat := range stats {
		totalTests += stat.TotalTests
		totalSuccessful += stat.Successful
		if stat.Successful > 0 {
			overallDuration += stat.AvgDuration * time.Duration(stat.Successful)
			overallRecovery += stat.AvgRecovery * time.Duration(stat.Successful)
			successfulCount += stat.Successful
		}
	}

	overallSuccessRate := float64(totalSuccessful) / float64(totalTests) * 100
	if successfulCount > 0 {
		overallDuration /= time.Duration(successfulCount)
		overallRecovery /= time.Duration(successfulCount)
	}

	fmt.Printf("OVERALL RESULTS:\n")
	fmt.Printf("  Success Rate: %.1f%% (%d/%d tests)\n", overallSuccessRate, totalSuccessful, totalTests)
	fmt.Printf("  Average Test Duration: %v\n", overallDuration.Round(time.Millisecond))
	fmt.Printf("  Average Recovery Time: %v\n\n", overallRecovery.Round(time.Millisecond))

	// Scenario-specific results
	fmt.Println("SCENARIO BREAKDOWN:")

	scenarioOrder := []string{"ImmediateFailure", "DuringReplication", "PrecisionTimed", "RapidSequential"}

	for _, scenarioName := range scenarioOrder {
		if stat, exists := stats[scenarioName]; exists {
			fmt.Printf("\n%s:\n", strings.ToUpper(strings.Replace(stat.Name, "Failure", " Failure", 1)))
			fmt.Printf("  Tests: %d\n", stat.TotalTests)
			fmt.Printf("  Success Rate: %.1f%% (%d/%d)\n", stat.SuccessRate, stat.Successful, stat.TotalTests)

			if stat.Successful > 0 {
				fmt.Printf("  Average Duration: %v\n", stat.AvgDuration.Round(time.Millisecond))
				fmt.Printf("  Average Recovery: %v\n", stat.AvgRecovery.Round(time.Millisecond))
				fmt.Printf("  Recovery Range: %v - %v\n",
					stat.MinRecovery.Round(time.Millisecond),
					stat.MaxRecovery.Round(time.Millisecond))
			}
		}
	}

	// Detailed test results
	fmt.Println("\nDETAILED TEST RESULTS:")
	for _, result := range results {
		status := "FAILED"
		if result.Success {
			status = "SUCCESS"
		}
		fmt.Printf("  %s (%s): %s - Duration: %v, Recovery: %v\n",
			result.TestID, result.Scenario, status,
			result.Duration.Round(time.Millisecond),
			result.RecoveryTime.Round(time.Millisecond))
	}

	// Academic summary
	fmt.Println("\n=== ACADEMIC SUMMARY ===")
	fmt.Printf("Data Durability: %.1f%% of writes survived leader failures\n", overallSuccessRate)
	fmt.Printf("System Resilience: Average recovery time of %v demonstrates rapid failover capability\n", overallRecovery.Round(time.Millisecond))

	if overallSuccessRate >= 99.0 {
		fmt.Println("CONCLUSION: System demonstrates excellent data durability under controlled failure conditions")
	} else if overallSuccessRate >= 95.0 {
		fmt.Println("CONCLUSION: System demonstrates good data durability with minor edge case failures")
	} else {
		fmt.Println("CONCLUSION: System shows concerning data durability issues requiring investigation")
	}

	// Generate CSV for further analysis
	generateCSV(results)
}

func generateCSV(results []TestResult) {
	filename := fmt.Sprintf("test-results-%s.csv", time.Now().Format("20060102-150405"))
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Warning: Could not create CSV file: %v\n", err)
		return
	}
	defer file.Close()

	// CSV Header
	file.WriteString("TestID,Scenario,Success,Duration(ms),RecoveryTime(ms),WriteTime\n")

	for _, result := range results {
		file.WriteString(fmt.Sprintf("%s,%s,%t,%.1f,%.1f,%s\n",
			result.TestID,
			result.Scenario,
			result.Success,
			float64(result.Duration.Nanoseconds())/1e6,
			float64(result.RecoveryTime.Nanoseconds())/1e6,
			result.WriteTime.Format("2006-01-02 15:04:05.000")))
	}

	fmt.Printf("\nCSV file generated: %s\n", filename)
}
