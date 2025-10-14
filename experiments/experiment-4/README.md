# Experiment 4: Enhanced Precision Failover Testing

## Overview

This experiment implements a rigorous, academically-oriented failover testing framework that provides definitive proof of write durability during the most vulnerable failure scenarios in distributed consensus systems. Unlike experiment-3's continuous stress testing, this experiment focuses on **precise, controlled failover scenarios** with detailed measurement and verification.

## Key Innovations

### 1. Precise Leader Identification
- **Deterministic Key-to-Partition Mapping**: Uses SHA-256 hash to deterministically map each key to a specific partition
- **Real-time Leader Tracking**: Continuously monitors which pod is the leader for each partition
- **Pre-failure Leader State**: Captures complete leader information before inducing failures

### 2. Controlled Failure Injection
- **Targeted Pod Termination**: Specifically terminates the leader pod responsible for a given key
- **Precise Timing Control**: Implements various timing windows from immediate (0ms) to delayed (200ms)
- **Forced Termination**: Uses Kubernetes API with zero grace period for immediate pod termination

### 3. Dual Read Testing Modes (**NEW**)
- **Immediate Read Testing**: Attempts to read data immediately after leader termination (simulates real-time client experience)
- **Post-Recovery Read Testing**: Waits for leader election completion before reading (validates data durability)
- **Comparative Analysis**: Provides success rates for both immediate availability and eventual consistency

### 4. Multiple Test Scenarios

#### Immediate Failure (Most Critical)
```
Write(key, value) -> [0ms delay] -> Terminate Leader -> [Immediate Read] -> Wait for Recovery -> [Post-Recovery Read]
```
**Academic Significance**: Tests the absolute worst-case scenario where leader fails immediately after write, measuring both immediate availability and eventual consistency

#### During Replication (Vulnerable Windows)
```
Write(key, value) -> [50ms/100ms/200ms delay] -> Terminate Leader -> [Immediate Read] -> Wait for Recovery -> [Post-Recovery Read]
```
**Academic Significance**: Tests various points during the replication process, comparing immediate vs. eventual data access

#### Precision Timed (Fine-grained Analysis)
```
Write(key, value) -> [10ms/25ms/75ms delay] -> Terminate Leader -> [Immediate Read] -> Wait for Recovery -> [Post-Recovery Read]
```
**Academic Significance**: Tests specific timing windows within the consensus protocol phases with dual read validation

#### Rapid Sequential (Cascading Failures)
```
Test1 -> Test2 -> Test3 (with minimal delays, both read modes)
```
**Academic Significance**: Tests system resilience under rapid successive failures across both read patterns

## Technical Architecture

### Enhanced Data Structures

```go
type ReadTestMode int
const (
    ImmediateRead    ReadTestMode = iota  // Read immediately after failure
    PostRecoveryRead                      // Read after leader recovery
)

type TestResult struct {
    TestID           string                // Unique test identifier
    Scenario         FailoverTestScenario  // Test scenario type
    ReadMode         ReadTestMode          // Read testing mode
    Key              string                // Test key
    Value            string                // Expected value
    WriteTime        time.Time             // Precise write timestamp
    FailureTime      time.Time             // Precise failure timestamp
    RecoveryTime     time.Time             // Leader recovery timestamp
    VerificationTime time.Time             // Verification timestamp
    Success          bool                  // Post-recovery test outcome
    Duration         time.Duration         // Total test duration
    LeaderBefore     LeaderInfo           // Leader state before failure
    LeaderAfter      LeaderInfo           // Leader state after recovery
    ImmediateReadErr string               // Immediate read error (empty if successful)
    ImmediateReadTime time.Time           // Immediate read attempt timestamp
}
```

### Key-to-Partition Algorithm
```go
func (eft *EnhancedFailoverTest) getKeyPartition(key string) int {
    hash := sha256.Sum256([]byte(key))
    return int(hash[0]) % eft.partitionCount
}
```
This ensures each test key is deterministically mapped to a specific partition, enabling precise leader targeting.

### Leader Identification Process
1. **Query Kubernetes API**: Retrieve all RaftGroup resources
2. **Extract Leader Information**: Parse leader pod name, term, and state
3. **Cache Leader State**: Maintain real-time cache of leader information
4. **Map Key to Leader**: Determine which pod is responsible for each test key

## Academic Validation Methodology

### 1. Pre-Test Validation
- Verify connectivity to distributed map
- Confirm leader election stability
- Validate partition mapping consistency

