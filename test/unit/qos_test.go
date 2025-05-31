package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

func TestQoSClass(t *testing.T) {
	tests := []struct {
		name        string
		class       qos.Class
		isValid     bool
		priority    int
		dscp        string
		description string
	}{
		{
			name:        "EF class",
			class:       qos.EF,
			isValid:     true,
			priority:    1,
			dscp:        "ef",
			description: "Expedited Forwarding - Real-time traffic (Voice, Video calls)",
		},
		{
			name:        "AF41 class",
			class:       qos.AF41,
			isValid:     true,
			priority:    2,
			dscp:        "af41",
			description: "Assured Forwarding 41 - Business-critical applications",
		},
		{
			name:        "AF21 class",
			class:       qos.AF21,
			isValid:     true,
			priority:    3,
			dscp:        "af21",
			description: "Assured Forwarding 21 - Important data applications",
		},
		{
			name:        "CS1 class",
			class:       qos.CS1,
			isValid:     true,
			priority:    4,
			dscp:        "cs1",
			description: "Class Selector 1 - Background traffic",
		},
		{
			name:        "Invalid class",
			class:       qos.Class("INVALID"),
			isValid:     false,
			priority:    6,
			dscp:        "cs0",
			description: "Unknown QoS class",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isValid, tt.class.IsValid())
			assert.Equal(t, tt.priority, tt.class.Priority())
			assert.Equal(t, tt.dscp, tt.class.DSCP())
			assert.Equal(t, tt.description, tt.class.Description())
		})
	}
}

func TestClassification(t *testing.T) {
	classification := qos.Classification{
		Protocol:   "test-protocol",
		Class:      qos.EF,
		Confidence: 0.9,
		Source:     "ai",
		Timestamp:  1234567890,
	}

	assert.Equal(t, "test-protocol", classification.Protocol)
	assert.Equal(t, qos.EF, classification.Class)
	assert.Equal(t, 0.9, classification.Confidence)
	assert.Equal(t, "ai", classification.Source)
	assert.Equal(t, int64(1234567890), classification.Timestamp)
}

func TestRule(t *testing.T) {
	t.Run("Valid rule", func(t *testing.T) {
		rule, err := qos.NewRule("test-rule", ".*voice.*", qos.EF, 1)
		require.NoError(t, err)
		assert.Equal(t, "test-rule", rule.Name)
		assert.Equal(t, ".*voice.*", rule.Pattern)
		assert.Equal(t, qos.EF, rule.Class)
		assert.Equal(t, 1, rule.Priority)
		assert.True(t, rule.Enabled)
	})

	t.Run("Invalid regex pattern", func(t *testing.T) {
		_, err := qos.NewRule("test-rule", "[invalid", qos.EF, 1)
		assert.Error(t, err)
	})

	t.Run("Rule matching", func(t *testing.T) {
		rule, err := qos.NewRule("voice-rule", ".*voice.*", qos.EF, 1)
		require.NoError(t, err)

		assert.True(t, rule.Match("voice-protocol"))
		assert.True(t, rule.Match("test-voice-app"))
		assert.False(t, rule.Match("video-protocol"))
	})

	t.Run("Disabled rule", func(t *testing.T) {
		rule, err := qos.NewRule("disabled-rule", ".*test.*", qos.EF, 1)
		require.NoError(t, err)
		rule.Enabled = false

		assert.False(t, rule.Match("test-protocol"))
	})
}

func TestClassifier(t *testing.T) {
	classifier := qos.NewClassifier(qos.CS1, 0.8)

	t.Run("Default classification", func(t *testing.T) {
		classification := classifier.ClassifyProtocol("unknown-protocol")
		assert.Equal(t, "unknown-protocol", classification.Protocol)
		assert.Equal(t, qos.CS1, classification.Class)
		assert.Equal(t, "default", classification.Source)
	})

	t.Run("Predefined classification", func(t *testing.T) {
		classifier.AddPredefinedClassification("sip", qos.EF)
		classification := classifier.ClassifyProtocol("sip")
		assert.Equal(t, "sip", classification.Protocol)
		assert.Equal(t, qos.EF, classification.Class)
		assert.Equal(t, "predefined", classification.Source)
		assert.Equal(t, 1.0, classification.Confidence)
	})

	t.Run("Custom rule classification", func(t *testing.T) {
		rule, err := qos.NewRule("voice-rule", ".*voice.*", qos.EF, 1)
		require.NoError(t, err)

		err = classifier.AddCustomRule(rule)
		require.NoError(t, err)

		classification := classifier.ClassifyProtocol("voice-app")
		assert.Equal(t, "voice-app", classification.Protocol)
		assert.Equal(t, qos.EF, classification.Class)
		assert.Equal(t, "custom_rule", classification.Source)
		assert.Equal(t, 0.9, classification.Confidence)
	})

	t.Run("Multiple protocols", func(t *testing.T) {
		protocols := []string{"sip", "voice-app", "unknown"}
		classifications := classifier.ClassifyProtocols(protocols)

		assert.Len(t, classifications, 3)
		assert.Equal(t, qos.EF, classifications["sip"].Class)
		assert.Equal(t, qos.EF, classifications["voice-app"].Class)
		assert.Equal(t, qos.CS1, classifications["unknown"].Class)
	})
}

