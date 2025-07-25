#!/bin/bash

# author: Chirag C. Shetty (cshetty2@illinois.edu)
# date: Sep 14, 2023 

# run as: . ./cloudlab_k8s_setup.sh
# does: the setup required to start a k8s cluster. At the end will give instructions to setup the network

#Source: https://www.linuxtechi.com/install-kubernetes-on-ubuntu-22-04/

#########################
alias dividerInstruction="printf '%*s\n' "${COLUMNS:-$(tput cols)}" '' | tr ' ' -;"
alias dividerAttention="printf '%*s\n' "${COLUMNS:-$(tput cols)}" '' | tr ' ' '*';"
alias dividerAction="printf '%*s\n' "${COLUMNS:-$(tput cols)}" '' | tr ' ' +;"
alias dividerEnd="printf '%*s\n' "${COLUMNS:-$(tput cols)}" '' | tr ' ' =;"
###########################

echo "Assumes you have acquired a cluster of atleast 2 machines on cloudlab"
read -p "Press Enter once done:"

echo -e "\nThis script must be run once on EACH node you want in the cluster"
echo -e "Recommended: \n1. Run all scripts TOGETHER on all machines so that you can collect the ip data to be added to /etc/hosts"
echo -e "\n2. Keep 'node0' - as named by cloudlab - in the cluster. It will be made the k8smaster by this script. Else you need to manually edit the /etc/hosts file"
read -p "Press Enter once done:"

######################## Setup /etc/hosts settings #############################
dividerAction
echo "In cloudlab, nodes reserved as a cluster are named node0, node1, node2 etc. By default we will define 'node0' as the control-plane node or 'k8smaster'. All other node<i> will be named k8sworker<i> in /etc/hosts"
echo -e "\nNOTE: If you want to manually change the etc/hosts different from this convention, skip next step"


read -p "Do you want script to automatically modify the /etc/hosts ? Enter 'y', if yes: " inp

if [ $inp = 'y' ]
then
    echo "Hostname: $HOSTNAME"
    node_name=$(echo $HOSTNAME | cut -d'.' -f1 | tail -c2)


    echo -e "\nCollect lines like below from each machine into a file."

    dividerAttention
    if [ $node_name -eq 0 ] ; 
    then 
        k8s_node_name="k8master"; 
        ifconfig | grep -m 1 'inet'  | tr -s ' ' | cut -d " " -f 3 | sed "s/$/   $HOSTNAME    $k8s_node_name/" ; 
    else 
        ifconfig | grep -m 1 'inet'  | tr -s ' ' | cut -d " " -f 3 | sed "s/$/   $HOSTNAME    k8sworker$node_name/"; 
    fi
    dividerAttention


    echo -e "\nPlease enter the  'ip_adress hostname k8s_node_name' for all nodes to be added to the cluster:"
    echo -e "Example:\n128.110.217.113  node0.k8ssetup.dcsq-pg0.utah.cloudlab.us    k8smaster\n128.110.217.81   node1.k8ssetup.dcsq-pg0.utah.cloudlab.us    k8sworker1\n......\nEOF\n"
    dividerAction
    echo > hosts_detail.txt
    echo -e "Enter(end with blank line):"
    while read line; do [ -z "$line" ] && break; echo "$line" >> hosts_detail.txt ; done

else

    echo -e "\nNow collect ip adresses of all nodes as 'ip_adress hostname k8s_node_name' ."
    echo -e "Example:\n128.110.217.113  node0.k8ssetup.dcsq-pg0.utah.cloudlab.us    k8smaster\n128.110.217.81   node1.k8ssetup.dcsq-pg0.utah.cloudlab.us    k8sworker1\n......"
    echo -e "\nWrite them to a file hosts_detail.txt"
    read -p "Press Enter once done:"
    dividerAction
fi

if [ -f "hosts_detail.txt" ]; then
    echo -e "\nhosts_detail.txt exists."
else
    echo -e "\nError: hosts_detail.txt does NOT exists."
    kill -INT $$   # Equivalent of CTRL+C
fi

dividerInstruction

## Append to hosts_detail.txt
cat hosts_detail.txt | sudo tee -a /etc/hosts
dividerInstruction
#Check
cat /etc/hosts

##############################################################################


sudo apt update

## Execute beneath swapoff and sed command to disable swap.
sudo swapoff -a
sudo sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab

## Kernel modules. originally explained here: https://kubernetes.io/docs/setup/production-environment/container-runtimes/
sudo tee /etc/modules-load.d/containerd.conf <<EOF
overlay
br_netfilter
EOF
sudo modprobe overlay
sudo modprobe br_netfilter

