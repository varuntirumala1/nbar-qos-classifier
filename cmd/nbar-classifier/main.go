package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/ai"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/cache"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/metrics"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/ssh"
)

// Application represents the main application
type Application struct {
	config     *config.Config
	logger     *logger.Logger
	metrics    *metrics.Metrics
	cache      *cache.Cache
	sshClient  *ssh.Client
	aiManager  *ai.Manager
	classifier *qos.Classifier
}

// Version information (set by build)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Parse command line flags
	var (
		configPath      = flag.String("config", "", "Path to configuration file")
		showVersion     = flag.Bool("version", false, "Show version information")
		fetchFromSwitch = flag.Bool("fetch-from-switch", false, "Fetch protocol list from switch via SSH")
		inputFile       = flag.String("input-file", "", "Input file containing NBAR protocol list")
		outputType      = flag.String("output", "text", "Output format: 'text' or 'cisco'")
		pushConfig      = flag.Bool("push-config", false, "Push updated config to switch via SSH")
		dryRun          = flag.Bool("dry-run", false, "Test implementation without pushing config")
		saveConfig      = flag.Bool("save-config", false, "Save configuration to startup-config after pushing changes")
		batchSize       = flag.Int("batch-size", 0, "Number of protocols to analyze in each AI batch (0 = use config)")
		logLevel        = flag.String("log-level", "", "Log level (debug, info, warn, error)")
		enableMetrics   = flag.Bool("enable-metrics", false, "Enable metrics server")
		enableWeb       = flag.Bool("enable-web", false, "Enable web interface")
	)
	flag.Parse()

	// Show version information
	if *showVersion {
		fmt.Printf("NBAR QoS Classifier\n")
		fmt.Printf("Version: %s\n", Version)
		fmt.Printf("Git Commit: %s\n", GitCommit)
		fmt.Printf("Build Time: %s\n", BuildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Override config with command line flags
	if *batchSize > 0 {
		cfg.App.BatchSize = *batchSize
	}
	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}
	if *enableMetrics {
		cfg.Metrics.Enabled = true
	}
	if *enableWeb {
		cfg.Web.Enabled = true
	}

	// Initialize logger
	log, err := logger.New(&cfg.Logging)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	log.WithFields(logger.Fields{
		"version":    Version,
		"git_commit": GitCommit,
		"build_time": BuildTime,
	}).Info("Starting NBAR QoS Classifier")

	// Create application
	app, err := NewApplication(cfg, log)
	if err != nil {
		log.WithError(err).Fatal("Failed to create application")
	}
	defer app.Close()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received shutdown signal")
		cancel()
	}()

	// Start background services
	if err := app.StartServices(ctx); err != nil {
		log.WithError(err).Fatal("Failed to start services")
	}

	// Execute main operation based on flags
	if err := app.Execute(ctx, &ExecuteOptions{
		FetchFromSwitch: *fetchFromSwitch,
		InputFile:       *inputFile,
		OutputType:      *outputType,
		PushConfig:      *pushConfig,
		DryRun:          *dryRun,
		SaveConfig:      *saveConfig,
	}); err != nil {
		log.WithError(err).Fatal("Execution failed")
	}

	log.Info("Application completed successfully")
}

// NewApplication creates a new application instance
func NewApplication(cfg *config.Config, log *logger.Logger) (*Application, error) {
	app := &Application{
		config: cfg,
		logger: log,
	}

	// Initialize metrics
	if cfg.Metrics.Enabled {
		app.metrics = metrics.New(&cfg.Metrics, log)
	}

	// Initialize cache
	app.cache = cache.New(&cfg.Cache)
	if err := app.cache.Load(); err != nil {
		log.WithError(err).Warn("Failed to load cache")
	}

	// Initialize SSH client
	sshClient, err := ssh.New(&cfg.SSH, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}
	app.sshClient = sshClient

	// Initialize AI manager
	aiManager, err := ai.NewManager(&cfg.AI, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI manager: %w", err)
	}
	app.aiManager = aiManager

	// Initialize QoS classifier
	app.classifier = qos.NewClassifier(
		qos.Class(cfg.QoS.DefaultClass),
		cfg.QoS.ConfidenceThreshold,
	)

	// Load predefined classifications
	if err := app.loadPredefinedClassifications(); err != nil {
		return nil, fmt.Errorf("failed to load predefined classifications: %w", err)
	}

	// Load custom rules
	if err := app.loadCustomRules(); err != nil {
		return nil, fmt.Errorf("failed to load custom rules: %w", err)
	}

	return app, nil
}

