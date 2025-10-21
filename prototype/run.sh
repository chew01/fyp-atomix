echo "Building docker image..."
docker build -t prototype-controller:local ./controller

echo "(Re)starting minikube..."
minikube delete
minikube start
minikube addons enable ingress
echo "Minikube started."

echo "Installing atomix-runtime..."
helm install -n kube-system atomix-runtime atomix/atomix-runtime --wait
echo "Installed atomix-runtime."


echo "Loading image into minikube..."
minikube image load prototype-controller:local
echo "Docker image loaded."

echo "Applying kubernetes configs..."
kubectl apply -f ./deploy/atomix/store.yaml
kubectl apply -f ./deploy/atomix/storage-profile.yaml
kubectl apply -f ./deploy/controller/controller-rbac.yaml
kubectl apply -f ./deploy/controller/deployment.yaml
kubectl expose deployment prototype --type=NodePort --port=8080
kubectl apply -f ./deploy/controller/ingress.yaml
echo "Kubernetes configs applied."

minikube tunnel

