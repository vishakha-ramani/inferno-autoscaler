#!/bin/bash

# Script to install and configure Prometheus using Helm and Kubernetes YAML files

echo "Step 1: Adding Prometheus Helm repository"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
if [ $? -ne 0 ]; then
    echo "Failed to add Helm repository. Exiting."
    exit 1
fi

echo "Step 2: Updating Helm repository to fetch the latest charts"
helm repo update
if [ $? -ne 0 ]; then
    echo "Failed to update Helm repository. Exiting."
    exit 1
fi

echo "Step 3: Creating 'monitoring' namespace in Kubernetes"
kubectl create namespace monitoring
if [ $? -ne 0 ]; then
    echo "Failed to create 'monitoring' namespace. Exiting."
    exit 1
fi

echo "Step 4: Installing kube-prometheus-stack using Helm in 'monitoring' namespace"
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n monitoring
if [ $? -ne 0 ]; then
    echo "Failed to install kube-prometheus-stack. Exiting."
    exit 1
fi

echo "Step 5: Applying custom configurations for Prometheus"

echo "Applying ServiceAccount configuration"
kubectl apply -f serviceaccount-prometheus.yaml
if [ $? -ne 0 ]; then
    echo "Failed to apply ServiceAccount configuration. Exiting."
    exit 1
fi

echo "Applying ClusterRole configuration"
kubectl apply -f clusterrole-prometheus.yaml
if [ $? -ne 0 ]; then
    echo "Failed to apply ClusterRole configuration. Exiting."
    exit 1
fi

echo "Applying ClusterRoleBinding configuration"
kubectl apply -f clusterrolebinding-prometheus.yaml
if [ $? -ne 0 ]; then
    echo "Failed to apply ClusterRoleBinding configuration. Exiting."
    exit 1
fi

echo "Applying Prometheus custom configuration"
kubectl apply -f prometheus.yaml
if [ $? -ne 0 ]; then
    echo "Failed to apply Prometheus configuration. Exiting."
    exit 1
fi

# Wait for 1 minute to ensure resources are properly created
echo "Waiting for 1 minute to allow resources to initialize..."
sleep 60

echo "Prometheus setup and configuration completed successfully."
