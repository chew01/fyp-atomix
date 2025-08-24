echo "(Re)starting minikube..."
minikube delete
minikube start

echo "Installing atomix-runtime..."
helm install -n kube-system atomix-runtime atomix/atomix-runtime --wait
echo "Installed atomix-runtime."

echo "Building docker image..."
docker build -t consensusmap-client:local .
echo "Built docker image, loading into minikube..."
minikube image load consensusmap-client:local
echo "Docker image loaded."

echo "Applying kubernetes configs..."
kubectl apply -f consensus-store.yaml
kubectl apply -f storage-profile.yaml
kubectl apply -f deployment.yaml
echo "Kubernetes configs applied."

