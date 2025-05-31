package cache

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

// Entry represents a cache entry
type Entry struct {
	Classification qos.Classification `json:"classification"`
	Timestamp      int64              `json:"timestamp"` // Unix timestamp in nanoseconds
	TTL            int64              `json:"ttl"`       // TTL in nanoseconds
	AccessCount    int                `json:"access_count"`
	LastAccessed   int64              `json:"last_accessed"` // Unix timestamp in nanoseconds
}

// IsExpired checks if the cache entry has expired
func (e *Entry) IsExpired() bool {
	if e.TTL <= 0 {
		return false // No expiration
	}
	return time.Now().UnixNano() >= e.Timestamp+e.TTL
}

// Touch updates the last accessed time and increments access count
func (e *Entry) Touch() {
	e.LastAccessed = time.Now().UnixNano()
	e.AccessCount++
}

// Cache represents a protocol classification cache
type Cache struct {
	entries    map[string]*Entry
	config     *config.CacheConfig
	mutex      sync.RWMutex
	stats      Stats
	maxSize    int
	ttl        time.Duration
	filePath   string
	backupPath string
}

// Stats represents cache statistics
type Stats struct {
	Hits        int64 `json:"hits"`
	Misses      int64 `json:"misses"`
	Evictions   int64 `json:"evictions"`
	Size        int   `json:"size"`
	LastCleanup int64 `json:"last_cleanup"`
}

// New creates a new cache instance
func New(cfg *config.CacheConfig) *Cache {
	return &Cache{
		entries:    make(map[string]*Entry),
		config:     cfg,
		maxSize:    cfg.MaxSize,
		ttl:        cfg.TTL,
		filePath:   cfg.FilePath,
		backupPath: cfg.BackupPath,
		stats:      Stats{},
	}
}

// Get retrieves a classification from the cache
func (c *Cache) Get(protocol string) (qos.Classification, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, exists := c.entries[protocol]
	if !exists {
		c.stats.Misses++
		return qos.Classification{}, false
	}

	// Check if entry has expired
	if entry.IsExpired() {
		delete(c.entries, protocol)
		c.stats.Size--
		c.stats.Misses++
		return qos.Classification{}, false
	}

	// Update access statistics
	entry.Touch()
	c.stats.Hits++

	return entry.Classification, true
}

// Set stores a classification in the cache
func (c *Cache) Set(protocol string, classification qos.Classification) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if entry already exists
	_, exists := c.entries[protocol]

	// Check if we need to evict entries (only if adding new entry)
	if !exists && len(c.entries) >= c.maxSize {
		c.evictLRU()
	}

	// Create new entry
	now := time.Now()
	entry := &Entry{
		Classification: classification,
		Timestamp:      now.UnixNano(),
		TTL:            int64(c.ttl),
		AccessCount:    1,
		LastAccessed:   now.UnixNano(),
	}

	// Store entry
	if !exists {
		c.stats.Size++
	}
	c.entries[protocol] = entry
}

// SetBatch stores multiple classifications in the cache
func (c *Cache) SetBatch(classifications map[string]qos.Classification) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for protocol, classification := range classifications {
		// Check if entry already exists
		_, exists := c.entries[protocol]

		// Check if we need to evict entries (only if adding new entry)
		if !exists && len(c.entries) >= c.maxSize {
			c.evictLRU()
		}

		// Create new entry
		now := time.Now()
		entry := &Entry{
			Classification: classification,
			Timestamp:      now.UnixNano(),
			TTL:            int64(c.ttl),
			AccessCount:    1,
			LastAccessed:   now.UnixNano(),
		}

		// Store entry
		if !exists {
			c.stats.Size++
		}
		c.entries[protocol] = entry
	}
}

// Delete removes a classification from the cache
func (c *Cache) Delete(protocol string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, exists := c.entries[protocol]; exists {
		delete(c.entries, protocol)
		c.stats.Size--
	}
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries = make(map[string]*Entry)
	c.stats.Size = 0
}

// Size returns the number of entries in the cache
func (c *Cache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.stats.Size = len(c.entries)
	return len(c.entries)
}

// GetStats returns cache statistics
func (c *Cache) GetStats() Stats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	stats := c.stats
	stats.Size = len(c.entries)
	return stats
}

// HitRate returns the cache hit rate
func (c *Cache) HitRate() float64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	total := c.stats.Hits + c.stats.Misses
	if total == 0 {
		return 0
	}
	return float64(c.stats.Hits) / float64(total)
}

