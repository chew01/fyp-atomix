# CLAUDE.md - Experiment 4: Enhanced Precision Failover Testing

This file provides guidance to Claude Code (claude.ai/code) when working with this enhanced distributed consensus failover testing experiment.

## Project Overview
Enhanced Atomix distributed consensus experiment implementing precise data durability validation during leader failures with deterministic key-to-partition mapping, controlled failure injection, and comprehensive academic-quality analysis.

## Key Objectives
- Implement precise failover test protocol with targeted leader termination
- Validate data durability during the most vulnerable failure scenarios  
- Provide quantitative evidence suitable for academic presentation
- Test multiple timing windows from immediate (0ms) to delayed (200ms) failures
- Generate comprehensive analysis reports with statistical significance

## Project Structure

```
.
├── CLAUDE.md                           # This file - project instructions
├── README.md                          # Comprehensive experiment overview
├── METHODOLOGY.md                     # Academic methodology documentation
├── main.go                           # Enhanced failover testing application
├── analyze-results.go                # Results analysis and reporting tool
├── go.mod                           # Go module dependencies
├── Dockerfile                       # Container build configuration
├── consensus-store.yaml             # ConsensusStore deployment (3 replicas, 3 groups)
├── storage-profile.yaml             # Storage profile binding
├── deployment.yaml                  # Application deployment with enhanced RBAC
└── run.sh                          # Complete environment setup script
```

## Enhanced Features

### Precision Testing Capabilities
- **Deterministic Key-Partition Mapping**: SHA-256 hash-based key assignment to specific partitions
- **Real-time Leader Identification**: Live tracking of leader pods for each partition
- **Targeted Failure Injection**: Precise termination of leader responsible for specific keys
- **Multiple Test Scenarios**: Immediate, during replication, rapid sequential, and precision timed failures
- **Comprehensive Verification**: End-to-end validation of write durability

### Academic Quality Measurements
- **Nanosecond Precision Timing**: Exact timestamps for all critical events
- **Leader State Tracking**: Complete before/after leader information
- **Statistical Analysis**: Success rates, recovery times, and correlation analysis
- **Detailed Reporting**: Academic-grade results suitable for peer review

## Test Scenarios

### 1. Immediate Failure (Critical Test)
```
Write(key, value) → [0ms] → Terminate Leader → Verify Data Survival
```
Tests absolute worst-case scenario of immediate leader failure after write.

### 2. During Replication (Vulnerable Windows)
```
Write(key, value) → [50ms/100ms/200ms] → Terminate Leader → Verify Data Survival
```
Tests various phases of the consensus replication process.

### 3. Precision Timed (Fine-grained Analysis)
```
Write(key, value) → [10ms/25ms/75ms] → Terminate Leader → Verify Data Survival
```
Fine-grained analysis of specific timing windows within consensus protocol.

### 4. Rapid Sequential (Cascading Failures)
```
Test1 → Test2 → Test3 (minimal delays between tests)
```
Tests system resilience under rapid successive leader failures.

## Key Technical Innovations

### Deterministic Partition Assignment
```go
func (eft *EnhancedFailoverTest) getKeyPartition(key string) int {
    hash := sha256.Sum256([]byte(key))
    return int(hash[0]) % eft.partitionCount
}
```

### Precise Leader Targeting
- Real-time Kubernetes API queries for RaftGroup resources
- Extraction of leader pod names, terms, and states
- Cached leader information with automatic updates
- Mapping of test keys to their responsible leader pods

### Enhanced RBAC Permissions
```yaml
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch", "delete"]  # Critical: includes "delete" for pod termination
```

## Commands and Usage

### Complete Setup and Execution
```bash
cd /Users/jun/fyp-atomix/experiments/experiment-4
./run.sh                    # Full environment setup and deployment
```

