#!/bin/bash

echo "Starting Atomix Failover Capability Test Setup..."

echo "(Re)starting minikube..."
minikube delete
minikube start

echo "Installing atomix-runtime..."
helm install -n kube-system atomix-runtime atomix/atomix-runtime --wait
echo "Installed atomix-runtime."

echo "Building docker image..."
docker build -t experiment-client:local .
echo "Built docker image, loading into minikube..."
minikube image load experiment-client:local
echo "Docker image loaded."

echo "Applying kubernetes configs..."
kubectl apply -f consensus-store.yaml
kubectl apply -f storage-profile.yaml

echo "Waiting for consensus-store to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=consensus-store --timeout=300s

echo "Applying deployment (with ServiceAccount and RBAC)..."
kubectl apply -f deployment.yaml

echo "Waiting for atomix-experiment deployment to be ready..."
kubectl wait --for=condition=ready pod -l name=atomix-experiment --timeout=120s

echo ""
echo "=== Atomix Failover Test Environment Ready ==="
echo ""
echo "To monitor the test:"
echo "  kubectl logs -f deployment/atomix-experiment"
echo ""
echo "To simulate leader failure during test:"
echo "  1. List consensus-store pods: kubectl get pods -l app.kubernetes.io/name=consensus-store"
echo "  2. Delete a pod: kubectl delete pod <pod-name>"
echo ""
echo "To access test logs from inside the pod:"
echo "  kubectl exec -it deployment/atomix-experiment -- cat /app/logs/failover-test-results.log"
echo ""
echo "Test Configuration:"
echo "  - Write Interval: 1 second"
echo "  - Read Interval: 2 seconds"  
echo "  - Test Duration: 10 minutes"
echo "  - ConsensusStore Replicas: 3"
echo ""

