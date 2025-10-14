#!/bin/bash

echo "Starting Atomix Concurrency Capability Test Setup..."

echo "(Re)starting minikube..."
minikube delete
minikube start

echo "Installing atomix-runtime..."
helm install -n kube-system atomix-runtime atomix/atomix-runtime --wait
echo "Installed atomix-runtime."

echo "Building docker image..."
docker build -t concurrency-client:local .
echo "Built docker image, loading into minikube..."
minikube image load concurrency-client:local
echo "Docker image loaded."

echo "Applying kubernetes configs..."
kubectl apply -f consensus-store.yaml
kubectl apply -f storage-profile.yaml

echo "Waiting for consensus-store to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=consensus-store --timeout=300s

echo "Applying deployment (with ServiceAccount and RBAC)..."
kubectl apply -f deployment.yaml

echo "Waiting for atomix-concurrency-experiment deployment to be ready..."
kubectl wait --for=condition=ready pod -l name=atomix-concurrency-experiment --timeout=120s

echo ""
echo "=== Atomix Concurrency Test Environment Ready ==="
echo ""
echo "To monitor the test:"
echo "  kubectl logs -f deployment/atomix-concurrency-experiment"
echo ""
echo "To access test results:"
echo "  kubectl exec -it deployment/atomix-concurrency-experiment -- cat /app/logs/concurrency-test-results.log"
echo "  kubectl exec -it deployment/atomix-concurrency-experiment -- cat /app/logs/concurrency-test-results.csv"
echo "  kubectl exec -it deployment/atomix-concurrency-experiment -- cat /app/logs/consistency-summary.txt"
echo ""
echo "To copy results to local machine:"
echo "  kubectl cp deployment/atomix-concurrency-experiment:/app/logs/concurrency-test-results.csv ./concurrency-test-results.csv"
echo "  kubectl cp deployment/atomix-concurrency-experiment:/app/logs/consistency-summary.txt ./consistency-summary.txt"
echo ""
echo "To analyze results locally:"
echo "  go run cmd-analyze-results.go concurrency-test-results.csv concurrency-test-results.log"
echo ""
echo "Test Configuration:"
echo "  - Concurrent Clients: 10"
echo "  - Operations per Client: 100"  
echo "  - Contention Keys: 5"
echo "  - Test Duration: 10 minutes"
echo "  - ConsensusStore Replicas: 3"
echo ""
echo "Tests to be performed:"
echo "  1. Linearizability Test: Multiple clients performing concurrent Put operations with sequence tracking"
echo "  2. Read-Your-Writes Test: Each client writes then immediately reads"
echo "  3. No Lost Updates Test: Concurrent Get-then-Update operations with versioning (adapted for available API)"
echo ""