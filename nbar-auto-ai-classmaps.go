package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// QoSClass represents the classification of a protocol.
type QoSClass string

const (
	EF    QoSClass = "EF"    // Expedited Forwarding - Highest priority, real-time
	AF41  QoSClass = "AF41"  // Assured Forwarding - High priority, business-critical
	AF21  QoSClass = "AF21"  // Assured Forwarding - Medium priority
	CS1   QoSClass = "CS1"   // Class Selector - Low priority, best effort
	Other QoSClass = "OTHER" // Default/unclassified
)

// ProtocolClassification represents a protocol and its QoS classification.
type ProtocolClassification struct {
	Protocol string   `json:"protocol"`
	Class    QoSClass `json:"class"`
}

// Cache file for protocol classifications.
const cacheFilePath = "protocol_classifications_cache.json"

// Predefined classifications for well-known protocols.
var predefinedClassifications = map[string]QoSClass{
	"rtp":            EF,
	"rtp-audio":      EF,
	"rtp-video":      EF,
	"rtcp":           EF,
	"sip":            EF,
	"facetime":       EF,
	"wifi-calling":   EF,
	"web-rtc":        EF,
	"web-rtc-audio":  EF,
	"ms-teams":       EF,
	"ms-teams-media": EF,
	"zoom-meetings":  AF41,
	"skype":          AF41,
	"HNB":            AF41,
	"discord":        AF41,
	"vmware-vsphere": AF41,
	"youtube":        AF41,
	"netflix":        AF41,
	"smtp":           AF21,
	"secure-smtp":    AF21,
	"tftp":           AF21,
	"http":           AF21,
	"http-alt":       AF21,
	"https":          AF21,
	"quic":           AF21,
	"ssl":            AF21,
}

// DeepSeekResponse represents the structure of the API response.
type DeepSeekResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Index int `json:"index"`
	} `json:"choices"`
}

// Global variable to track if 1Password should be used
var use1Password bool

// fetchFromOnePassword retrieves a secret from 1Password using the CLI.
func fetchFromOnePassword(itemRef string) (string, error) {
	// If 1Password integration is not enabled, return the itemRef as-is
	if !use1Password {
		// If it looks like a 1Password reference but 1Password is not enabled, warn the user
		if strings.HasPrefix(itemRef, "op://") {
			fmt.Println("Warning: 1Password reference detected but --use-1password flag not set. Using reference as literal value.")
		}
		return itemRef, nil
	}

	// Print the reference we're trying to fetch for debugging
	fmt.Printf("Fetching from 1Password: %s\n", itemRef)

	// Parse the 1Password reference
	parts := strings.Split(itemRef, "/")
	if len(parts) < 4 {
		return "", fmt.Errorf("invalid 1Password reference format: %s (expected op://vault/item/field)", itemRef)
	}

	// Extract vault and item
	vault := parts[2]
	item := parts[3]
	// Field is extracted but only used in the standard op read path
	// We'll handle it differently for SSH keys

	// For SSH keys, we'll try a different approach using 'op item get' with JSON output
	if strings.Contains(itemRef, "private%20key") || strings.Contains(itemRef, "private key") {
		fmt.Println("Detected SSH key request, using alternative fetch method")

		// Get the entire item as JSON
		cmd := exec.Command("op", "item", "get", item, "--vault", vault, "--format=json")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()

		if err != nil {
			errMsg := stderr.String()
			fmt.Printf("1Password CLI error details: %s\n", errMsg)
			return "", fmt.Errorf("error fetching item from 1Password: %v", err)
		}

		// Parse the JSON to extract the private key field
		var itemData map[string]interface{}
		if err := json.Unmarshal(output, &itemData); err != nil {
			return "", fmt.Errorf("error parsing JSON response: %v", err)
		}

		// Try to find the private key field
		if fields, ok := itemData["fields"].([]interface{}); ok {
			for _, fieldData := range fields {
				if field, ok := fieldData.(map[string]interface{}); ok {
					label, _ := field["label"].(string)
					if strings.ToLower(label) == "private key" {
						if value, ok := field["value"].(string); ok {
							return value, nil
						}
					}
				}
			}
		}

		// If we couldn't find the field, print the available fields for debugging
		fmt.Println("Available fields in the item:")
		if fields, ok := itemData["fields"].([]interface{}); ok {
			for _, fieldData := range fields {
				if field, ok := fieldData.(map[string]interface{}); ok {
					label, _ := field["label"].(string)
					fmt.Printf("- %s\n", label)
				}
			}
		}

		return "", fmt.Errorf("private key field not found in item %s", item)
	}

	// For other types of secrets, use the standard 'op read' command
	cmd := exec.Command("op", "read", itemRef)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		errMsg := stderr.String()
		fmt.Printf("1Password CLI error details: %s\n", errMsg)

		// Try to get more information about what might be wrong
		fmt.Println("Checking 1Password CLI version:")
		versionCmd := exec.Command("op", "--version")
		versionOutput, _ := versionCmd.Output()
		fmt.Printf("1Password CLI version: %s\n", string(versionOutput))

		return "", fmt.Errorf("error fetching secret from 1Password (reference: %s): %v", itemRef, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// publicKeyAuthFunc loads the private key from file and returns an ssh.AuthMethod.
func publicKeyAuthFunc(keyPath string) (ssh.AuthMethod, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}
	return ssh.PublicKeys(signer), nil
}

// publicKeyAuthFuncFromContent parses the private key content and returns an ssh.AuthMethod.
func publicKeyAuthFuncFromContent(keyContent string) (ssh.AuthMethod, error) {
	signer, err := ssh.ParsePrivateKey([]byte(keyContent))
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}
	return ssh.PublicKeys(signer), nil
}

