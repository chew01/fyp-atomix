package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ConcurrencyTestResult struct {
	TestType      string
	ClientID      string
	Operation     string
	Success       bool
	Duration      time.Duration
	Timestamp     time.Time
	Details       string
}

type TestSummary struct {
	TestType         string
	TotalOperations  int
	SuccessfulOps    int
	FailedOps        int
	SuccessRate      float64
	AvgDuration      time.Duration
	MinDuration      time.Duration
	MaxDuration      time.Duration
	ClientCount      int
}

type ClientPerformance struct {
	ClientID        string
	TestType        string
	TotalOps        int
	SuccessfulOps   int
	SuccessRate     float64
	AvgDuration     time.Duration
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run analyze-results.go <csv-file> [log-file]")
		os.Exit(1)
	}

	csvFile := os.Args[1]
	var logFile string
	if len(os.Args) > 2 {
		logFile = os.Args[2]
	} else {
		logFile = "concurrency-test-results.log"
	}

	results, err := parseCSVResults(csvFile)
	if err != nil {
		log.Fatalf("Failed to parse CSV file: %v", err)
	}

	logResults, err := parseLogFile(logFile)
	if err != nil {
		fmt.Printf("Warning: Could not parse log file: %v\n", err)
	}

	generateAnalysisReport(results, logResults)
	generateDetailedCSV(results)
}

func parseCSVResults(filename string) ([]ConcurrencyTestResult, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var results []ConcurrencyTestResult

	// Skip header
	for i := 1; i < len(records); i++ {
		record := records[i]
		if len(record) < 9 {
			continue
		}

		success, _ := strconv.ParseBool(record[6])
		duration, _ := time.ParseDuration(record[7])
		timestamp, _ := time.Parse("2006-01-02 15:04:05.000", record[5])

		result := ConcurrencyTestResult{
			TestType:  record[0],
			ClientID:  record[1],
			Operation: record[3],
			Success:   success,
			Duration:  duration,
			Timestamp: timestamp,
			Details:   record[8],
		}

		results = append(results, result)
	}

	return results, nil
}

func parseLogFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	results := make(map[string]string)
	scanner := bufio.NewScanner(file)

	// Key patterns to extract from logs
	patterns := map[string]*regexp.Regexp{
		"linearizability_result": regexp.MustCompile(`LINEARIZABILITY_RESULT: Final counter value: (\d+), Expected: (\d+)`),
		"linearizability_pass":   regexp.MustCompile(`LINEARIZABILITY_PASS: Counter value matches expected operations`),
		"linearizability_fail":   regexp.MustCompile(`LINEARIZABILITY_FAIL: Counter value (\d+) does not match expected (\d+)`),
		"ryw_result":            regexp.MustCompile(`READ_YOUR_WRITES_RESULT: (\d+)/(\d+) total pairs consistent \(([0-9.]+)%\)`),
		"cas_result":            regexp.MustCompile(`NO_LOST_UPDATES_RESULT: (\d+)/(\d+) CAS operations successful \(([0-9.]+)%\)`),
		"lost_updates_fail":     regexp.MustCompile(`NO_LOST_UPDATES_FAIL: Lost updates detected! (\d+) operations lost`),
		"lost_updates_pass":     regexp.MustCompile(`NO_LOST_UPDATES_PASS: No lost updates detected`),
		"overall_result":        regexp.MustCompile(`OVERALL_RESULT: (true|false)`),
	}

	for scanner.Scan() {
		line := scanner.Text()
		for key, pattern := range patterns {
			if match := pattern.FindStringSubmatch(line); len(match) > 0 {
				results[key] = strings.Join(match[1:], "|")
			}
		}
	}

	return results, scanner.Err()
}