// loadPredefinedClassifications loads predefined protocol classifications
func (app *Application) loadPredefinedClassifications() error {
	for className, classConfig := range app.config.QoS.Classes {
		class := qos.Class(className)
		if !class.IsValid() {
			app.logger.WithField("class", className).Warn("Invalid QoS class in configuration")
			continue
		}

		for _, protocol := range classConfig.Protocols {
			app.classifier.AddPredefinedClassification(protocol, class)
		}
	}

	app.logger.WithField("count", len(app.classifier.GetPredefinedClassifications())).Info("Loaded predefined classifications")
	return nil
}

// loadCustomRules loads custom classification rules
func (app *Application) loadCustomRules() error {
	for _, ruleConfig := range app.config.QoS.CustomRules {
		if !ruleConfig.Enabled {
			continue
		}

		rule, err := qos.NewRule(
			ruleConfig.Name,
			ruleConfig.Pattern,
			qos.Class(ruleConfig.Class),
			ruleConfig.Priority,
		)
		if err != nil {
			app.logger.WithError(err).WithField("rule", ruleConfig.Name).Warn("Failed to create custom rule")
			continue
		}

		if err := app.classifier.AddCustomRule(rule); err != nil {
			app.logger.WithError(err).WithField("rule", ruleConfig.Name).Warn("Failed to add custom rule")
			continue
		}
	}

	app.logger.WithField("count", len(app.classifier.GetCustomRules())).Info("Loaded custom rules")
	return nil
}

// StartServices starts background services
func (app *Application) StartServices(ctx context.Context) error {
	// Start metrics server
	if app.metrics != nil {
		go func() {
			if err := app.metrics.StartServer(); err != nil {
				app.logger.WithError(err).Error("Metrics server failed")
			}
		}()
	}

	// Start cache cleanup routine
	if app.cache != nil {
		app.cache.StartCleanupRoutine(time.Hour)
	}

	return nil
}

// ExecuteOptions contains options for the main execution
type ExecuteOptions struct {
	FetchFromSwitch bool
	InputFile       string
	OutputType      string
	PushConfig      bool
	DryRun          bool
	SaveConfig      bool
}

// Execute runs the main application logic
func (app *Application) Execute(ctx context.Context, opts *ExecuteOptions) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		app.logger.Performance("total_execution", duration, logger.Fields{
			"fetch_from_switch": opts.FetchFromSwitch,
			"output_type":       opts.OutputType,
			"push_config":       opts.PushConfig,
			"dry_run":           opts.DryRun,
		})
	}()

	// Fetch protocols
	var protocols []string
	var err error

	if opts.FetchFromSwitch {
		app.logger.Info("Fetching protocols from switch")
		protocols, err = app.sshClient.FetchProtocols()
		if err != nil {
			return fmt.Errorf("failed to fetch protocols from switch: %w", err)
		}
		app.logger.WithField("count", len(protocols)).Info("Fetched protocols from switch")
	} else if opts.InputFile != "" {
		app.logger.WithField("file", opts.InputFile).Info("Loading protocols from file")
		protocols, err = app.loadProtocolsFromFile(opts.InputFile)
		if err != nil {
			return fmt.Errorf("failed to load protocols from file: %w", err)
		}
		app.logger.WithField("count", len(protocols)).Info("Loaded protocols from file")
	} else {
		return fmt.Errorf("either --fetch-from-switch or --input-file must be specified")
	}

	// Classify protocols
	app.logger.Info("Starting protocol classification")
	classifications, err := app.classifyProtocols(ctx, protocols)
	if err != nil {
		return fmt.Errorf("failed to classify protocols: %w", err)
	}

	// Generate output
	output, err := app.generateOutput(classifications, opts.OutputType)
	if err != nil {
		return fmt.Errorf("failed to generate output: %w", err)
	}

	// Write output to file
	outputFile := "nbar-protocols-qos.txt"
	if err := os.WriteFile(outputFile, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	// Log statistics
	app.logStatistics(classifications)

	// Handle config push/dry run
	if opts.PushConfig || opts.DryRun {
		if err := app.handleConfigPush(ctx, output, opts); err != nil {
			return fmt.Errorf("failed to handle config push: %w", err)
		}
	}

	app.logger.WithField("output_file", outputFile).Info("Classification completed successfully")
	return nil
}

// Close closes the application and cleans up resources
func (app *Application) Close() error {
	var errors []error

	if app.cache != nil {
		if err := app.cache.Save(); err != nil {
			errors = append(errors, fmt.Errorf("failed to save cache: %w", err))
		}
	}

	if app.sshClient != nil {
		if err := app.sshClient.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close SSH client: %w", err))
		}
	}

	if app.aiManager != nil {
		if err := app.aiManager.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close AI manager: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("multiple errors during close: %v", errors)
	}

	return nil
}