func TestValidateProtocolName(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		valid    bool
	}{
		{"Valid protocol", "http", true},
		{"Valid with hyphen", "web-rtc", true},
		{"Valid with underscore", "ms_teams", true},
		{"Valid with numbers", "h264", true},
		{"Single character", "a", true},
		{"Empty string", "", false},
		{"Invalid characters", "http@", false},
		{"Starting with hyphen", "-http", false},
		{"Ending with hyphen", "http-", false},
		{"Uppercase", "HTTP", true}, // Should be converted to lowercase
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := qos.ValidateProtocolName(tt.protocol)
			if tt.valid {
				assert.NoError(t, err)
				assert.True(t, qos.IsValidProtocolName(tt.protocol))
			} else {
				assert.Error(t, err)
				assert.False(t, qos.IsValidProtocolName(tt.protocol))
			}
		})
	}
}

func TestGroupProtocolsByClass(t *testing.T) {
	classifications := map[string]qos.Classification{
		"sip":     {Protocol: "sip", Class: qos.EF},
		"http":    {Protocol: "http", Class: qos.AF21},
		"youtube": {Protocol: "youtube", Class: qos.AF41},
		"unknown": {Protocol: "unknown", Class: qos.CS1},
	}

	grouped := qos.GroupProtocolsByClass(classifications)

	assert.Contains(t, grouped[qos.EF], "sip")
	assert.Contains(t, grouped[qos.AF21], "http")
	assert.Contains(t, grouped[qos.AF41], "youtube")
	assert.Contains(t, grouped[qos.CS1], "unknown")
}

func TestGetClassStatistics(t *testing.T) {
	classifications := map[string]qos.Classification{
		"sip":     {Protocol: "sip", Class: qos.EF},
		"rtp":     {Protocol: "rtp", Class: qos.EF},
		"http":    {Protocol: "http", Class: qos.AF21},
		"youtube": {Protocol: "youtube", Class: qos.AF41},
		"unknown": {Protocol: "unknown", Class: qos.CS1},
	}

	stats := qos.GetClassStatistics(classifications)

	assert.Equal(t, 2, stats[qos.EF])
	assert.Equal(t, 1, stats[qos.AF21])
	assert.Equal(t, 1, stats[qos.AF41])
	assert.Equal(t, 1, stats[qos.CS1])
}

func TestFilterByConfidence(t *testing.T) {
	classifications := map[string]qos.Classification{
		"high":   {Protocol: "high", Class: qos.EF, Confidence: 0.9},
		"medium": {Protocol: "medium", Class: qos.AF21, Confidence: 0.7},
		"low":    {Protocol: "low", Class: qos.CS1, Confidence: 0.5},
	}

	filtered := qos.FilterByConfidence(classifications, 0.8)

	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "high")
	assert.NotContains(t, filtered, "medium")
	assert.NotContains(t, filtered, "low")
}

func TestFilterBySource(t *testing.T) {
	classifications := map[string]qos.Classification{
		"ai":         {Protocol: "ai", Class: qos.EF, Source: "ai"},
		"predefined": {Protocol: "predefined", Class: qos.AF21, Source: "predefined"},
		"cache":      {Protocol: "cache", Class: qos.CS1, Source: "cache"},
	}

	filtered := qos.FilterBySource(classifications, "ai")

	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "ai")
	assert.NotContains(t, filtered, "predefined")
	assert.NotContains(t, filtered, "cache")
}