### Monitoring and Analysis
```bash
# Real-time test monitoring
kubectl logs -f deployment/enhanced-failover-experiment

# Access detailed results
kubectl exec -it deployment/enhanced-failover-experiment -- cat /app/logs/enhanced-failover-test-results.log

# Copy logs for analysis
kubectl cp deployment/enhanced-failover-experiment:/app/logs/enhanced-failover-test-results.log ./test-results.log

# Run analysis tool
go run analyze-results.go test-results.log
```

### Individual Components
```bash
# Build and deploy application
docker build -t enhanced-failover-client:local .
minikube image load enhanced-failover-client:local

# Kubernetes resources
kubectl apply -f test.yaml      # Deploy consensus store
kubectl apply -f storage-profile.yaml      # Configure storage bindings  
kubectl apply -f deployment.yaml           # Deploy enhanced testing app

# Check system status
kubectl get multiraftclusters             # Verify consensus cluster
kubectl get pods -l app.kubernetes.io/name=consensus-store  # Check store pods
kubectl describe storageprofile enhanced-failover-experiment  # Verify bindings
```

## Expected Results and Analysis

### Success Metrics
- **Data Durability**: >99% write survival rate expected
- **Recovery Time**: Consistent 1-3 second leader election
- **Scenario Performance**: Similar success rates across timing windows
- **System Resilience**: Rapid recovery from cascading failures

### Analysis Output
```
=== ENHANCED FAILOVER TEST ANALYSIS REPORT ===
OVERALL RESULTS:
  Success Rate: 100.0% (12/12 tests)
  Average Test Duration: 1.337s
  Average Recovery Time: 1.245s

SCENARIO BREAKDOWN:
IMMEDIATE FAILURE:
  Success Rate: 100.0% (3/3)
  Average Recovery: 1.156s

DURING REPLICATION:
  Success Rate: 100.0% (3/3)  
  Average Recovery: 1.298s
```

## Academic Contributions

### Quantitative Evidence
- Measurable proof of distributed consensus durability
- Statistical analysis of recovery characteristics
- Correlation studies between failure timing and success rates

### Reproducible Framework
- Deterministic testing methodology
- Standardized measurement protocols
- Peer-reviewable experimental design

### Practical Insights
- Real-world Kubernetes deployment validation
- Production-relevant failure scenarios
- Performance characteristics under stress

## Comparison with Experiment-3

| Aspect | Experiment-3 | Experiment-4 |
|--------|--------------|--------------|
| **Approach** | Continuous stress testing | Controlled precision testing |
| **Duration** | 10+ minutes | 2-3 minutes focused tests |
| **Failure Method** | Manual/random pod deletion | Automated targeted termination |
| **Measurements** | General monitoring | Precise timing analysis |
| **Academic Value** | System stability proof | Protocol validation |
| **Reproducibility** | Variable | Deterministic |

## Development Guidelines

### Code Quality
- Self-documenting code without comments (per project standards)
- Comprehensive error handling and logging
- Precise timing measurements with nanosecond accuracy
- Clean separation of test phases and responsibilities

### Testing Approach
- Deterministic test execution for reproducibility
- Comprehensive validation of all assumptions
- Graceful handling of edge cases and errors
- Detailed logging for post-analysis

### Academic Standards
- Rigorous methodology documentation
- Statistical significance in measurements  
- Peer-reviewable experimental design
- Clear presentation of results and limitations

## Important Implementation Notes

- **Pod Termination**: Requires enhanced RBAC with pod deletion permissions
- **Leader Identification**: Uses real-time Kubernetes API queries for accuracy
- **Timing Precision**: All measurements use Go's high-resolution time package
- **Data Structures**: Cannot use `atomix.Map` as struct field - must instantiate per use
- **Error Handling**: Comprehensive validation at each test phase
- **Resource Management**: Proper cleanup and graceful shutdown handling

This enhanced experiment provides the rigorous, scientific validation needed for academic presentation while maintaining practical relevance for production distributed systems.