// loadProtocolsFromFile loads protocols from an input file
func (app *Application) loadProtocolsFromFile(filename string) ([]string, error) {
	app.logger.WithField("file", filename).Debug("Loading protocols from file")

	// Read the file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Parse protocols (one per line, ignore empty lines and comments)
	var protocols []string
	lines := strings.Split(string(data), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Validate protocol name
		if err := qos.ValidateProtocolName(line); err != nil {
			app.logger.WithFields(logger.Fields{
				"line":     i + 1,
				"protocol": line,
				"error":    err.Error(),
			}).Warn("Invalid protocol name, skipping")
			continue
		}

		protocols = append(protocols, strings.ToLower(line))
	}

	app.logger.WithFields(logger.Fields{
		"file":           filename,
		"total_lines":    len(lines),
		"valid_protocols": len(protocols),
	}).Info("Loaded protocols from file")

	return protocols, nil
}

// classifyProtocols classifies a list of protocols
func (app *Application) classifyProtocols(ctx context.Context, protocols []string) (map[string]qos.Classification, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		app.logger.Performance("protocol_classification", duration, logger.Fields{
			"protocol_count": len(protocols),
		})
	}()

	results := make(map[string]qos.Classification)

	// First, try to classify using predefined rules and cache
	needAIClassification := make([]string, 0)

	for _, protocol := range protocols {
		// Check cache first
		if cached, found := app.cache.Get(protocol); found {
			results[protocol] = cached
			if app.metrics != nil {
				app.metrics.RecordCacheHit("protocol_classification")
			}
			continue
		}

		if app.metrics != nil {
			app.metrics.RecordCacheMiss("protocol_classification")
		}

		// Try predefined classification
		classification := app.classifier.ClassifyProtocol(protocol)
		if classification.Source != "default" {
			results[protocol] = classification
			app.cache.Set(protocol, classification)
			continue
		}

		// Need AI classification
		needAIClassification = append(needAIClassification, protocol)
	}

	app.logger.WithFields(logger.Fields{
		"cached_count":    len(protocols) - len(needAIClassification),
		"ai_needed_count": len(needAIClassification),
	}).Info("Initial classification results")

	// Use AI for remaining protocols
	if len(needAIClassification) > 0 {
		app.logger.WithField("count", len(needAIClassification)).Info("Starting AI classification")

		aiResults, err := app.aiManager.ClassifyProtocols(ctx, needAIClassification, app.config.App.BatchSize)
		if err != nil {
			app.logger.WithError(err).Warn("AI classification failed, using default classifications")
			// Continue with default classifications instead of failing
		} else {
			// Merge AI results
			for protocol, classification := range aiResults {
				results[protocol] = classification
				app.cache.Set(protocol, classification)
			}
		}

		// Set default for any remaining unclassified protocols
		for _, protocol := range needAIClassification {
			if _, exists := results[protocol]; !exists {
				defaultClassification := qos.Classification{
					Protocol:   protocol,
					Class:      app.classifier.GetDefaultClass(),
					Confidence: 0.5,
					Source:     "default",
					Timestamp:  time.Now().Unix(),
				}
				results[protocol] = defaultClassification
				app.cache.Set(protocol, defaultClassification)
			}
		}
	}

	// Record metrics
	if app.metrics != nil {
		for _, classification := range results {
			app.metrics.RecordProtocolClassification(
				classification.Class.String(),
				classification.Source,
				time.Since(start),
			)
		}
	}

	return results, nil
}

// generateOutput generates the output in the specified format
func (app *Application) generateOutput(classifications map[string]qos.Classification, outputType string) (string, error) {
	switch outputType {
	case "cisco":
		return app.generateCiscoConfig(classifications)
	case "text":
		return app.generateTextOutput(classifications)
	default:
		return "", fmt.Errorf("unsupported output type: %s", outputType)
	}
}

