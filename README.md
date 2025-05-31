# NBAR QoS Classifier v2.0

A modern, enterprise-grade tool for automatically classifying network protocols into QoS classes using AI.

## Overview

The NBAR QoS Classifier is a sophisticated network automation tool that connects to Cisco network switches, fetches NBAR (Network-Based Application Recognition) protocol data, and intelligently classifies protocols into appropriate QoS (Quality of Service) classes. It combines predefined classifications, custom rules, and AI-powered classification for comprehensive protocol management.

## ✨ Features

### Core Functionality
- **Intelligent Protocol Classification**: Uses AI (DeepSeek R1) with fallback providers
- **Multi-Source Classification**: Predefined rules, custom patterns, and AI classification
- **Real-time Protocol Discovery**: Fetches live protocol data from Cisco switches via SSH
- **Configuration Generation**: Generates Cisco IOS configuration commands for QoS policies
- **Dry-run Mode**: Test configurations without making changes

### Enterprise Features
- **Secure Credential Management**: 1Password integration with fallback options
- **Comprehensive Caching**: Intelligent caching with TTL and compression
- **Metrics & Monitoring**: Prometheus metrics with Grafana dashboards
- **Structured Logging**: JSON logging with multiple output options
- **Web Interface**: Modern web UI for management and monitoring
- **Rate Limiting**: Configurable rate limiting for AI API calls
- **Circuit Breaker**: Fault tolerance for external service calls

## 🏗️ Architecture

The application follows a modular, microservices-inspired architecture:

```
├── cmd/nbar-classifier/     # Main application entry point
├── pkg/                     # Core packages
│   ├── ai/                 # AI provider implementations
│   ├── cache/              # Caching layer
│   ├── config/             # Configuration management
│   ├── metrics/            # Prometheus metrics
│   ├── qos/                # QoS classification logic
│   ├── ssh/                # SSH client for switch communication
│   └── web/                # Web interface (future)
├── internal/               # Internal packages
│   ├── logger/             # Structured logging
│   └── validator/          # Input validation
├── configs/                # Configuration files
├── deployments/            # Docker and Kubernetes manifests
└── test/                   # Test suites
```

## 📋 Prerequisites

### Runtime Requirements
- **Network Access**: SSH connectivity to Cisco switches
- **AI API Access**: DeepSeek API key (or other supported providers)
- **Optional**: 1Password CLI for secure credential management

### Development Requirements
- **Go 1.23+**: For building from source
- **Docker**: For containerized deployment
- **Kubernetes**: For orchestrated deployment (optional)

## 🚀 Quick Start

### Using Pre-built Binary

1. **Download the latest release**:
   ```bash
   curl -L https://github.com/varuntirumala1/nbar-qos-classifier/releases/latest/download/nbar-classifier-linux-amd64 -o nbar-classifier
   chmod +x nbar-classifier
   ```

2. **Create configuration**:
   ```bash
   cp configs/config.yaml my-config.yaml
   # Edit my-config.yaml with your settings
   ```

3. **Run the classifier**:
   ```bash
   ./nbar-classifier --config=my-config.yaml --fetch-from-switch --output=cisco
   ```

### Building from Source

1. **Clone and build**:
   ```bash
   git clone https://github.com/varuntirumala1/nbar-qos-classifier.git
   cd nbar-qos-classifier
   make build
   ```

2. **Run with development config**:
   ```bash
   make run-dev
   ```

### Using Docker

1. **Run with Docker Compose**:
   ```bash
   cd deployments/docker
   docker-compose up -d
   ```

2. **Access the application**:
   - Web Interface: http://localhost:8080
   - Metrics: http://localhost:9090
   - Grafana: http://localhost:3000

## ⚙️ Configuration

The application uses a YAML configuration file with comprehensive options:

### Basic Configuration

```yaml
# configs/config.yaml
app:
  name: "nbar-qos-classifier"
  version: "2.0.0"
  environment: "production"
  batch_size: 25
  timeout: "90s"
  max_retries: 3

ssh:
  host: "192.168.120.1"
  port: "22"
  user: "admin"
  key_file: "/path/to/ssh/key"  # or 1Password reference
  timeout: "10s"

ai:
  provider: "deepseek"
  api_key: "your-api-key"  # or 1Password reference
  model: "deepseek-reasoner"
  temperature: 0.1
  max_tokens: 1000
```

### Credential Management Options