### 2. Enhanced Test Execution Protocol
```
Phase 1: Setup
    - Generate unique test key and value
    - Identify target partition and current leader
    - Record baseline leader state

Phase 2: Write Operation
    - Execute precise write operation
    - Record exact write timestamp
    - Confirm write acknowledgment

Phase 3: Failure Injection
    - Apply configured delay (0ms to 200ms)
    - Terminate specific leader pod
    - Record exact failure timestamp

Phase 4a: Immediate Read Test (NEW)
    - Attempt immediate read after leader termination
    - Record success/failure and error details
    - Capture client experience during failover

Phase 4b: Recovery Monitoring
    - Monitor leader election process
    - Wait for new leader establishment
    - Record recovery completion timestamp

Phase 5: Post-Recovery Verification
    - Read the same key from the distributed map
    - Verify value matches original write
    - Record verification timestamp and result
```

### 3. Enhanced Result Analysis
- **Post-Recovery Success Rate**: Percentage of tests where data survived leader failure (eventual consistency)
- **Immediate Read Success Rate**: Percentage of tests where data was accessible immediately after failure
- **Recovery Time**: Duration from failure to new leader election
- **Total Duration**: End-to-end test completion time
- **Comparative Analysis**: Success rates across immediate vs. post-recovery read modes
- **Scenario Comparison**: Performance across different timing windows and read modes
- **Client Experience Modeling**: Real-world failure impact on application availability

## Expected Academic Results

### Comprehensive Data Durability and Availability Analysis
The enhanced experiment provides quantitative evidence across two critical dimensions:

#### Data Durability (Post-Recovery Tests)
1. **Committed writes survive immediate leader failures** (Immediate Failure scenario)
2. **Writes during replication phases have predictable behavior** (During Replication scenario)
3. **System maintains consistency under rapid failures** (Rapid Sequential scenario)
4. **Fine-grained timing analysis reveals protocol characteristics** (Precision Timed scenario)

#### Real-Time Availability (Immediate Read Tests) (**NEW**)
1. **Client experience during leader transitions** measured for each scenario
2. **Service availability patterns** during consensus protocol execution
3. **Error modes and failure characteristics** during leader election
4. **Timeout and retry behavior** under different failure timing windows

### Enhanced Performance Characteristics
- **Leader Election Time**: Typical 1-3 seconds for new leader establishment
- **Write Durability**: 100% success rate expected for properly committed writes (post-recovery)
- **Immediate Availability**: Variable success rate depending on timing and replication state
- **Recovery Patterns**: Consistent behavior across different failure timing and read modes
- **Client Impact**: Quantified service degradation during failover periods

## Running the Experiment

### Prerequisites
- Minikube or Kubernetes cluster
- Helm 3.x
- Docker

### Setup and Execution

#### Default Comprehensive Testing (Recommended)
```bash
cd /Users/jun/fyp-atomix/experiments/experiment-4
./run.sh
```

#### Test Mode Configuration
The experiment supports two test modes via the `TEST_MODE` environment variable:

**Comprehensive Mode (Default):**
```bash
# Runs both immediate read and post-recovery read tests
export TEST_MODE=comprehensive
./run.sh
```

**Precision Mode (Backward Compatible):**
```bash
# Runs only post-recovery read tests (original behavior)
export TEST_MODE=precision
./run.sh
```

You can also modify the `deployment.yaml` file to change the default test mode:
```yaml
env:
- name: TEST_MODE
  value: "comprehensive"  # or "precision"
```

### Monitoring
```bash
# Real-time logs
kubectl logs -f deployment/enhanced-failover-experiment

# Detailed results
kubectl exec -it deployment/enhanced-failover-experiment -- cat /app/logs/enhanced-failover-test-results.log
```

## Enhanced Sample Output