// generateCiscoConfig generates Cisco configuration output
func (app *Application) generateCiscoConfig(classifications map[string]qos.Classification) (string, error) {
	// Group protocols by QoS class
	grouped := qos.GroupProtocolsByClass(classifications)

	var output strings.Builder

	// Helper function to write class-maps with max 16 protocols each
	writeClassMaps := func(className, description string, protocols []string) []string {
		if len(protocols) == 0 {
			return []string{}
		}

		numClassMaps := (len(protocols) + 15) / 16
		classMapNames := make([]string, 0, numClassMaps)

		for i := 0; i < numClassMaps; i++ {
			classMapName := className
			if numClassMaps > 1 {
				classMapName = fmt.Sprintf("%s_%d", className, i+1)
			}
			classMapNames = append(classMapNames, classMapName)

			output.WriteString(fmt.Sprintf("class-map match-any %s\n", classMapName))
			output.WriteString(fmt.Sprintf(" description %s\n", description))

			start := i * 16
			end := start + 16
			if end > len(protocols) {
				end = len(protocols)
			}

			for j := start; j < end; j++ {
				output.WriteString(fmt.Sprintf(" match protocol %s\n", protocols[j]))
			}
			output.WriteString("!\n")
		}

		return classMapNames
	}

	// Generate class-maps for each QoS class
	efClassMaps := writeClassMaps("QOS_EF", "Expedited Forwarding - Real-time traffic", grouped[qos.EF])
	af41ClassMaps := writeClassMaps("QOS_AF41", "Assured Forwarding 41 - Business-critical applications", grouped[qos.AF41])
	af21ClassMaps := writeClassMaps("QOS_AF21", "Assured Forwarding 21 - Important data applications", grouped[qos.AF21])
	_ = writeClassMaps("QOS_CS1", "Class Selector 1 - Background traffic", grouped[qos.CS1])

	// Generate policy-map
	output.WriteString("! Ingress marking policy-map\n")
	output.WriteString("policy-map PM_MARK_AVC_WIRED_INGRESS\n")
	output.WriteString(" description Marks incoming traffic based on App (AVC) or VLAN fallback. CS1 traffic handled by class-default.\n")

	for _, cm := range efClassMaps {
		output.WriteString(fmt.Sprintf(" class %s\n  set dscp ef\n", cm))
	}
	for _, cm := range af41ClassMaps {
		output.WriteString(fmt.Sprintf(" class %s\n  set dscp af41\n", cm))
	}
	for _, cm := range af21ClassMaps {
		output.WriteString(fmt.Sprintf(" class %s\n  set dscp af21\n", cm))
	}
	output.WriteString(" class class-default\n  set dscp cs1\n!\n")

	return output.String(), nil
}

// generateTextOutput generates human-readable text output
func (app *Application) generateTextOutput(classifications map[string]qos.Classification) (string, error) {
	grouped := qos.GroupProtocolsByClass(classifications)

	var output strings.Builder

	currentDate := time.Now().Format("2006-01-02")
	output.WriteString("# NBAR Protocols Classified by QoS\n")
	output.WriteString(fmt.Sprintf("# Generated on %s using AI\n", currentDate))
	output.WriteString("# For use with Cisco 9300 Switch\n\n")

	// Write each QoS class section
	classes := []qos.Class{qos.EF, qos.AF41, qos.AF21, qos.CS1}
	for _, class := range classes {
		protocols := grouped[class]
		if len(protocols) == 0 {
			continue
		}

		output.WriteString(fmt.Sprintf("## %s - %s\n", class, class.Description()))
		for i, protocol := range protocols {
			output.WriteString(fmt.Sprintf("%d. %s\n", i+1, protocol))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

// logStatistics logs classification statistics
func (app *Application) logStatistics(classifications map[string]qos.Classification) {
	stats := qos.GetClassStatistics(classifications)

	app.logger.WithFields(logger.Fields{
		"total_protocols": len(classifications),
		"ef_count":        stats[qos.EF],
		"af41_count":      stats[qos.AF41],
		"af21_count":      stats[qos.AF21],
		"cs1_count":       stats[qos.CS1],
	}).Info("Classification statistics")

	// Update metrics
	if app.metrics != nil {
		for class, count := range stats {
			app.metrics.SetQoSClassDistribution(class.String(), count)
		}
	}
}

// handleConfigPush handles configuration push or dry run
func (app *Application) handleConfigPush(ctx context.Context, config string, opts *ExecuteOptions) error {
	if opts.DryRun {
		app.logger.Info("Running in dry-run mode - configuration will not be applied")

		// Show what would be done
		app.logger.WithFields(logger.Fields{
			"config_length": len(config),
			"push_config":   opts.PushConfig,
			"save_config":   opts.SaveConfig,
		}).Info("Dry-run: Configuration ready for deployment")

		// Write dry-run output to file
		dryRunFile := "nbar-protocols-qos-dryrun.txt"
		if err := os.WriteFile(dryRunFile, []byte(config), 0644); err != nil {
			app.logger.WithError(err).Warn("Failed to write dry-run file")
		} else {
			app.logger.WithField("file", dryRunFile).Info("Dry-run configuration written to file")
		}

		return nil
	}

	if opts.PushConfig {
		app.logger.Info("Pushing configuration to switch")

		// Push configuration to switch
		if err := app.sshClient.PushConfig(config); err != nil {
			return fmt.Errorf("failed to push configuration: %w", err)
		}

		app.logger.Info("Configuration successfully pushed to switch")

		// Save configuration if requested
		if opts.SaveConfig {
			app.logger.Info("Saving configuration to startup-config")
			if err := app.sshClient.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}
			app.logger.Info("Configuration successfully saved to startup-config")
		}
	}

	return nil
}
