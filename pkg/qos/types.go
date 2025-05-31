package qos

import (
	"fmt"
	"regexp"
	"strings"
)

// Class represents a QoS classification
type Class string

// Standard QoS classes
const (
	EF    Class = "EF"    // Expedited Forwarding - Highest priority, real-time
	AF41  Class = "AF41"  // Assured Forwarding 41 - High priority, business-critical
	AF21  Class = "AF21"  // Assured Forwarding 21 - Medium priority
	CS1   Class = "CS1"   // Class Selector 1 - Low priority, best effort
	Other Class = "OTHER" // Default/unclassified
)

// AllClasses returns all available QoS classes
func AllClasses() []Class {
	return []Class{EF, AF41, AF21, CS1, Other}
}

// String returns the string representation of the QoS class
func (c Class) String() string {
	return string(c)
}

// IsValid checks if the QoS class is valid
func (c Class) IsValid() bool {
	for _, class := range AllClasses() {
		if c == class {
			return true
		}
	}
	return false
}

// Priority returns the priority level of the QoS class (lower number = higher priority)
func (c Class) Priority() int {
	switch c {
	case EF:
		return 1
	case AF41:
		return 2
	case AF21:
		return 3
	case CS1:
		return 4
	case Other:
		return 5
	default:
		return 6
	}
}

// DSCP returns the DSCP marking for the QoS class
func (c Class) DSCP() string {
	switch c {
	case EF:
		return "ef"
	case AF41:
		return "af41"
	case AF21:
		return "af21"
	case CS1:
		return "cs1"
	default:
		return "cs0"
	}
}

// Description returns a human-readable description of the QoS class
func (c Class) Description() string {
	switch c {
	case EF:
		return "Expedited Forwarding - Real-time traffic (Voice, Video calls)"
	case AF41:
		return "Assured Forwarding 41 - Business-critical applications"
	case AF21:
		return "Assured Forwarding 21 - Important data applications"
	case CS1:
		return "Class Selector 1 - Background traffic"
	case Other:
		return "Unclassified traffic"
	default:
		return "Unknown QoS class"
	}
}

// Classification represents a protocol and its QoS classification
type Classification struct {
	Protocol   string  `json:"protocol"`
	Class      Class   `json:"class"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"` // predefined, ai, custom_rule, cache
	Timestamp  int64   `json:"timestamp,omitempty"`
}