// evictLRU evicts the least recently used entry
func (c *Cache) evictLRU() {
	var oldestProtocol string
	var oldestTime int64 = time.Now().UnixNano()

	for protocol, entry := range c.entries {
		if entry.LastAccessed < oldestTime {
			oldestTime = entry.LastAccessed
			oldestProtocol = protocol
		}
	}

	if oldestProtocol != "" {
		delete(c.entries, oldestProtocol)
		c.stats.Evictions++
	}
}

// Cleanup removes expired entries
func (c *Cache) Cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now().Unix()
	removed := 0
	for protocol, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, protocol)
			removed++
		}
	}
	c.stats.Size = len(c.entries)
	c.stats.LastCleanup = now
}

// StartCleanupRoutine starts a background cleanup routine
func (c *Cache) StartCleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			c.Cleanup()
		}
	}()
}

// Load loads cache from file
func (c *Cache) Load() error {
	if c.filePath == "" {
		return nil
	}

	file, err := os.Open(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, that's okay
		}
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file

	// Check if compression is enabled
	if c.config.Compression {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			// Try reading as uncompressed
			if _, seekErr := file.Seek(0, 0); seekErr != nil {
				return fmt.Errorf("failed to seek file: %w", seekErr)
			}
			reader = file
		} else {
			defer gzReader.Close()
			reader = gzReader
		}
	}

	// Try to load as new format first
	var entries map[string]*Entry
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&entries); err != nil {
		// Fallback to old format (map[string]string)
		if _, seekErr := file.Seek(0, 0); seekErr != nil {
			return fmt.Errorf("failed to seek file for fallback: %w", seekErr)
		}
		if c.config.Compression {
			gzReader, gzErr := gzip.NewReader(file)
			if gzErr != nil {
				return fmt.Errorf("failed to create gzip reader for fallback: %w", gzErr)
			}
			defer gzReader.Close()
			reader = gzReader
		} else {
			reader = file
		}

		var oldFormat map[string]string
		decoder = json.NewDecoder(reader)
		if err := decoder.Decode(&oldFormat); err != nil {
			return fmt.Errorf("failed to decode cache file: %w", err)
		}

		// Convert old format to new format
		entries = make(map[string]*Entry)
		for protocol, classStr := range oldFormat {
			entries[protocol] = &Entry{
				Classification: qos.Classification{
					Protocol: protocol,
					Class:    qos.Class(classStr),
					Source:   "cache",
				},
				Timestamp:    time.Now().Unix(),
				TTL:          int64(c.ttl.Seconds()),
				AccessCount:  0,
				LastAccessed: time.Now().Unix(),
			}
		}
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries = entries
	c.stats.Size = len(entries)

	return nil
}

// Save saves cache to file
func (c *Cache) Save() error {
	if c.filePath == "" {
		return nil
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Create backup if backup path is specified
	if c.backupPath != "" {
		if _, err := os.Stat(c.filePath); err == nil {
			if err := c.copyFile(c.filePath, c.backupPath); err != nil {
				// Log error but don't fail the save operation
				fmt.Printf("Warning: failed to create backup: %v\n", err)
			}
		}
	}

	file, err := os.Create(c.filePath)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	var writer io.Writer = file

	// Use compression if enabled
	if c.config.Compression {
		gzWriter := gzip.NewWriter(file)
		defer gzWriter.Close()
		writer = gzWriter
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c.entries); err != nil {
		return fmt.Errorf("failed to encode cache: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func (c *Cache) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// GetAll returns all cached classifications
func (c *Cache) GetAll() map[string]qos.Classification {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]qos.Classification)
	for protocol, entry := range c.entries {
		if !entry.IsExpired() {
			result[protocol] = entry.Classification
		}
	}

	return result
}

// GetValidProtocols returns all cached protocols that are in the provided list
func (c *Cache) GetValidProtocols(validProtocols []string) map[string]qos.Classification {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	validSet := make(map[string]bool)
	for _, protocol := range validProtocols {
		validSet[protocol] = true
	}

	result := make(map[string]qos.Classification)
	for protocol, entry := range c.entries {
		if validSet[protocol] && !entry.IsExpired() {
			result[protocol] = entry.Classification
		}
	}

	return result
}

// Exists checks if a protocol exists in the cache
func (c *Cache) Exists(protocol string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[protocol]
	if !exists {
		return false
	}

	return !entry.IsExpired()
}

// GetExpiredEntries returns all expired entries
func (c *Cache) GetExpiredEntries() map[string]*Entry {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string]*Entry)
	for protocol, entry := range c.entries {
		if entry.IsExpired() {
			result[protocol] = entry
		}
	}

	return result
}

// RemoveExpiredEntries removes all expired entries
func (c *Cache) RemoveExpiredEntries() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	removed := 0
	for protocol, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, protocol)
			removed++
		}
	}

	c.stats.Size = len(c.entries)
	return removed
}
