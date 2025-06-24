#!/bin/bash

############################################################
echo "==> setting up environment"
############################################################

############################################################
# Settings
############################################################

# dry run
# set -n

############################################################
# Paths and directories
############################################################

export SCRIPTS_DIR=.
export BASE_DIR=../..
export PKG_DIR=$BASE_DIR/pkg
export CMD_DIR=$BASE_DIR/cmd
export SAMPLES_DIR=$BASE_DIR/sample-data
export SERVICES_DIR=$BASE_DIR/services
export YAMLS_DIR=$SERVICES_DIR/yamls

# static data
export INFERNO_DATA_PATH=$SAMPLES_DIR/large/

############################################################
# Parameters
############################################################

. ./setparms.sh

############################################################
# Routines
############################################################

break_continue_message=""

continue_prompt() {
  while true
  do
    if [[ "$break_continue_message" != "" ]]
    then
      sleep 0
      echo -n "${break_continue_message} (y/n): "
      read ans
      if [[ "$ans" = "y" ]]
      then
        break
      fi
    fi

    echo -n "Continue waiting? (y/n): "
    read ans
    if [[ "$ans" = "n" ]]
    then
      break
    fi
  done
}