// Rule represents a custom classification rule
type Rule struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Class       Class  `json:"class"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	regex       *regexp.Regexp
}

// NewRule creates a new classification rule
func NewRule(name, pattern string, class Class, priority int) (*Rule, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return &Rule{
		Name:     name,
		Pattern:  pattern,
		Class:    class,
		Priority: priority,
		Enabled:  true,
		regex:    regex,
	}, nil
}

// Match checks if the rule matches the given protocol
func (r *Rule) Match(protocol string) bool {
	if !r.Enabled || r.regex == nil {
		return false
	}
	return r.regex.MatchString(strings.ToLower(protocol))
}

// Validate validates the rule
func (r *Rule) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("rule name cannot be empty")
	}
	if r.Pattern == "" {
		return fmt.Errorf("rule pattern cannot be empty")
	}
	if !r.Class.IsValid() {
		return fmt.Errorf("invalid QoS class: %s", r.Class)
	}
	if r.Priority < 1 {
		return fmt.Errorf("rule priority must be positive")
	}

	// Test regex compilation
	_, err := regexp.Compile(r.Pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	return nil
}

// CompileRegex compiles the regex pattern for the rule
func (r *Rule) CompileRegex() error {
	regex, err := regexp.Compile(r.Pattern)
	if err != nil {
		return fmt.Errorf("failed to compile regex: %w", err)
	}
	r.regex = regex
	return nil
}

// ClassConfig represents configuration for a QoS class
type ClassConfig struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	DSCP        string   `yaml:"dscp" json:"dscp"`
	Priority    int      `yaml:"priority" json:"priority"`
	Protocols   []string `yaml:"protocols" json:"protocols"`
}

// Classifier handles protocol classification logic
type Classifier struct {
	predefinedClassifications map[string]Class
	customRules               []*Rule
	defaultClass              Class
	confidenceThreshold       float64
}

// NewClassifier creates a new QoS classifier
func NewClassifier(defaultClass Class, confidenceThreshold float64) *Classifier {
	return &Classifier{
		predefinedClassifications: make(map[string]Class),
		customRules:               make([]*Rule, 0),
		defaultClass:              defaultClass,
		confidenceThreshold:       confidenceThreshold,
	}
}

// AddPredefinedClassification adds a predefined classification
func (c *Classifier) AddPredefinedClassification(protocol string, class Class) {
	c.predefinedClassifications[strings.ToLower(protocol)] = class
}

// AddCustomRule adds a custom classification rule
func (c *Classifier) AddCustomRule(rule *Rule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	if err := rule.CompileRegex(); err != nil {
		return err
	}
	c.customRules = append(c.customRules, rule)
	return nil
}

// ClassifyProtocol classifies a single protocol
func (c *Classifier) ClassifyProtocol(protocol string) Classification {
	protocol = strings.ToLower(protocol)

	// Check predefined classifications first
	if class, exists := c.predefinedClassifications[protocol]; exists {
		return Classification{
			Protocol:   protocol,
			Class:      class,
			Confidence: 1.0,
			Source:     "predefined",
		}
	}

	// Check custom rules (sorted by priority)
	for _, rule := range c.customRules {
		if rule.Match(protocol) {
			return Classification{
				Protocol:   protocol,
				Class:      rule.Class,
				Confidence: 0.9, // High confidence for rule matches
				Source:     "custom_rule",
			}
		}
	}

	// Return default classification
	return Classification{
		Protocol:   protocol,
		Class:      c.defaultClass,
		Confidence: 0.5, // Low confidence for default
		Source:     "default",
	}
}

// ClassifyProtocols classifies multiple protocols
func (c *Classifier) ClassifyProtocols(protocols []string) map[string]Classification {
	results := make(map[string]Classification)
	for _, protocol := range protocols {
		results[protocol] = c.ClassifyProtocol(protocol)
	}
	return results
}

// GetPredefinedClassifications returns all predefined classifications
func (c *Classifier) GetPredefinedClassifications() map[string]Class {
	result := make(map[string]Class)
	for k, v := range c.predefinedClassifications {
		result[k] = v
	}
	return result
}

// GetCustomRules returns all custom rules
func (c *Classifier) GetCustomRules() []*Rule {
	result := make([]*Rule, len(c.customRules))
	copy(result, c.customRules)
	return result
}

// SetDefaultClass sets the default QoS class
func (c *Classifier) SetDefaultClass(class Class) {
	c.defaultClass = class
}

// GetDefaultClass returns the default QoS class
func (c *Classifier) GetDefaultClass() Class {
	return c.defaultClass
}

// SetConfidenceThreshold sets the confidence threshold
func (c *Classifier) SetConfidenceThreshold(threshold float64) {
	c.confidenceThreshold = threshold
}

// GetConfidenceThreshold returns the confidence threshold
func (c *Classifier) GetConfidenceThreshold() float64 {
	return c.confidenceThreshold
}

// ValidateProtocolName validates a protocol name
func ValidateProtocolName(protocol string) error {
	if protocol == "" {
		return fmt.Errorf("protocol name cannot be empty")
	}

	// Most Cisco protocol names use lowercase with hyphens and underscores
	validProtocolPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*[a-z0-9]$|^[a-z0-9]$`)
	if !validProtocolPattern.MatchString(strings.ToLower(protocol)) {
		return fmt.Errorf("invalid protocol name format: %s", protocol)
	}

	return nil
}

// IsValidProtocolName checks if a protocol name is valid
func IsValidProtocolName(protocol string) bool {
	return ValidateProtocolName(protocol) == nil
}

// GroupProtocolsByClass groups protocols by their QoS class
func GroupProtocolsByClass(classifications map[string]Classification) map[Class][]string {
	result := make(map[Class][]string)

	for protocol, classification := range classifications {
		result[classification.Class] = append(result[classification.Class], protocol)
	}

	return result
}

// GetClassStatistics returns statistics about classifications
func GetClassStatistics(classifications map[string]Classification) map[Class]int {
	stats := make(map[Class]int)

	for _, classification := range classifications {
		stats[classification.Class]++
	}

	return stats
}

// FilterByConfidence filters classifications by confidence threshold
func FilterByConfidence(classifications map[string]Classification, threshold float64) map[string]Classification {
	result := make(map[string]Classification)

	for protocol, classification := range classifications {
		if classification.Confidence >= threshold {
			result[protocol] = classification
		}
	}

	return result
}

// FilterBySource filters classifications by source
func FilterBySource(classifications map[string]Classification, source string) map[string]Classification {
	result := make(map[string]Classification)

	for protocol, classification := range classifications {
		if classification.Source == source {
			result[protocol] = classification
		}
	}

	return result
}
