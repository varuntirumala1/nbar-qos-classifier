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

Before running the tool, you need to set up your credentials in 1Password:

1. Store your DeepSeek API key in 1Password
2. Store your SSH private key in 1Password
3. Update the references in `run-nbar-qos.sh` to match your 1Password structure:
   ```bash
   API_KEY="op://YourVault/DeepSeek/API Key"
   SWITCH_KEY_FILE="op://YourVault/YourSSHKey/private key"
   ```

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

This tool uses 1Password to securely fetch sensitive credentials:
- DeepSeek API key
- SSH private key

No credentials are stored in plain text.

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