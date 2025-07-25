#!/bin/bash

# Path to the client script
CLIENT_SCRIPT="client.py"

# Poisson process parameters
rpm=40 # requests per minute
if [ -n "$1" ]; then
  rpm=$1
fi
LAMBDA_RATE=$rpm  # Requests per minute (mean arrival rate)
MEAN_INTERVAL=$((60 / LAMBDA_RATE))  # Average interarrival time in seconds

# Check if the client script exists
if [[ ! -f "$CLIENT_SCRIPT" ]]; then
  echo "Error: $CLIENT_SCRIPT not found."
  exit 1
fi

# Function to generate a random interarrival time using exponential distribution
generate_interarrival_time() {
  awk -v mean="$MEAN_INTERVAL" 'BEGIN{srand(); print -mean * log(1 - rand())}'
}

# Main loop to launch client instances
echo "Starting Poisson-distributed client launcher at $rpm RPM ..."
while true; do
  # Launch the client script in the background
  # echo "Launching a new client instance..."
  python3 "$CLIENT_SCRIPT" &

  # Generate the next interarrival time
  INTERVAL=$(generate_interarrival_time)
  # echo "Next client will be launched in $INTERVAL seconds."

  # Sleep for the calculated interval
  sleep "$INTERVAL"
done