func generateAnalysisReport(results []ConcurrencyTestResult, logResults map[string]string) {
	fmt.Println("=== ATOMIX CONCURRENCY CAPABILITY TEST ANALYSIS ===")
	fmt.Printf("Analysis Date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Total Operations Analyzed: %d\n\n", len(results))

	// Group results by test type
	testGroups := make(map[string][]ConcurrencyTestResult)
	for _, result := range results {
		testGroups[result.TestType] = append(testGroups[result.TestType], result)
	}

	// Generate test summaries
	var summaries []TestSummary
	for testType, testResults := range testGroups {
		summary := analyzeTestType(testType, testResults)
		summaries = append(summaries, summary)
	}

	// Sort summaries by test type
	sort.Slice(summaries, func(i, j int) bool {
		order := map[string]int{"Linearizability": 1, "ReadYourWrites": 2, "NoLostUpdates": 3}
		return order[summaries[i].TestType] < order[summaries[j].TestType]
	})

	// Overall statistics
	totalOps := 0
	successfulOps := 0
	var totalDuration time.Duration

	for _, summary := range summaries {
		totalOps += summary.TotalOperations
		successfulOps += summary.SuccessfulOps
		if summary.SuccessfulOps > 0 {
			totalDuration += summary.AvgDuration * time.Duration(summary.SuccessfulOps)
		}
	}

	overallSuccessRate := float64(successfulOps) / float64(totalOps) * 100
	avgDuration := time.Duration(0)
	if successfulOps > 0 {
		avgDuration = totalDuration / time.Duration(successfulOps)
	}

	fmt.Printf("OVERALL PERFORMANCE:\n")
	fmt.Printf("  Total Operations: %d\n", totalOps)
	fmt.Printf("  Successful Operations: %d (%.1f%%)\n", successfulOps, overallSuccessRate)
	fmt.Printf("  Average Operation Duration: %v\n\n", avgDuration.Round(time.Microsecond))

	// Test-specific results
	fmt.Println("TEST TYPE BREAKDOWN:")
	for _, summary := range summaries {
		fmt.Printf("\n%s:\n", strings.ToUpper(summary.TestType))
		fmt.Printf("  Total Operations: %d\n", summary.TotalOperations)
		fmt.Printf("  Success Rate: %.1f%% (%d successful, %d failed)\n", 
			summary.SuccessRate, summary.SuccessfulOps, summary.FailedOps)
		fmt.Printf("  Unique Clients: %d\n", summary.ClientCount)
		fmt.Printf("  Average Duration: %v\n", summary.AvgDuration.Round(time.Microsecond))
		fmt.Printf("  Duration Range: %v - %v\n", 
			summary.MinDuration.Round(time.Microsecond), 
			summary.MaxDuration.Round(time.Microsecond))
	}

	// Log-based consistency analysis
	fmt.Println("\n=== CONSISTENCY GUARANTEE ANALYSIS ===")
	
	if finalResult, exists := logResults["linearizability_result"]; exists {
		parts := strings.Split(finalResult, "|")
		if len(parts) >= 2 {
			actualValue := parts[0]
			expectedValue := parts[1]
			fmt.Printf("LINEARIZABILITY:\n")
			fmt.Printf("  Final Counter: %s (Expected: %s)\n", actualValue, expectedValue)
			
			if _, passExists := logResults["linearizability_pass"]; passExists {
				fmt.Printf("  Result: PASS - All operations preserved atomically\n")
			} else if failData, failExists := logResults["linearizability_fail"]; failExists {
				failParts := strings.Split(failData, "|")
				if len(failParts) >= 2 {
					fmt.Printf("  Result: FAIL - Lost %s operations out of %s\n", 
						failParts[1], failParts[0])
				}
			}
		}
	}

	if rywResult, exists := logResults["ryw_result"]; exists {
		parts := strings.Split(rywResult, "|")
		if len(parts) >= 3 {
			consistent := parts[0]
			total := parts[1]
			percentage := parts[2]
			fmt.Printf("\nREAD-YOUR-WRITES:\n")
			fmt.Printf("  Consistent Pairs: %s/%s (%.1s%%)\n", consistent, total, percentage)
			
			if percentage == "100.0" {
				fmt.Printf("  Result: PASS - Perfect read-your-writes consistency\n")
			} else {
				fmt.Printf("  Result: PARTIAL - Some consistency violations detected\n")
			}
		}
	}

	if casResult, exists := logResults["cas_result"]; exists {
		parts := strings.Split(casResult, "|")
		if len(parts) >= 3 {
			successful := parts[0]
			total := parts[1]
			percentage := parts[2]
			fmt.Printf("\nNO LOST UPDATES:\n")
			fmt.Printf("  Successful CAS Operations: %s/%s (%.1s%%)\n", successful, total, percentage)
			
			if _, passExists := logResults["lost_updates_pass"]; passExists {
				fmt.Printf("  Result: PASS - No updates lost in concurrent operations\n")
			} else if lostData, failExists := logResults["lost_updates_fail"]; failExists {
				fmt.Printf("  Result: FAIL - %s operations lost due to race conditions\n", lostData)
			}
		}
	}

	// Overall consistency verdict
	fmt.Println("\n=== ACADEMIC SUMMARY ===")
	if overallResult, exists := logResults["overall_result"]; exists {
		if overallResult == "true" {
			fmt.Println("CONCLUSION: System demonstrates all three consistency guarantees successfully")
			fmt.Printf("  - Linearizability: Operations appear atomic and ordered\n")
			fmt.Printf("  - Read-Your-Writes: Clients always see their own writes immediately\n")
			fmt.Printf("  - No Lost Updates: Concurrent operations preserve all intended modifications\n")
		} else {
			fmt.Println("CONCLUSION: System failed to meet one or more consistency guarantees")
			fmt.Printf("  - Review individual test results above for specific violations\n")
		}
	}

	fmt.Printf("\nPerformance Characteristics:\n")
	fmt.Printf("  - Average operation latency: %v\n", avgDuration.Round(time.Microsecond))
	fmt.Printf("  - Overall operation success rate: %.1f%%\n", overallSuccessRate)
	fmt.Printf("  - System demonstrated under concurrent load with multiple clients\n")

	// Client performance analysis
	fmt.Println("\n=== CLIENT PERFORMANCE ANALYSIS ===")
	clientPerf := analyzeClientPerformance(results)
	
	for testType, clients := range clientPerf {
		fmt.Printf("\n%s Client Performance:\n", strings.ToUpper(testType))
		
		// Sort clients by success rate
		sort.Slice(clients, func(i, j int) bool {
			return clients[i].SuccessRate > clients[j].SuccessRate
		})
		
		bestClient := clients[0]
		worstClient := clients[len(clients)-1]
		
		fmt.Printf("  Best Performing Client: %s (%.1f%% success, avg %v)\n", 
			bestClient.ClientID, bestClient.SuccessRate, bestClient.AvgDuration.Round(time.Microsecond))
		fmt.Printf("  Worst Performing Client: %s (%.1f%% success, avg %v)\n", 
			worstClient.ClientID, worstClient.SuccessRate, worstClient.AvgDuration.Round(time.Microsecond))
		
		// Calculate performance spread
		rateSpread := bestClient.SuccessRate - worstClient.SuccessRate
		if rateSpread < 5.0 {
			fmt.Printf("  Performance Consistency: EXCELLENT (%.1f%% spread)\n", rateSpread)
		} else if rateSpread < 10.0 {
			fmt.Printf("  Performance Consistency: GOOD (%.1f%% spread)\n", rateSpread)
		} else {
			fmt.Printf("  Performance Consistency: POOR (%.1f%% spread)\n", rateSpread)
		}
	}
}

