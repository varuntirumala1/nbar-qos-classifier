# NBAR QoS Classifier

A tool for automatically classifying network protocols into QoS classes using AI.

## Overview

This tool connects to Cisco network switches, fetches the list of NBAR (Network-Based Application Recognition) protocols, and classifies them into appropriate QoS (Quality of Service) classes. It uses a combination of predefined classifications and AI-based classification for unknown protocols.

## Features

- Fetches protocol lists from Cisco switches via SSH
- Classifies protocols into QoS classes (EF, AF41, AF21, CS1)
- Uses DeepSeek R1 AI to classify unknown protocols
- Generates Cisco configuration commands for QoS policy
- Securely fetches credentials from 1Password
- Supports dry-run mode for testing without making changes

## Prerequisites

- Go 1.18 or higher (for compilation only)
- 1Password CLI (`op`) installed and configured
- SSH access to the network switch
- DeepSeek API key

## Installation

1. Clone this repository:
   ```
   git clone https://github.com/varuntirumala1/nbar-qos-classifier.git
   cd nbar-qos-classifier
   ```

2. Compile the program:
   ```
   go build nbar-auto-ai-classmaps.go
   ```

3. Make the script executable:
   ```
   chmod +x run-nbar-qos.sh
   ```

## Configuration

This tool supports multiple methods for handling credentials:

### Method 1: Using 1Password (Recommended for Security)

1. Store your DeepSeek API key in 1Password
2. Store your SSH private key in 1Password
3. Update the references in `run-nbar-qos.sh` to match your 1Password structure:
   ```bash
   API_KEY="op://YourVault/DeepSeek/API Key"
   SWITCH_KEY_FILE="op://YourVault/YourSSHKey/private key"
   ```
4. Make sure to use the `--use-1password` flag when running the tool

### Method 2: Using Environment Variables

1. Set environment variables for your credentials:
   ```bash
   export DEEPSEEK_API_KEY="your-api-key-here"
   export SSH_KEY_PATH="/path/to/your/ssh/key"
   ```

2. Update the `run-nbar-qos.sh` script to use these environment variables:
   ```bash
   API_KEY="$DEEPSEEK_API_KEY"
   SWITCH_KEY_FILE="$SSH_KEY_PATH"
   ```

### Method 3: Direct File Paths

1. Store your SSH key in a secure location on your filesystem
2. Update the `run-nbar-qos.sh` script with direct paths:
   ```bash
   API_KEY="your-api-key-here"
   SWITCH_KEY_FILE="/path/to/your/ssh/key"
   ```

**Note**: Methods 2 and 3 are less secure as they involve storing credentials in plain text. Use Method 1 (1Password) for production environments.

## Usage

Run the tool using the provided shell script:

```bash
./run-nbar-qos.sh --fetch-from-switch --output=cisco
```

### Common Options

- `--fetch-from-switch`: Fetch protocol list from the switch
- `--output=cisco`: Output in Cisco configuration format
- `--dry-run`: Test without making changes
- `--push-config`: Push configuration to the switch
- `--save-config`: Save configuration to startup-config

## Security

### Credential Handling

This tool offers multiple methods for handling credentials, with varying security levels:

1. **1Password Integration (Most Secure)**:
   - DeepSeek API key and SSH private key are stored in 1Password
   - Credentials are fetched securely at runtime
   - No credentials are stored in plain text

2. **Environment Variables (Moderate Security)**:
   - Credentials are stored in environment variables
   - Not persisted in script files, but visible in process listings
   - Suitable for development environments

3. **Direct File Paths (Basic Security)**:
   - API key stored directly in script
   - SSH key stored as a file on disk
   - Simplest approach but least secure
   - Suitable only for testing environments

### Best Practices

- Use 1Password integration for production environments
- Ensure SSH keys have appropriate permissions (chmod 600)
- Never commit credentials to version control
- Consider using a dedicated service account for switch access

## How It Works

1. **Fetching Protocols**: The tool connects to the switch via SSH and runs the `show ip nbar protocol-discovery` command to get the list of protocols.

2. **Classification**: Each protocol is classified into one of four QoS classes:
   - **EF (Expedited Forwarding)**: Real-time applications like VoIP, video conferencing
   - **AF41 (Assured Forwarding 41)**: Interactive applications like web conferencing, remote desktop
   - **AF21 (Assured Forwarding 21)**: Business applications like email, file transfers
   - **CS1 (Class Selector 1)**: Background traffic like updates, backups

3. **AI Classification**: For unknown protocols, the tool uses DeepSeek R1 AI to analyze the protocol name and determine the most appropriate QoS class.

4. **Configuration Generation**: Based on the classifications, the tool generates Cisco IOS configuration commands for QoS policy.

5. **Configuration Deployment**: Optionally, the tool can push the configuration to the switch and save it to startup-config.

## License

[MIT License](LICENSE)