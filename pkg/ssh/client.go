package ssh

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"golang.org/x/crypto/ssh"
)

// Client represents an SSH client for switch communication
type Client struct {
	config     *config.SSHConfig
	logger     *logger.Logger
	pool       *ConnectionPool
	authMethod ssh.AuthMethod
}

// ConnectionPool manages SSH connections
type ConnectionPool struct {
	connections chan *ssh.Client
	maxSize     int
	mutex       sync.Mutex
	active      map[*ssh.Client]bool
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(maxSize int) *ConnectionPool {
	return &ConnectionPool{
		connections: make(chan *ssh.Client, maxSize),
		maxSize:     maxSize,
		active:      make(map[*ssh.Client]bool),
	}
}

// Get retrieves a connection from the pool
func (p *ConnectionPool) Get() *ssh.Client {
	select {
	case conn := <-p.connections:
		p.mutex.Lock()
		p.active[conn] = true
		p.mutex.Unlock()
		return conn
	default:
		return nil
	}
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(conn *ssh.Client) {
	if conn == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.active[conn] {
		delete(p.active, conn)
		select {
		case p.connections <- conn:
			// Connection returned to pool
		default:
			// Pool is full, close the connection
			conn.Close()
		}
	}
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Close all pooled connections
	close(p.connections)
	for conn := range p.connections {
		conn.Close()
	}

	// Close all active connections
	for conn := range p.active {
		conn.Close()
	}
}

// New creates a new SSH client
func New(cfg *config.SSHConfig, logger *logger.Logger) (*Client, error) {
	client := &Client{
		config: cfg,
		logger: logger,
		pool:   NewConnectionPool(cfg.ConnectionPoolSize),
	}

	// Set up authentication
	if err := client.setupAuth(); err != nil {
		return nil, fmt.Errorf("failed to setup authentication: %w", err)
	}

	return client, nil
}

// setupAuth configures SSH authentication
func (c *Client) setupAuth() error {
	keyFile := c.config.KeyFile

	var keyData []byte
	var err error

	// Check if it's a 1Password reference (should be resolved by config loader)
	if strings.HasPrefix(keyFile, "op://") {
		c.logger.WithField("key_ref", keyFile).Debug("Loading SSH key from 1Password")

		// Use 1Password CLI to get the key
		cmd := exec.Command("op", "read", keyFile)
		keyData, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to read SSH key from 1Password: %w", err)
		}
	} else if strings.HasPrefix(keyFile, "-----BEGIN") {
		// Key file contains the actual key data (resolved from 1Password)
		c.logger.Debug("Using resolved SSH key data")
		keyData = []byte(keyFile)
	} else {
		c.logger.WithField("key_file", keyFile).Debug("Loading SSH key from file")

		// Read from file
		keyData, err = os.ReadFile(keyFile)
		if err != nil {
			return fmt.Errorf("failed to read SSH key file: %w", err)
		}
	}

	// Parse the private key
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return fmt.Errorf("failed to parse SSH private key: %w", err)
	}

	c.authMethod = ssh.PublicKeys(signer)
	c.logger.Debug("SSH authentication configured successfully")

	return nil
}

// Connect establishes an SSH connection
func (c *Client) Connect() (*ssh.Client, error) {
	// Try to get a connection from the pool first
	if conn := c.pool.Get(); conn != nil {
		// Test if connection is still alive
		if c.testConnection(conn) {
			return conn, nil
		}
		// Connection is dead, close it
		conn.Close()
	}

	// Create new connection
	config := &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            []ssh.AuthMethod{c.authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.Timeout,
	}

	addr := fmt.Sprintf("%s:%s", c.config.Host, c.config.Port)

	start := time.Now()
	conn, err := ssh.Dial("tcp", addr, config)
	duration := time.Since(start)

	c.logger.SSHConnection(c.config.Host, c.config.User, err == nil, duration)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	return conn, nil
}

// testConnection tests if an SSH connection is still alive
func (c *Client) testConnection(conn *ssh.Client) bool {
	session, err := conn.NewSession()
	if err != nil {
		return false
	}
	defer session.Close()

	// Try a simple command
	err = session.Run("echo test")
	return err == nil
}

// ReturnConnection returns a connection to the pool
func (c *Client) ReturnConnection(conn *ssh.Client) {
	c.pool.Put(conn)
}

// Close closes the SSH client and all connections
func (c *Client) Close() error {
	c.pool.Close()
	return nil
}

// ExecuteCommand executes a command on the switch
func (c *Client) ExecuteCommand(command string) (string, error) {
	conn, err := c.Connect()
	if err != nil {
		return "", err
	}
	defer c.ReturnConnection(conn)

	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	c.logger.WithFields(logger.Fields{
		"host":    c.config.Host,
		"command": command,
	}).Debug("Executing SSH command")

	output, err := session.CombinedOutput(command)
	if err != nil {
		return "", fmt.Errorf("failed to execute command: %w", err)
	}

	return string(output), nil
}

// FetchProtocols fetches NBAR protocols from the switch
func (c *Client) FetchProtocols() ([]string, error) {
	c.logger.WithComponent("ssh").WithField("operation", "fetch_protocols").Info("Starting protocol discovery")

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.Performance("fetch_protocols", duration, logger.Fields{
			"host": c.config.Host,
		})
	}()

	// Execute the NBAR protocol discovery command
	cmd := "terminal length 0 ; show ip nbar protocol-discovery"
	output, err := c.ExecuteCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch protocols: %w", err)
	}

	// Parse the output to extract protocol names
	protocols, err := c.parseProtocolOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse protocol output: %w", err)
	}

	c.logger.ProtocolDiscovery(c.config.Host, len(protocols), time.Since(start))

	return protocols, nil
}