### Immediate Read Test Sample
```
[2024-08-24 15:30:01.123] IMMEDIATE_READ_TEST_START: imm-test-000001, Scenario: ImmediateFailure, Delay: 0s
[2024-08-24 15:30:01.124] IMMEDIATE_WRITE: immediate-key-imm-test-000001 -> immediate-value-imm-test-000001-1724515801124 (partition 1, leader consensus-store-1-2)
[2024-08-24 15:30:01.126] WRITE_COMPLETE: imm-test-000001 (duration: 2ms)
[2024-08-24 15:30:01.126] FORCED_TERMINATION: Terminating leader pod consensus-store-1-2 for partition 1
[2024-08-24 15:30:01.127] TERMINATION_SUCCESS: Pod consensus-store-1-2 terminated
[2024-08-24 15:30:01.128] IMMEDIATE_READ_ATTEMPT: Trying immediate read after leader termination
[2024-08-24 15:30:01.133] IMMEDIATE_READ_FAILED: imm-test-000001 - Immediate read failed: context deadline exceeded
[2024-08-24 15:30:02.456] NEW_LEADER_ELECTED: Partition 1, Pod consensus-store-1-0, Term 5
[2024-08-24 15:30:02.457] LEADER_RECOVERY: imm-test-000001 (duration: 1.330s)
[2024-08-24 15:30:02.459] IMMEDIATE_TEST_COMPLETE: imm-test-000001 - Post-recovery: success, Immediate: failed (total duration: 1.335s)
```

### Post-Recovery Test Sample
```
[2024-08-24 15:30:05.123] PRECISION_TEST_START: test-000002, Scenario: ImmediateFailure, Delay: 0s
[2024-08-24 15:30:05.124] PRECISION_WRITE: precision-key-test-000002 -> precision-value-test-000002-1724515805124 (partition 2, leader consensus-store-2-1)
[2024-08-24 15:30:05.126] WRITE_COMPLETE: test-000002 (duration: 2ms)
[2024-08-24 15:30:05.126] FORCED_TERMINATION: Terminating leader pod consensus-store-2-1 for partition 2
[2024-08-24 15:30:05.127] TERMINATION_SUCCESS: Pod consensus-store-2-1 terminated
[2024-08-24 15:30:06.456] NEW_LEADER_ELECTED: Partition 2, Pod consensus-store-2-0, Term 3
[2024-08-24 15:30:06.457] LEADER_RECOVERY: test-000002 (duration: 1.330s)
[2024-08-24 15:30:06.459] PRECISION_SUCCESS: test-000002 verified (total duration: 1.335s)
```

## Comparison with Previous Versions

| Aspect | Experiment-3 | Experiment-4 (Original) | Experiment-4 (Enhanced) |
|--------|--------------|------------------------|------------------------|
| **Focus** | Continuous stress testing | Precise controlled testing | Comprehensive dual-mode testing |
| **Duration** | 10+ minutes | 2-3 minutes | 4-6 minutes |
| **Methodology** | Random failures | Targeted failures | Targeted failures with dual reads |
| **Read Testing** | Post-recovery only | Post-recovery only | **Immediate + Post-recovery** |
| **Measurements** | General statistics | Precise timing data | **Comparative availability analysis** |
| **Academic Value** | System stability | Protocol verification | **Client experience modeling** |
| **Failure Control** | Manual/Random | Automated/Precise | Automated/Precise with availability testing |
| **Test Count** | ~50-100 | ~10-15 | **~20-30 (doubled)** |

## Enhanced Academic Contributions

1. **Comprehensive Durability Proof**: Provides measurable evidence of distributed consensus durability across failure scenarios
2. **Dual-Mode Availability Analysis**: **NEW** - Quantifies both immediate availability and eventual consistency
3. **Client Experience Modeling**: **NEW** - Measures real-world impact on application availability during failures
4. **Timing Analysis**: Reveals system behavior at different failure timing windows for both read modes
5. **Comparative Protocol Analysis**: **NEW** - Direct comparison of immediate vs. post-recovery data access patterns
6. **Reproducible Results**: Deterministic testing methodology enables peer review
7. **Fine-grained Protocol Insights**: Analysis of consensus protocol characteristics under dual testing modes
8. **Production Environment Validation**: Real-world Kubernetes environment testing with comprehensive failure scenarios

### Key Academic Value Propositions

#### For Distributed Systems Research
- **Availability vs. Consistency Trade-offs**: Quantified measurements of the CAP theorem in practice
- **Failure Impact Assessment**: Precise measurement of client-visible service degradation
- **Protocol Behavior Analysis**: Detailed insights into multi-raft consensus under various failure conditions

#### For Academic Presentation
- **Comprehensive Dataset**: Both durability and availability metrics across multiple scenarios
- **Real-world Relevance**: Practical insights for production distributed systems design
- **Reproducible Methodology**: Scientific rigor with deterministic test execution

This enhanced testing framework provides the comprehensive, dual-perspective analysis needed for academic presentation while delivering practical insights for production distributed systems architecture and client application design.