#!/bin/bash

# Script to run the NBAR QoS classification program with credentials
# Created: April 3, 2025
# Updated: To use 1Password for secure credential storage

# Set the 1Password references for API key and SSH credentials
API_KEY="op://Infrastructure/DeepSeek/NBAR-QOS-API-Key"
SWITCH_HOST="192.168.120.1"
SWITCH_USER="99940218"
# There are multiple SC01 items, so we need to use the specific item ID
SWITCH_KEY_FILE="op://Infrastructure/2xxtsdrmo7hrqasbefskas4zxq/private%20key"

# Check if 1Password CLI is installed
if ! command -v op &> /dev/null; then
  echo "Error: 1Password CLI is not installed. Please install it from https://1password.com/downloads/command-line/"
  exit 1
fi

# Check if user is signed in to 1Password CLI
if ! op account list &> /dev/null; then
  echo "Please sign in to 1Password CLI first using 'op signin'"
  exit 1
fi

# List available vaults to help with configuration
echo "Available 1Password vaults:"
op vault list
echo ""

# Verify item existence
echo "Checking if 1Password items exist..."
if op item get "DeepSeek" &> /dev/null; then
  echo "✅ DeepSeek item found"
else
  echo "❌ DeepSeek item not found. Please create it in your 1Password vault."
fi

if op item get 2xxtsdrmo7hrqasbefskas4zxq --vault Infrastructure &> /dev/null; then
  echo "✅ SC01 item found by ID"
else
  echo "❌ SC01 item not found by ID. Please check the item ID."
fi

# Check if the compiled binary exists or if the source code is newer
if [ ! -f "./nbar-auto-ai-classmaps" ] || [ nbar-auto-ai-classmaps.go -nt nbar-auto-ai-classmaps ]; then
  if [ ! -f "./nbar-auto-ai-classmaps" ]; then
    echo "Compiled binary not found. Compiling now..."
  else
    echo "Source code is newer than binary. Recompiling..."
  fi

  go build nbar-auto-ai-classmaps.go
  if [ $? -ne 0 ]; then
    echo "Error: Failed to compile the program. Please check for errors."
    exit 1
  fi
  echo "Compilation successful."
fi

# Run the compiled program with the credentials and pass all arguments
./nbar-auto-ai-classmaps \
  --api-key="$API_KEY" \
  --switch-host="$SWITCH_HOST" \
  --switch-user="$SWITCH_USER" \
  --switch-key-file="$SWITCH_KEY_FILE" \
  --use-1password \
  "$@"

# Print usage information if no arguments are provided
if [ $# -eq 0 ]; then
  echo ""
  echo "Usage examples:"
  echo "  ./run-nbar-qos.sh --fetch-from-switch --output=cisco"
  echo "  ./run-nbar-qos.sh --fetch-from-switch --dry-run"
  echo "  ./run-nbar-qos.sh --fetch-from-switch --push-config"
  echo "  ./run-nbar-qos.sh --fetch-from-switch --push-config --save-config"
  echo ""
  echo "Options:"
  echo "  --save-config    Save configuration to startup-config after pushing changes"
  echo ""
  echo "Note: This script uses 1Password CLI to securely fetch credentials."
  echo "Make sure you're signed in to 1Password CLI before running this script."
  echo ""
  echo "This script will use the compiled binary if available, or compile it if needed."
  echo "You can manually compile the program with: go build nbar-auto-ai-classmaps.go"
  echo ""
fi