// parseProtocolOutput parses the NBAR protocol discovery output
func (c *Client) parseProtocolOutput(output string) ([]string, error) {
	protocolSet := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(output))

	// Look for the command output section
	cmdOutputStart := strings.Index(output, "show ip nbar protocol-discovery")
	if cmdOutputStart >= 0 {
		// Find the end of the command line
		cmdLineEnd := strings.Index(output[cmdOutputStart:], "\n")
		if cmdLineEnd >= 0 {
			// Extract just the command output
			cmdOutput := output[cmdOutputStart+cmdLineEnd+1:]
			scanner = bufio.NewScanner(strings.NewReader(cmdOutput))
		}
	}

	inProtocolTable := false
	lineNum := 0
	protocolHeaderSeen := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check for interface headers (e.g., "TenGigabitEthernet1/0/1")
		if strings.Contains(line, "Ethernet") || strings.Contains(line, "GigabitEthernet") {
			// Reset for new interface section
			protocolHeaderSeen = false
			continue
		}

		// Detect the start of a protocol table by looking for the header
		if strings.Contains(line, "Protocol") && strings.Contains(line, "Packet Count") {
			protocolHeaderSeen = true
			continue
		}

		// Look for the dashed separator line that comes after the header
		if protocolHeaderSeen && strings.Contains(line, "-----") {
			inProtocolTable = true
			protocolHeaderSeen = false
			continue
		}

		// If we're in a protocol table and find a line with "Total", we've reached the end
		if inProtocolTable && strings.HasPrefix(strings.TrimSpace(line), "Total") {
			inProtocolTable = false
			continue
		}

		// Process protocol lines only when we're in a protocol table
		if inProtocolTable {
			protocol := c.extractProtocolName(line, lineNum)
			if protocol != "" {
				protocolSet[protocol] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Convert set to sorted slice
	protocols := make([]string, 0, len(protocolSet))
	for p := range protocolSet {
		protocols = append(protocols, p)
	}
	sort.Strings(protocols)

	return protocols, nil
}

// extractProtocolName extracts a protocol name from a line
func (c *Client) extractProtocolName(line string, lineNum int) string {
	// Skip empty lines
	if strings.TrimSpace(line) == "" {
		return ""
	}

	// Protocol names are at the beginning of the line, followed by spaces and numbers
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}

	protocolName := fields[0]

	// Skip lines that are part of the protocol data (they are indented and start with numbers)
	if len(line) > 0 && line[0] == ' ' && len(fields) > 0 && strings.Contains(fields[0], "0") {
		return ""
	}

	// Skip lines with dashes or other non-protocol content
	if strings.HasPrefix(protocolName, "-") {
		return ""
	}

	// Skip very short names (likely not protocols)
	if len(protocolName) < 3 {
		c.logger.WithField("protocol", protocolName).Debug("Skipping short protocol name")
		return ""
	}

	// Skip common words that aren't protocols
	commonWords := map[string]bool{
		"Line": true, "There": true, "Your": true, "The": true, "This": true,
		"From": true, "With": true, "That": true, "Have": true, "For": true,
		"And": true, "Not": true, "Are": true, "Last": true, "Port": true,
		"Total": true, "Protocol": true,
	}
	if commonWords[protocolName] {
		c.logger.WithField("protocol", protocolName).Debug("Skipping common word")
		return ""
	}

	// Additional check: ensure it looks like a valid protocol name
	// Most Cisco protocol names use lowercase with hyphens
	validProtocolPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*[a-z0-9]$`)
	if !validProtocolPattern.MatchString(protocolName) {
		c.logger.WithField("protocol", protocolName).Debug("Skipping invalid protocol format")
		return ""
	}

	c.logger.WithFields(logger.Fields{
		"protocol": protocolName,
		"line":     lineNum,
	}).Debug("Found valid protocol")

	return protocolName
}

// FetchRunningConfig fetches the running configuration from the switch
func (c *Client) FetchRunningConfig() (string, error) {
	c.logger.WithComponent("ssh").WithField("operation", "fetch_config").Info("Fetching running configuration")

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.Performance("fetch_config", duration, logger.Fields{
			"host": c.config.Host,
		})
	}()

	cmd := "show running-config"
	output, err := c.ExecuteCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to fetch running config: %w", err)
	}

	return output, nil
}

// PushConfig pushes configuration changes to the switch
func (c *Client) PushConfig(configCommands string) error {
	c.logger.WithComponent("ssh").WithField("operation", "push_config").Info("Pushing configuration to switch")

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.Performance("push_config", duration, logger.Fields{
			"host": c.config.Host,
		})
	}()

	conn, err := c.Connect()
	if err != nil {
		return err
	}
	defer c.ReturnConnection(conn)

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Execute the configuration commands in config mode
	configCmd := fmt.Sprintf("configure terminal\n%s\nend", configCommands)

	c.logger.WithFields(logger.Fields{
		"host":     c.config.Host,
		"commands": configCommands,
	}).Info("Executing configuration commands")

	output, err := session.CombinedOutput(configCmd)
	if err != nil {
		return fmt.Errorf("failed to push config: %w, output: %s", err, string(output))
	}

	c.logger.WithField("output", string(output)).Debug("Configuration push output")

	return nil
}

// SaveConfig saves the running configuration to startup-config
func (c *Client) SaveConfig() error {
	c.logger.WithComponent("ssh").WithField("operation", "save_config").Info("Saving configuration to startup-config")

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.Performance("save_config", duration, logger.Fields{
			"host": c.config.Host,
		})
	}()

	cmd := "write memory"
	output, err := c.ExecuteCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to save config: %w, output: %s", err, output)
	}

	c.logger.WithField("output", output).Debug("Save configuration output")

	return nil
}
