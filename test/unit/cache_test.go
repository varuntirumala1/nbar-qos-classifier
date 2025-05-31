package unit

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/cache"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

func TestCache(t *testing.T) {
	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         time.Hour,
		MaxSize:     100,
		FilePath:    "test_cache.json",
		Compression: false,
		BackupPath:  "test_cache_backup.json",
	}

	// Clean up test files
	defer func() {
		os.Remove(cfg.FilePath)
		os.Remove(cfg.BackupPath)
	}()

	c := cache.New(cfg)

	t.Run("Basic operations", func(t *testing.T) {
		// Test empty cache
		_, found := c.Get("nonexistent")
		assert.False(t, found)
		assert.Equal(t, 0, c.Size())

		// Test set and get
		classification := qos.Classification{
			Protocol:   "test-protocol",
			Class:      qos.EF,
			Confidence: 0.9,
			Source:     "ai",
			Timestamp:  time.Now().Unix(),
		}

		c.Set("test-protocol", classification)
		assert.Equal(t, 1, c.Size())

		retrieved, found := c.Get("test-protocol")
		assert.True(t, found)
		assert.Equal(t, classification.Protocol, retrieved.Protocol)
		assert.Equal(t, classification.Class, retrieved.Class)
		assert.Equal(t, classification.Confidence, retrieved.Confidence)
		assert.Equal(t, classification.Source, retrieved.Source)
	})

	t.Run("Batch operations", func(t *testing.T) {
		c.Clear()

		classifications := map[string]qos.Classification{
			"protocol1": {Protocol: "protocol1", Class: qos.EF, Source: "ai"},
			"protocol2": {Protocol: "protocol2", Class: qos.AF41, Source: "ai"},
			"protocol3": {Protocol: "protocol3", Class: qos.AF21, Source: "ai"},
		}

		c.SetBatch(classifications)
		assert.Equal(t, 3, c.Size())

		for protocol, expected := range classifications {
			retrieved, found := c.Get(protocol)
			assert.True(t, found)
			assert.Equal(t, expected.Protocol, retrieved.Protocol)
			assert.Equal(t, expected.Class, retrieved.Class)
		}
	})

	t.Run("Delete operation", func(t *testing.T) {
		c.Set("to-delete", qos.Classification{Protocol: "to-delete", Class: qos.CS1})
		assert.Equal(t, 4, c.Size()) // 3 from batch + 1 new

		c.Delete("to-delete")
		assert.Equal(t, 3, c.Size())

		_, found := c.Get("to-delete")
		assert.False(t, found)
	})

	t.Run("Exists operation", func(t *testing.T) {
		assert.True(t, c.Exists("protocol1"))
		assert.False(t, c.Exists("nonexistent"))
	})

	t.Run("GetAll operation", func(t *testing.T) {
		all := c.GetAll()
		assert.Len(t, all, 3)
		assert.Contains(t, all, "protocol1")
		assert.Contains(t, all, "protocol2")
		assert.Contains(t, all, "protocol3")
	})

	t.Run("GetValidProtocols operation", func(t *testing.T) {
		validProtocols := []string{"protocol1", "protocol3", "nonexistent"}
		valid := c.GetValidProtocols(validProtocols)
		assert.Len(t, valid, 2)
		assert.Contains(t, valid, "protocol1")
		assert.Contains(t, valid, "protocol3")
		assert.NotContains(t, valid, "nonexistent")
	})
}

func TestCacheStats(t *testing.T) {
	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         time.Hour,
		MaxSize:     100,
		FilePath:    "",
		Compression: false,
	}

	c := cache.New(cfg)

	// Initial stats
	stats := c.GetStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0, stats.Size)

	// Add some data and test stats
	c.Set("test", qos.Classification{Protocol: "test", Class: qos.EF})

	// Test cache hit
	_, found := c.Get("test")
	assert.True(t, found)

	// Test cache miss
	_, found = c.Get("nonexistent")
	assert.False(t, found)

	stats = c.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 1, stats.Size)

	// Test hit rate
	hitRate := c.HitRate()
	assert.Equal(t, 0.5, hitRate) // 1 hit out of 2 total requests
}

func TestCacheExpiration(t *testing.T) {
	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         100 * time.Millisecond, // Very short TTL for testing
		MaxSize:     100,
		FilePath:    "",
		Compression: false,
	}

	c := cache.New(cfg)

	// Add an entry
	c.Set("expiring", qos.Classification{Protocol: "expiring", Class: qos.EF})

	// Should be available immediately
	_, found := c.Get("expiring")
	assert.True(t, found)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	_, found = c.Get("expiring")
	assert.False(t, found)
}