// fetchProtocolsFromSwitch connects via SSH to the switch and runs the command
// to retrieve the fresh list of discovered NBAR protocols.
func fetchProtocolsFromSwitch(host, user, keyFile, port string) ([]string, error) {
	var auth ssh.AuthMethod
	var err error

	// Check if keyFile is a 1Password reference
	if strings.HasPrefix(keyFile, "op://") {
		keyContent, err := fetchFromOnePassword(keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch SSH key from 1Password: %v", err)
		}
		auth, err = publicKeyAuthFuncFromContent(keyContent)
		if err != nil {
			return nil, err
		}
	} else {
		auth, err = publicKeyAuthFunc(keyFile)
		if err != nil {
			return nil, err
		}
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%s", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Use a simpler approach without pseudo-TTY
	fmt.Println("Running commands on switch...")

	// Execute both commands in a single command string
	// First disable pagination, then run the protocol discovery command
	cmd := "terminal length 0 ; sh ip nbar protocol-discovery"
	fmt.Println("Executing: " + cmd)

	// Run the command directly
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to run command: %v\nOutput: %s", err, string(output))
	}

	// Debug: Print the raw output for troubleshooting
	fmt.Println("\nRaw output from switch:")
	outputStr := string(output)
	fmt.Println(outputStr)

	// Look for the command output section
	cmdOutputStart := strings.Index(outputStr, "sh ip nbar protocol-discovery")
	if cmdOutputStart >= 0 {
		// Find the end of the command line
		cmdLineEnd := strings.Index(outputStr[cmdOutputStart:], "\n")
		if cmdLineEnd >= 0 {
			// Extract just the command output
			cmdOutput := outputStr[cmdOutputStart+cmdLineEnd+1:]
			fmt.Println("\nExtracted command output:")
			fmt.Println(cmdOutput)

			// Use the extracted output for parsing
			output = []byte(cmdOutput)
		}
	}

	// Extract protocol names by properly parsing the table structure
	protocolSet := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(output))

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
			// Skip empty lines
			if strings.TrimSpace(line) == "" {
				continue
			}

			// Protocol names are at the beginning of the line, followed by spaces and numbers
			fields := strings.Fields(line)
			if len(fields) > 0 {
				// The first field should be the protocol name
				protocolName := fields[0]

				// Skip lines that are part of the protocol data (they are indented and start with numbers)
				if len(line) > 0 && line[0] == ' ' && len(fields) > 0 && strings.Contains(fields[0], "0") {
					continue
				}

				// Skip lines with dashes or other non-protocol content
				if strings.HasPrefix(protocolName, "-") {
					continue
				}

				// Skip very short names (likely not protocols)
				if len(protocolName) < 3 {
					fmt.Printf("Skipping short name (not a protocol): %s\n", protocolName)
					continue
				}

				// Skip common words that aren't protocols
				commonWords := map[string]bool{
					"Line": true, "There": true, "Your": true, "The": true, "This": true,
					"From": true, "With": true, "That": true, "Have": true, "For": true,
					"And": true, "Not": true, "Are": true, "Last": true, "Port": true,
					"HNB": true, "Total": true, "Protocol": true,
				}
				if commonWords[protocolName] {
					fmt.Printf("Skipping common word (not a protocol): %s\n", protocolName)
					continue
				}

				// Additional check: ensure it looks like a valid protocol name
				// Most Cisco protocol names use lowercase with hyphens
				validProtocolPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*[a-z0-9]$`)
				if !validProtocolPattern.MatchString(protocolName) {
					fmt.Printf("Skipping invalid protocol name format: %s\n", protocolName)
					continue
				}

				// Valid protocol found
				fmt.Printf("Found protocol: %s (line %d)\n", protocolName, lineNum)
				protocolSet[protocolName] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	protocols := make([]string, 0, len(protocolSet))
	for p := range protocolSet {
		protocols = append(protocols, p)
	}
	sort.Strings(protocols)
	return protocols, nil
}

// fetchCurrentConfigFromSwitch connects via SSH and fetches the current running configuration.
func fetchCurrentConfigFromSwitch(host, user, keyFile, port string) (string, error) {
	var auth ssh.AuthMethod
	var err error

	// Check if keyFile is a 1Password reference
	if strings.HasPrefix(keyFile, "op://") {
		keyContent, err := fetchFromOnePassword(keyFile)
		if err != nil {
			return "", fmt.Errorf("failed to fetch SSH key from 1Password: %v", err)
		}
		auth, err = publicKeyAuthFuncFromContent(keyContent)
		if err != nil {
			return "", err
		}
	} else {
		auth, err = publicKeyAuthFunc(keyFile)
		if err != nil {
			return "", err
		}
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%s", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", fmt.Errorf("failed to dial switch: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Fetch the running configuration.
	cmd := "show running-config"
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to fetch running config: %v", err)
	}
	return string(output), nil
}

// parseQoSConfig parses the running configuration to extract QoS class-maps and protocol matches
func parseQoSConfig(runningConfig string) map[string][]string {
	// Map to store class-map names and their protocol matches
	classMapProtocols := make(map[string][]string)

	// Regular expressions to match class-map definitions and protocol matches
	classMapRegex := regexp.MustCompile(`(?m)^class-map match-any (QOS_[A-Z0-9_]+)`)
	protocolRegex := regexp.MustCompile(`(?m)^ match protocol ([a-zA-Z0-9_-]+)`)

	// Find all class-map definitions
	classMapMatches := classMapRegex.FindAllStringSubmatch(runningConfig, -1)
	for _, match := range classMapMatches {
		if len(match) >= 2 {
			classMapName := match[1]

			// Find the start of this class-map in the config
			classMapStart := strings.Index(runningConfig, "class-map match-any "+classMapName)
			if classMapStart >= 0 {
				// Find the end of this class-map (next line starting without a space)
				classMapEnd := classMapStart
				lines := strings.Split(runningConfig[classMapStart:], "\n")
				classMapContent := ""
				for i, line := range lines {
					if i > 0 && !strings.HasPrefix(line, " ") && line != "" {
						break
					}
					classMapEnd += len(line) + 1 // +1 for the newline
					classMapContent += line + "\n"
				}

				// Extract protocol matches from this class-map
				protocolMatches := protocolRegex.FindAllStringSubmatch(classMapContent, -1)
				protocols := []string{}
				for _, protocolMatch := range protocolMatches {
					if len(protocolMatch) >= 2 {
						protocols = append(protocols, protocolMatch[1])
					}
				}

				classMapProtocols[classMapName] = protocols
			}
		}
	}

	return classMapProtocols
}

// generateDiffCommands generates commands to add and remove protocols based on the diff
func generateDiffCommands(currentConfig map[string][]string, newConfig map[string][]string) string {
	var commands strings.Builder

	// Process each class-map
	for classMap, newProtocols := range newConfig {
		// Get current protocols for this class-map
		currentProtocols := currentConfig[classMap]
		if currentProtocols == nil {
			currentProtocols = []string{}
		}

		// Convert to maps for easier comparison
		currentProtoMap := make(map[string]bool)
		newProtoMap := make(map[string]bool)

		for _, p := range currentProtocols {
			currentProtoMap[p] = true
		}
		for _, p := range newProtocols {
			newProtoMap[p] = true
		}

		// Find protocols to remove (in current but not in new)
		for p := range currentProtoMap {
			if !newProtoMap[p] {
				commands.WriteString(fmt.Sprintf("class-map match-any %s\n no match protocol %s\n", classMap, p))
			}
		}

		// Find protocols to add (in new but not in current)
		for p := range newProtoMap {
			if !currentProtoMap[p] {
				commands.WriteString(fmt.Sprintf("class-map match-any %s\n match protocol %s\n", classMap, p))
			}
		}
	}

	return commands.String()
}

// testConfigDiff tests the configuration diff without pushing changes
func testConfigDiff(host, user, keyFile, port, newConfig string) error {
	// Fetch current configuration
	currentConfigStr, err := fetchCurrentConfigFromSwitch(host, user, keyFile, port)
	if err != nil {
		return fmt.Errorf("failed to fetch current config: %v", err)
	}

	// Parse the current configuration to extract QoS class-maps and their protocols
	currentClassMaps := parseQoSConfig(currentConfigStr)

	// Parse the new configuration to extract QoS class-maps and their protocols
	newClassMaps := parseQoSConfig(newConfig)

	// Generate diff commands (add/remove protocols)
	diffCommands := generateDiffCommands(currentClassMaps, newClassMaps)

	// If there are no changes, we're done
	if diffCommands == "" {
		fmt.Println("No changes needed, configuration is up to date")
		return nil
	}

	// Print the diff commands that would be pushed
	fmt.Println("DRY RUN - The following configuration changes would be made:")
	fmt.Println(diffCommands)

	// Print a summary of changes
	addCount, removeCount := countChanges(diffCommands)
	fmt.Printf("DRY RUN SUMMARY: Would add %d protocol matches and remove %d protocol matches\n",
		addCount, removeCount)

	return nil
}

// countChanges counts the number of add and remove operations in the diff commands
func countChanges(diffCommands string) (int, int) {
	addCount := 0
	removeCount := 0

	lines := strings.Split(diffCommands, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, " match protocol ") {
			addCount++
		} else if strings.HasPrefix(line, " no match protocol ") {
			removeCount++
		}
	}

	return addCount, removeCount
}

// saveConfigToStartup saves the running configuration to startup-config using an existing SSH client.
func saveConfigToStartup(client *ssh.Client) error {
	// Create a new session using the existing client
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Execute the write memory command
	output, err := session.CombinedOutput("write memory")
	if err != nil {
		return fmt.Errorf("failed to save config: %v, output: %s", err, string(output))
	}

	return nil
}

// pushConfigToSwitch connects via SSH and pushes the incremental configuration changes.
// Returns a boolean indicating if changes were made and the SSH client for reuse.
func pushConfigToSwitch(host, user, keyFile, port, newConfig string) (bool, *ssh.Client, error) {
	var auth ssh.AuthMethod
	var err error

	// Check if keyFile is a 1Password reference
	if strings.HasPrefix(keyFile, "op://") {
		keyContent, err := fetchFromOnePassword(keyFile)
		if err != nil {
			return false, nil, fmt.Errorf("failed to fetch SSH key from 1Password: %v", err)
		}
		auth, err = publicKeyAuthFuncFromContent(keyContent)
		if err != nil {
			return false, nil, err
		}
	} else {
		auth, err = publicKeyAuthFunc(keyFile)
		if err != nil {
			return false, nil, err
		}
	}
	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%s", host, port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return false, nil, fmt.Errorf("failed to dial switch: %v", err)
	}

	// Fetch current configuration
	currentConfigStr, err := fetchCurrentConfigFromSwitch(host, user, keyFile, port)
	if err != nil {
		client.Close()
		return false, nil, fmt.Errorf("failed to fetch current config: %v", err)
	}

	// Parse the current configuration to extract QoS class-maps and their protocols
	currentClassMaps := parseQoSConfig(currentConfigStr)

	// Parse the new configuration to extract QoS class-maps and their protocols
	newClassMaps := parseQoSConfig(newConfig)

	// Generate diff commands (add/remove protocols)
	diffCommands := generateDiffCommands(currentClassMaps, newClassMaps)

	// If there are no changes, we're done
	if diffCommands == "" {
		fmt.Println("No changes needed, configuration is up to date")
		return false, client, nil
	}

	// Push the diff commands to the switch
	fmt.Println("Pushing the following configuration changes:")
	fmt.Println(diffCommands)

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return false, nil, fmt.Errorf("failed to create SSH session: %v", err)
	}

	// Execute the diff commands in config mode
	configCmd := fmt.Sprintf("configure terminal\n%s\nend", diffCommands)
	output, err := session.CombinedOutput(configCmd)
	session.Close()
	if err != nil {
		client.Close()
		return false, nil, fmt.Errorf("failed to push config: %v, output: %s", err, string(output))
	}

	fmt.Println("Switch config push output:", string(output))
	return true, client, nil
}

// classifyProtocolsWithAI uses DeepSeek R1 to classify protocols concurrently.
func classifyProtocolsWithAI(protocols []string, apiKey string, batchSize int) (map[string]QoSClass, error) {
	results := make(map[string]QoSClass)
	// Start with predefined classifications.
	for p, c := range predefinedClassifications {
		results[p] = c
	}

	// Load cached classifications if available.
	cachedResults, err := loadClassificationCache()
	if err == nil {
		// Create a set of valid protocols for quick lookup
		validProtocols := make(map[string]bool)
		for _, p := range protocols {
			validProtocols[p] = true
		}

		// Only use cached classifications for protocols that are in our current list
		validCachedCount := 0
		for p, c := range cachedResults {
			if validProtocols[p] {
				if _, exists := results[p]; !exists {
					results[p] = c
					validCachedCount++
				}
			}
		}
		fmt.Println("Loaded cached classifications for", validCachedCount, "protocols")
	}

	// Determine protocols that need classification.
	needClassification := []string{}
	for _, p := range protocols {
		if _, exists := results[p]; !exists {
			needClassification = append(needClassification, p)
		}
	}

	// Process batches concurrently with limited concurrency.
	var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	sem := make(chan struct{}, 3) // limit to 3 concurrent API calls
	var overallErr error

	for i := 0; i < len(needClassification); i += batchSize {
		end := i + batchSize
		if end > len(needClassification) {
			end = len(needClassification)
		}
		batchProtocols := needClassification[i:end]

		wg.Add(1)
		sem <- struct{}{}
		go func(batch []string) {
			defer wg.Done()
			// Format protocols as a numbered list.
			protocolsText := ""
			for j, protocol := range batch {
				protocolsText += fmt.Sprintf("%d. %s\n", j+1, protocol)
			}
			prompt := fmt.Sprintf(`Classify these network protocols into QoS classes for a Cisco 9300 switch:

%s

QoS Classes:
- EF: Real-time (voice/video calls)
- AF41: Business-critical (interactive apps)
- AF21: Important (email, transfers)
- CS1: Background (updates, browsing)

Respond ONLY with JSON array:
[{"protocol":"name","class":"CLASS"}]`, protocolsText)

			resultText, err := callDeepSeekAPI(prompt, apiKey)
			if err != nil {
				overallErr = err
				<-sem
				return
			}
			var classifications []ProtocolClassification
			err = json.Unmarshal([]byte(resultText), &classifications)
			if err != nil {
				// Fallback: try regex extraction.
				re := regexp.MustCompile(`\$begin:math:display\$(\s*\{.*?\}\s*,?)+\$end:math:display\$`)
				matches := re.FindString(resultText)
				if matches != "" {
					err = json.Unmarshal([]byte(matches), &classifications)
					if err != nil {
						fmt.Println("Error parsing classification batch:", err)
						<-sem
						return
					}
				} else {
					// Manual extraction fallback.
					lines := strings.Split(resultText, "\n")
					for _, line := range lines {
						for _, protocol := range batch {
							if strings.Contains(line, protocol) {
								var class QoSClass = CS1
								if strings.Contains(strings.ToUpper(line), "EF") {
									class = EF
								} else if strings.Contains(strings.ToUpper(line), "AF41") {
									class = AF41
								} else if strings.Contains(strings.ToUpper(line), "AF21") {
									class = AF21
								}
								classifications = append(classifications, ProtocolClassification{
									Protocol: protocol,
									Class:    class,
								})
								break
							}
						}
					}
				}
			}
			mutex.Lock()
			for _, c := range classifications {
				results[c.Protocol] = c.Class
			}
			mutex.Unlock()
			<-sem
		}(batchProtocols)
	}
	wg.Wait()
	if overallErr != nil {
		return nil, overallErr
	}

	// Default any unclassified protocols to CS1.
	for _, p := range protocols {
		if _, exists := results[p]; !exists {
			results[p] = CS1
		}
	}
	saveClassificationCache(results)
	return results, nil
}

// loadClassificationCache loads cached protocol classifications from file.
func loadClassificationCache() (map[string]QoSClass, error) {
	file, err := os.Open(cacheFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cache map[string]string
	if err := json.NewDecoder(file).Decode(&cache); err != nil {
		return nil, err
	}
	result := make(map[string]QoSClass)
	for k, v := range cache {
		result[k] = QoSClass(v)
	}
	return result, nil
}

// saveClassificationCache saves protocol classifications to cache file.
func saveClassificationCache(classifications map[string]QoSClass) {
	cache := make(map[string]string)
	for k, v := range classifications {
		cache[k] = string(v)
	}
	file, err := os.Create(cacheFilePath)
	if err != nil {
		fmt.Printf("Warning: couldn't create cache file: %v\n", err)
		return
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(cache); err != nil {
		fmt.Printf("Warning: couldn't write to cache file: %v\n", err)
	}
}

// callDeepSeekAPI sends the prompt to DeepSeek R1 and returns the result.
func callDeepSeekAPI(prompt string, apiKey string) (string, error) {
	// Check if apiKey is a 1Password reference
	if strings.HasPrefix(apiKey, "op://") {
		var err error
		apiKey, err = fetchFromOnePassword(apiKey)
		if err != nil {
			return "", fmt.Errorf("failed to fetch API key from 1Password: %v", err)
		}
	}
	client := &http.Client{Timeout: 90 * time.Second}
	maxRetries := 3
	baseDelay := 2 * time.Second

	payload := map[string]interface{}{
		"model": "deepseek-reasoner",
		"messages": []map[string]string{
			{"role": "system", "content": "You are an expert in network protocols and QoS classification."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
		"max_tokens":  1000,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	var lastError error
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("POST", "https://api.deepseek.com/v1/chat/completions", bytes.NewBuffer(jsonPayload))
		if err != nil {
			lastError = fmt.Errorf("error creating request: %v", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			lastError = fmt.Errorf("request error: %v", err)
			if i < maxRetries-1 {
				time.Sleep(baseDelay * time.Duration(math.Pow(2, float64(i))))
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastError = fmt.Errorf("error reading response: %v", err)
			if i < maxRetries-1 {
				time.Sleep(baseDelay * time.Duration(math.Pow(2, float64(i))))
			}
			continue
		}

		if resp.StatusCode == 200 {
			var result struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				lastError = fmt.Errorf("error parsing JSON response: %v", err)
				continue
			}
			if len(result.Choices) > 0 {
				return result.Choices[0].Message.Content, nil
			}
			lastError = fmt.Errorf("empty choices in response")
			continue
		}

		var errorResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error.Message != "" {
			lastError = fmt.Errorf("API error: %s (type: %s, code: %s)", errorResp.Error.Message, errorResp.Error.Type, errorResp.Error.Code)
		} else {
			lastError = fmt.Errorf("API error: status code %d: %s", resp.StatusCode, string(body))
		}
		if i < maxRetries-1 {
			time.Sleep(baseDelay * time.Duration(math.Pow(2, float64(i))))
		}
	}
	return "", lastError
}

func main() {
	// Define flags.
	apiKeyPtr := flag.String("api-key", "", "API key for DeepSeek R1 or 1Password reference (op://vault/item/field)")
	batchSizePtr := flag.Int("batch-size", 25, "Number of protocols to analyze in each API batch")
	outputTypePtr := flag.String("output", "text", "Output format: 'text' or 'cisco'")
	fetchFromSwitchPtr := flag.Bool("fetch-from-switch", false, "Fetch protocol list from switch via SSH")
	inputFilePtr := flag.String("input-file", "", "Input file containing NBAR protocol list (if not fetching from switch)")

	// SSH flags for fetching protocols and/or pushing config.
	switchHostPtr := flag.String("switch-host", "", "Switch hostname or IP")
	switchUserPtr := flag.String("switch-user", "", "SSH username for switch")
	switchKeyFilePtr := flag.String("switch-key-file", "", "Path to SSH private key for switch or 1Password reference (op://vault/item/field)")
	switchPortPtr := flag.String("switch-port", "22", "SSH port for switch")
	pushConfigPtr := flag.Bool("push-config", false, "Push updated config to switch via SSH")
	dryRunPtr := flag.Bool("dry-run", false, "Test implementation without pushing config")
	saveConfigPtr := flag.Bool("save-config", false, "Save configuration to startup-config after pushing changes")

	// 1Password integration flags
	use1PasswordPtr := flag.Bool("use-1password", false, "Use 1Password CLI to fetch secrets")

	flag.Parse()

	// Set the global variable for 1Password integration
	use1Password = *use1PasswordPtr

	// Check if 1Password CLI is available when use-1password flag is set
	if use1Password {
		// Check if 1Password CLI is installed
		_, err := exec.LookPath("op")
		if err != nil {
			fmt.Println("Error: 1Password CLI is not installed. Please install it from https://1password.com/downloads/command-line/")
			os.Exit(1)
		}

		// Check if user is signed in to 1Password CLI
		cmd := exec.Command("op", "account", "list")
		if err := cmd.Run(); err != nil {
			fmt.Println("Error: Not signed in to 1Password CLI. Please sign in using 'op signin'")
			os.Exit(1)
		}

		fmt.Println("1Password CLI is available and ready to use")
	}

	if *apiKeyPtr == "" {
		fmt.Println("Error: DeepSeek R1 API key is required")
		flag.Usage()
		os.Exit(1)
	}

	var protocols []string
	var err error

	if *fetchFromSwitchPtr {
		if *switchHostPtr == "" || *switchUserPtr == "" || *switchKeyFilePtr == "" {
			fmt.Println("Error: switch-host, switch-user, and switch-key-file are required when fetching from switch")
			os.Exit(1)
		}
		protocols, err = fetchProtocolsFromSwitch(*switchHostPtr, *switchUserPtr, *switchKeyFilePtr, *switchPortPtr)
		if err != nil {
			fmt.Printf("Error fetching protocols from switch: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Fetched %d protocols from switch\n", len(protocols))
	} else {
		if *inputFilePtr == "" {
			fmt.Println("Error: input-file must be specified if not fetching from switch")
			os.Exit(1)
		}
		file, err := os.Open(*inputFilePtr)
		if err != nil {
			fmt.Printf("Error opening input file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		protocolSet := make(map[string]bool)
		scanner := bufio.NewScanner(file)
		protocolRegex := regexp.MustCompile(`^\s*([a-zA-Z0-9_-]+)\s+`)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Protocol") || strings.Contains(line, "Last clearing") ||
				strings.Contains(line, "TenGigabitEthernet") || strings.Contains(line, "Total") ||
				strings.TrimSpace(line) == "" {
				continue
			}
			matches := protocolRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				protocolSet[matches[1]] = true
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("Error reading input file: %v\n", err)
			os.Exit(1)
		}
		for p := range protocolSet {
			protocols = append(protocols, p)
		}
		sort.Strings(protocols)
	}

	fmt.Println("Using DeepSeek R1 to classify protocols into QoS classes...")
	classifications, err := classifyProtocolsWithAI(protocols, *apiKeyPtr, *batchSizePtr)
	if err != nil {
		fmt.Printf("Error classifying protocols: %v\n", err)
		os.Exit(1)
	}

	// Group protocols by QoS class.
	classByProtocol := make(map[QoSClass][]string)
	for protocol, class := range classifications {
		classByProtocol[class] = append(classByProtocol[class], protocol)
	}
	for class := range classByProtocol {
		sort.Strings(classByProtocol[class])
	}

	// Generate output configuration.
	var outputBuffer bytes.Buffer
	if *outputTypePtr == "cisco" {
		// Helper to write class-maps with a max of 16 protocols each.
		writeClassMaps := func(className, description string, protocols []string) []string {
			numClassMaps := (len(protocols) + 15) / 16
			classMapNames := make([]string, 0, numClassMaps)
			for i := 0; i < numClassMaps; i++ {
				classMapName := className
				if numClassMaps > 1 {
					classMapName = fmt.Sprintf("%s_%d", className, i+1)
				}
				classMapNames = append(classMapNames, classMapName)
				outputBuffer.WriteString(fmt.Sprintf("class-map match-any %s\n", classMapName))
				outputBuffer.WriteString(fmt.Sprintf(" description %s\n", description))
				start := i * 16
				end := start + 16
				if end > len(protocols) {
					end = len(protocols)
				}
				for j := start; j < end; j++ {
					outputBuffer.WriteString(fmt.Sprintf(" match protocol %s\n", protocols[j]))
				}
				outputBuffer.WriteString("!\n")
			}
			return classMapNames
		}

		efClassMaps := writeClassMaps("QOS_EF", "Expedited Forwarding - Real-time traffic (Voice, Video)", classByProtocol[EF])
		af41ClassMaps := writeClassMaps("QOS_AF41", "Assured Forwarding 41 - Business-critical applications", classByProtocol[AF41])
		af21ClassMaps := writeClassMaps("QOS_AF21", "Assured Forwarding 21 - Important data applications", classByProtocol[AF21])
		_ = writeClassMaps("QOS_CS1", "Class Selector 1 - Background traffic", classByProtocol[CS1])

		// DSCP class-maps for egress policy have been removed as requested by the user
		// since they are related to egress and already exist in the configuration

		outputBuffer.WriteString("! Ingress marking policy-map\n")
		outputBuffer.WriteString("policy-map PM_MARK_AVC_WIRED_INGRESS\n description Marks incoming traffic based on App (AVC) or VLAN fallback. CS1 traffic handled by class-default.\n")
		for _, cm := range efClassMaps {
			outputBuffer.WriteString(fmt.Sprintf(" class %s\n  set dscp ef\n", cm))
		}
		for _, cm := range af41ClassMaps {
			outputBuffer.WriteString(fmt.Sprintf(" class %s\n  set dscp af41\n", cm))
		}
		for _, cm := range af21ClassMaps {
			outputBuffer.WriteString(fmt.Sprintf(" class %s\n  set dscp af21\n", cm))
		}
		outputBuffer.WriteString(" class class-default\n  set dscp cs1\n!\n")

		// Note: Egress queuing policy-map and interface application sections have been removed
		// as requested by the user. Only class-maps for marking and policy-map for marking on ingress
		// are included in the output.
	} else {
		currentDate := time.Now().Format("2006-01-02")
		outputBuffer.WriteString("# NBAR Protocols Classified by QoS\n")
		outputBuffer.WriteString(fmt.Sprintf("# Generated on %s using DeepSeek R1 AI\n", currentDate))
		outputBuffer.WriteString("# For use with Cisco 9300 Switch and Ubiquiti UXG Fiber\n\n")
		outputBuffer.WriteString("## EF (Expedited Forwarding) - Real-time traffic\n")
		for i, protocol := range classByProtocol[EF] {
			outputBuffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, protocol))
		}
		outputBuffer.WriteString("\n## AF41 (Assured Forwarding) - Business-critical applications\n")
		for i, protocol := range classByProtocol[AF41] {
			outputBuffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, protocol))
		}
		outputBuffer.WriteString("\n## AF21 (Assured Forwarding) - Important data applications\n")
		for i, protocol := range classByProtocol[AF21] {
			outputBuffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, protocol))
		}
		outputBuffer.WriteString("\n## CS1 (Class Selector) - Background traffic\n")
		for i, protocol := range classByProtocol[CS1] {
			outputBuffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, protocol))
		}
	}
	configOutput := outputBuffer.String()
	outputFile := "nbar-protocols-qos.txt"
	if err := os.WriteFile(outputFile, []byte(configOutput), 0644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully classified %d protocols into QoS classes:\n", len(protocols))
	fmt.Printf("- EF:   %d protocols\n", len(classByProtocol[EF]))
	fmt.Printf("- AF41: %d protocols\n", len(classByProtocol[AF41]))
	fmt.Printf("- AF21: %d protocols\n", len(classByProtocol[AF21]))
	fmt.Printf("- CS1:  %d protocols\n", len(classByProtocol[CS1]))
	fmt.Printf("Output written to %s\n", outputFile)

	if *pushConfigPtr || *dryRunPtr {
		if *switchHostPtr == "" || *switchUserPtr == "" || *switchKeyFilePtr == "" {
			fmt.Println("Error: switch-host, switch-user, and switch-key-file are required when pushing config or running in dry-run mode")
			os.Exit(1)
		}

		if *dryRunPtr {
			fmt.Println("Running in dry-run mode - testing implementation without pushing config...")
			if err := testConfigDiff(*switchHostPtr, *switchUserPtr, *switchKeyFilePtr, *switchPortPtr, configOutput); err != nil {
				fmt.Printf("Error during dry run: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Dry run completed successfully.")
		} else {
			fmt.Println("Pushing incremental configuration changes to switch...")
			changesMade, sshClient, err := pushConfigToSwitch(*switchHostPtr, *switchUserPtr, *switchKeyFilePtr, *switchPortPtr, configOutput)
			if err != nil {
				fmt.Printf("Error pushing config to switch: %v\n", err)
				os.Exit(1)
			}
			defer sshClient.Close()
			fmt.Println("Configuration successfully pushed to switch.")

			// Save configuration if requested AND changes were made
			if *saveConfigPtr && changesMade {
				fmt.Println("Changes were made - saving configuration to startup-config...")
				if err := saveConfigToStartup(sshClient); err != nil {
					fmt.Printf("Error saving configuration: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("Configuration saved successfully.")
			} else if *saveConfigPtr {
				fmt.Println("No changes were made - skipping configuration save")
			}
		}
	}
}