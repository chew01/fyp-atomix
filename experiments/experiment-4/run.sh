#!/bin/bash

echo "Starting Enhanced Atomix Precision Failover Test Setup..."

echo "(Re)starting minikube..."
minikube delete
minikube start

echo "Installing atomix-runtime..."
helm install -n kube-system atomix-runtime atomix/atomix-runtime --wait
echo "Installed atomix-runtime."

echo "Building docker image..."
docker build -t enhanced-failover-client:local .
echo "Built docker image, loading into minikube..."
minikube image load enhanced-failover-client:local
echo "Docker image loaded."

echo "Applying kubernetes configs..."
kubectl apply -f consensus-store.yaml
kubectl apply -f storage-profile.yaml

echo "Waiting for consensus-store to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=consensus-store --timeout=300s

echo "Applying deployment (with enhanced RBAC permissions)..."
kubectl apply -f deployment.yaml

echo "Waiting for enhanced-failover-experiment deployment to be ready..."
kubectl wait --for=condition=ready pod -l name=enhanced-failover-experiment --timeout=120s

echo ""
echo "=== Enhanced Atomix Precision Failover Test Environment Ready ==="
echo ""
echo "To monitor the test:"
echo "  kubectl logs -f deployment/enhanced-failover-experiment"
echo ""
echo "To access detailed test logs:"
echo "  kubectl exec -it deployment/enhanced-failover-experiment -- cat /app/logs/enhanced-failover-test-results.log"
echo ""
echo "Test Features:"
echo "  - Automated leader identification for specific keys"
echo "  - Precise timing control for failure injection"
echo "  - Multiple test scenarios (immediate, during replication, rapid sequential)"
echo "  - Comprehensive verification of data durability"
echo "  - Detailed academic-quality reporting"
echo ""
echo "Test Scenarios:"
echo "  1. Immediate Failure: Write -> Immediate leader termination -> Verify"
echo "  2. During Replication: Write -> Timed delay -> Leader termination -> Verify"
echo "  3. Rapid Sequential: Multiple rapid failures in succession"
echo "  4. Precision Timed: Various precise timing windows (10ms, 25ms, 75ms)"
echo ""
echo "ConsensusStore Configuration:"
echo "  - 3 Replicas distributed across 3 consensus groups"
echo "  - Each key is deterministically mapped to a specific partition"
echo "  - Enhanced RBAC permissions for pod termination"
echo ""