#### Option 1: 1Password Integration (Recommended)
```yaml
security:
  use_1password: true

ssh:
  key_file: "op://Infrastructure/SSH-Key/private key"

ai:
  api_key: "op://Infrastructure/DeepSeek/API-Key"
```

#### Option 2: Environment Variables
```bash
export SSH_KEY_PATH="/secure/path/to/ssh/key"
export DEEPSEEK_API_KEY="your-api-key"
```

#### Option 3: Direct Configuration
```yaml
ssh:
  key_file: "/path/to/ssh/key"
ai:
  api_key: "your-api-key-here"
```

### Advanced Configuration

#### AI Provider Fallbacks
```yaml
ai:
  provider: "deepseek"
  fallback:
    - provider: "openai"
      enabled: true
    - provider: "claude"
      enabled: false

  providers:
    deepseek:
      api_key: "op://Vault/DeepSeek/key"
      model: "deepseek-reasoner"
    openai:
      api_key: "op://Vault/OpenAI/key"
      model: "gpt-4"
```

#### Custom QoS Rules
```yaml
qos:
  custom_rules:
    - name: "VoIP Protocols"
      pattern: ".*voice.*|.*voip.*|.*sip.*"
      class: "EF"
      priority: 1
      enabled: true

    - name: "Video Streaming"
      pattern: ".*video.*|.*stream.*"
      class: "AF41"
      priority: 2
      enabled: true
```

#### Caching Configuration
```yaml
cache:
  enabled: true
  ttl: "24h"
  max_size: 10000
  file_path: "protocol_cache.json"
  compression: true
  backup_path: "protocol_cache.backup.json"
```

## 📖 Usage

### Command Line Interface

The new CLI provides comprehensive options for protocol classification:

```bash
# Basic usage - fetch from switch and generate Cisco config
./nbar-classifier --config=config.yaml --fetch-from-switch --output=cisco

# Use input file instead of fetching from switch
./nbar-classifier --config=config.yaml --input-file=protocols.txt --output=text

# Dry run mode (test without making changes)
./nbar-classifier --config=config.yaml --fetch-from-switch --output=cisco --dry-run

# Push configuration to switch
./nbar-classifier --config=config.yaml --fetch-from-switch --output=cisco --push-config --save-config
```

### Command Line Options

| Option | Description | Example |
|--------|-------------|---------|
| `--config` | Path to configuration file | `--config=./configs/config.yaml` |
| `--fetch-from-switch` | Fetch protocols from switch via SSH | `--fetch-from-switch` |
| `--input-file` | Use existing protocol list file | `--input-file=protocols.txt` |
| `--output` | Output format (text/cisco) | `--output=cisco` |
| `--push-config` | Push config to switch | `--push-config` |
| `--dry-run` | Test without making changes | `--dry-run` |
| `--save-config` | Save to startup-config | `--save-config` |
| `--batch-size` | AI batch size override | `--batch-size=50` |
| `--log-level` | Log level override | `--log-level=debug` |
| `--enable-metrics` | Enable metrics server | `--enable-metrics` |
| `--enable-web` | Enable web interface | `--enable-web` |
| `--version` | Show version information | `--version` |

### Usage Examples

#### 1. Basic Protocol Classification
```bash
# Fetch protocols and classify them
./nbar-classifier \
  --config=configs/config.yaml \
  --fetch-from-switch \
  --output=cisco \
  --log-level=info
```

#### 2. Development Mode with Monitoring
```bash
# Run with web interface and metrics enabled
./nbar-classifier \
  --config=configs/config.yaml \
  --fetch-from-switch \
  --output=text \
  --enable-web \
  --enable-metrics \
  --log-level=debug
```

#### 3. Production Deployment
```bash
# Fetch, classify, and deploy to switch
./nbar-classifier \
  --config=configs/production.yaml \
  --fetch-from-switch \
  --output=cisco \
  --push-config \
  --save-config \
  --batch-size=25
```

#### 4. Testing and Validation
```bash
# Dry run to test configuration
./nbar-classifier \
  --config=configs/config.yaml \
  --fetch-from-switch \
  --output=cisco \
  --push-config \
  --dry-run \
  --log-level=debug
```

### Using Make Commands

For development and common tasks:

```bash
# Build and run with development settings
make run-dev

# Run tests
make test

# Build for production
make build

# Run with Docker
make docker-build docker-run

# Deploy to Kubernetes
make k8s-deploy
```

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