func analyzeTestType(testType string, results []ConcurrencyTestResult) TestSummary {
	summary := TestSummary{
		TestType: testType,
	}

	clientSet := make(map[string]bool)
	var durations []time.Duration

	for _, result := range results {
		summary.TotalOperations++
		clientSet[result.ClientID] = true

		if result.Success {
			summary.SuccessfulOps++
			durations = append(durations, result.Duration)
		} else {
			summary.FailedOps++
		}
	}

	summary.ClientCount = len(clientSet)
	summary.SuccessRate = float64(summary.SuccessfulOps) / float64(summary.TotalOperations) * 100

	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})

		summary.MinDuration = durations[0]
		summary.MaxDuration = durations[len(durations)-1]

		var totalDuration time.Duration
		for _, d := range durations {
			totalDuration += d
		}
		summary.AvgDuration = totalDuration / time.Duration(len(durations))
	}

	return summary
}

func analyzeClientPerformance(results []ConcurrencyTestResult) map[string][]ClientPerformance {
	// Group by test type and client
	clientGroups := make(map[string]map[string][]ConcurrencyTestResult)

	for _, result := range results {
		if clientGroups[result.TestType] == nil {
			clientGroups[result.TestType] = make(map[string][]ConcurrencyTestResult)
		}
		clientGroups[result.TestType][result.ClientID] = append(clientGroups[result.TestType][result.ClientID], result)
	}

	performance := make(map[string][]ClientPerformance)

	for testType, clients := range clientGroups {
		for clientID, clientResults := range clients {
			perf := ClientPerformance{
				ClientID: clientID,
				TestType: testType,
				TotalOps: len(clientResults),
			}

			var totalDuration time.Duration
			successCount := 0

			for _, result := range clientResults {
				if result.Success {
					successCount++
					totalDuration += result.Duration
				}
			}

			perf.SuccessfulOps = successCount
			perf.SuccessRate = float64(successCount) / float64(len(clientResults)) * 100

			if successCount > 0 {
				perf.AvgDuration = totalDuration / time.Duration(successCount)
			}

			performance[testType] = append(performance[testType], perf)
		}
	}

	return performance
}

func generateDetailedCSV(results []ConcurrencyTestResult) {
	filename := fmt.Sprintf("detailed-analysis-%s.csv", time.Now().Format("20060102-150405"))
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Warning: Could not create detailed CSV: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	writer.Write([]string{
		"TestType", "ClientID", "Operation", "Success", 
		"DurationMicroseconds", "Timestamp", "Details",
	})

	// Write data
	for _, result := range results {
		writer.Write([]string{
			result.TestType,
			result.ClientID,
			result.Operation,
			strconv.FormatBool(result.Success),
			fmt.Sprintf("%.1f", float64(result.Duration.Nanoseconds())/1000.0),
			result.Timestamp.Format("2006-01-02 15:04:05.000"),
			result.Details,
		})
	}

	fmt.Printf("\nDetailed analysis CSV generated: %s\n", filename)
}