func TestCacheEviction(t *testing.T) {
	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         time.Hour,
		MaxSize:     2, // Very small cache for testing eviction
		FilePath:    "",
		Compression: false,
	}

	c := cache.New(cfg)

	// Fill the cache to capacity
	c.Set("first", qos.Classification{Protocol: "first", Class: qos.EF})
	c.Set("second", qos.Classification{Protocol: "second", Class: qos.AF41})
	assert.Equal(t, 2, c.Size())

	// Access first to make it more recently used
	c.Get("first")

	// Add third item, should evict "second" (LRU)
	c.Set("third", qos.Classification{Protocol: "third", Class: qos.AF21})
	assert.Equal(t, 2, c.Size())

	// "first" and "third" should exist, "second" should be evicted
	_, found := c.Get("first")
	assert.True(t, found)

	_, found = c.Get("third")
	assert.True(t, found)

	_, found = c.Get("second")
	assert.False(t, found)
}

func TestCacheCleanup(t *testing.T) {
	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         50 * time.Millisecond,
		MaxSize:     100,
		FilePath:    "",
		Compression: false,
	}

	c := cache.New(cfg)

	// Add some entries
	c.Set("item1", qos.Classification{Protocol: "item1", Class: qos.EF})
	c.Set("item2", qos.Classification{Protocol: "item2", Class: qos.AF41})
	assert.Equal(t, 2, c.Size())

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Manual cleanup
	c.Cleanup()
	assert.Equal(t, 0, c.Size())
}

func TestCachePersistence(t *testing.T) {
	testFile := "test_persistence.json"
	defer os.Remove(testFile)

	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         time.Hour,
		MaxSize:     100,
		FilePath:    testFile,
		Compression: false,
	}

	// Create cache and add data
	c1 := cache.New(cfg)
	c1.Set("persistent", qos.Classification{
		Protocol:   "persistent",
		Class:      qos.EF,
		Confidence: 0.9,
		Source:     "ai",
	})

	// Save to file
	err := c1.Save()
	require.NoError(t, err)

	// Create new cache and load from file
	c2 := cache.New(cfg)
	err = c2.Load()
	require.NoError(t, err)

	// Verify data was loaded
	retrieved, found := c2.Get("persistent")
	assert.True(t, found)
	assert.Equal(t, "persistent", retrieved.Protocol)
	assert.Equal(t, qos.EF, retrieved.Class)
	assert.Equal(t, 0.9, retrieved.Confidence)
	assert.Equal(t, "ai", retrieved.Source)
}

func TestCacheCompression(t *testing.T) {
	testFile := "test_compression.json.gz"
	defer os.Remove(testFile)

	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         time.Hour,
		MaxSize:     100,
		FilePath:    testFile,
		Compression: true,
	}

	// Create cache and add data
	c1 := cache.New(cfg)
	c1.Set("compressed", qos.Classification{
		Protocol: "compressed",
		Class:    qos.EF,
		Source:   "ai",
	})

	// Save to compressed file
	err := c1.Save()
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(testFile)
	require.NoError(t, err)

	// Create new cache and load from compressed file
	c2 := cache.New(cfg)
	err = c2.Load()
	require.NoError(t, err)

	// Verify data was loaded
	retrieved, found := c2.Get("compressed")
	assert.True(t, found)
	assert.Equal(t, "compressed", retrieved.Protocol)
	assert.Equal(t, qos.EF, retrieved.Class)
}

func TestCacheBackup(t *testing.T) {
	testFile := "test_backup.json"
	backupFile := "test_backup_backup.json"
	defer func() {
		os.Remove(testFile)
		os.Remove(backupFile)
	}()

	cfg := &config.CacheConfig{
		Enabled:     true,
		TTL:         time.Hour,
		MaxSize:     100,
		FilePath:    testFile,
		BackupPath:  backupFile,
		Compression: false,
	}

	// Create initial cache file
	c1 := cache.New(cfg)
	c1.Set("original", qos.Classification{Protocol: "original", Class: qos.EF})
	err := c1.Save()
	require.NoError(t, err)

	// Create new cache and save again (should create backup)
	c2 := cache.New(cfg)
	c2.Set("new", qos.Classification{Protocol: "new", Class: qos.AF41})
	err = c2.Save()
	require.NoError(t, err)

	// Verify backup file exists
	_, err = os.Stat(backupFile)
	require.NoError(t, err)
}
