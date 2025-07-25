#!/bin/bash

# author: Chirag C. Shetty (cshetty2@illinois.edu)
# date: Jul 22, 2023 

# run as: . ./first_time_setup
# does the following: Install kops, kubectl, linkerd (L7 load balancer). Sets up ssh keys. Install and setup aws cli. Installs docker. (optinal) Downloads and builds DeathStarBench.


############################################################################################################################################
# This is to setup the main machine through which all cluster and experiemnts will be launched
############################################################################################################################################


dividerAction
sudo apt-get update

##################### Install kubectl ##################

curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl

###############################################################################################
# Setup key pair to be used in the cluster
# Source: https://stackoverflow.com/questions/43235179/how-to-execute-ssh-keygen-without-prompt

#dividerAction
#echo "Setup key pair to be used in the cluster: Source: https://stackoverflow.com/questions/43235179/how-to-execute-ssh-keygen-without-prompt"
#echo "n" | ssh-keygen -q -t rsa -N '' -f ~/.ssh/id_rsa


######### Docker Installation ##########
dividerAction
echo "Installing Docker:"
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

dividerAction
echo -e "Checking Docker:\n"

sudo docker run hello-world

dividerAttention
echo -e "Ensure docker installation succeeded. You should see a 'hello from docker' message above. \n"