sudo tee /etc/sysctl.d/kubernetes.conf <<EOF
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
EOF

## Reload the above changes
sudo sysctl --system

## Verify the modules are running
lsmod | grep br_netfilter
lsmod | grep overlay
sysctl net.bridge.bridge-nf-call-iptables net.bridge.bridge-nf-call-ip6tables net.ipv4.ip_forward


## Install Containerd runtime
sudo apt install -y curl gnupg2 software-properties-common apt-transport-https ca-certificates

## Enable docker repository
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmour -o /etc/apt/trusted.gpg.d/docker.gpg
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

sudo apt update

##########################################
echo "Installing Docker and containerd:"
echo "" 

sudo apt-get update
sudo apt-get -y install \
    ca-certificates \
    curl \
    gnupg lsb-release

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSLk https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg  ## download without verification. Used to work without 'k'. Not sure why it doesnt. TODO: Fix
#curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch="$(dpkg --print-architecture)" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  "$(. /etc/os-release && echo "$VERSION_CODENAME")" stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null


sudo apt-get update #installation below failed without this update

sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

echo -e "Checking Docker:\n"

sudo docker run hello-world
########################################


containerd config default | sudo tee /etc/containerd/config.toml >/dev/null 2>&1
sudo sed -i 's/SystemdCgroup \= false/SystemdCgroup \= true/g' /etc/containerd/config.toml
## Below is because kubeadm init gives the warning:
## detected that the sandbox image "registry.k8s.io/pause:3.6" of the container runtime is inconsistent with that used by kubeadm. It is recommended that using "registry.k8s.io/pause:3.9" as the CRI sandbox image.
sudo sed -i 's|sandbox_image = "registry.k8s.io/pause:3.*|sandbox_image = "registry.k8s.io/pause:3.9"|' /etc/containerd/config.toml

# Restart and enable containerd service
sudo systemctl restart containerd
sudo systemctl enable containerd

####
# Kubernetes package is not available in the default Ubuntu 22.04 package repositories. So we need to add kubernetes repositories
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo gpg --dearmour -o /etc/apt/trusted.gpg.d/kubernetes-xenial.gpg
sudo apt-add-repository "deb http://apt.kubernetes.io/ kubernetes-xenial main"
## Note: At time of writing this guide, Xenial is the latest Kubernetes repository but when repository is available for Ubuntu 22.04 (Jammy Jellyfish) then you need replace xenial word with ‘jammy’ in ‘apt-add-repository’ command.

sudo apt update
sudo apt install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl

###################################################################################

dividerAction
echo ""
read -p "If this node is the master, enter 'm'" inp

dividerAttention
if [ $inp = 'm' ]
then

    ### Install helm
    curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
    chmod 700 get_helm.sh
    ./get_helm.sh
    ################
    echo ""
    network_type='f'
    read -p "By default we use flannel network plugin. If you want Calico instead, enter 'c'" network_type
    
    dividerAttention
    echo -e "\n Please do the following:"
    
    if [ $network_type = 'c' ]
    then
        ## Calico
        echo -e "\n0. RUN and wait: \n  sudo kubeadm init --control-plane-endpoint=$HOSTNAME"
        
    else
        ## Flannel
        echo -e "\n0. RUN and wait: \n  sudo kubeadm init --control-plane-endpoint=$HOSTNAME --pod-network-cidr=10.244.0.0/16 "
        
    fi

    
    echo -e "\n1. Check that it says: the control plane initialization was successful"
    echo -e "\n2. Execute the instructions given at the end of command in step (0) to setup .kube/config"
    echo -e "\n3. Note the command to run on worker nodes to connect them to the network (Common errors: You may need to add sudo before the command). Take care to copy the command for joining new worker nodes and not new control nodes"
    echo -e "\n4. Run the following command to deploy the network:"
    if [ $network_type = 'c' ]
    then
        ## Calico
        echo -e  "\n kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.25.0/manifests/calico.yaml"
    else
        ## Flannel
        echo -e "\n sudo kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml"
    fi
    echo -e "\n5. Note that all kube-system nodes look fine: kubectl get pods -n kube-system"
    echo -e "\n6. As you add more workers, check they are all ready (note they will be ready only after network plugin is deployed): kubectl get nodes"

else
    echo -e "Note down the command from master node to join the k8s network and run"
fi

echo "Script complete"
dividerAttention
echo ""
