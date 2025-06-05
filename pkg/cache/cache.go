package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePattern(ctx context.Context, pattern string) error
	Exists(ctx context.Context, key string) (bool, error)
	GetMetrics() CacheMetrics
	Warm(ctx context.Context, keys []WarmupKey) error
	InvalidateByTags(ctx context.Context, tags []string) error
}

type WarmupKey struct {
	Key   string
	Value interface{}
	TTL   time.Duration
	Tags  []string
}

type CacheMetrics struct {
	Hits        int64
	Misses      int64
	Errors      int64
	TotalOps    int64
	HitRate     float64
	AverageTime time.Duration
}

type RedisCache struct {
	client    *redis.Client
	keyPrefix string
	metrics   *cacheMetrics
	tagIndex  sync.Map
}

type cacheMetrics struct {
	hits     prometheus.Counter
	misses   prometheus.Counter
	errors   prometheus.Counter
	duration prometheus.Histogram
}

type Config struct {
	Addr         string        `yaml:"addr" validate:"required"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	KeyPrefix    string        `yaml:"key_prefix"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
}

func NewRedisCache(config Config, serviceName string) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	metrics := &cacheMetrics{
		hits: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "cache_hits_total",
			Help:        "Total number of cache hits",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		misses: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "cache_misses_total",
			Help:        "Total number of cache misses",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		errors: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "cache_errors_total",
			Help:        "Total number of cache errors",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		duration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "cache_operation_duration_seconds",
			Help:        "Duration of cache operations",
			ConstLabels: prometheus.Labels{"service": serviceName},
			Buckets:     prometheus.DefBuckets,
		}),
	}

	return &RedisCache{
		client:    client,
		keyPrefix: config.KeyPrefix,
		metrics:   metrics,
	}, nil
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	start := time.Now()
	defer func() {
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)

	data, err := c.client.Get(ctx, fullKey).Result()
	if err != nil {
		if err == redis.Nil {
			c.metrics.misses.Inc()
			return ErrCacheKeyNotFound
		}
		c.metrics.errors.Inc()
		return fmt.Errorf("cache get error: %w", err)
	}

	c.metrics.hits.Inc()

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		c.metrics.errors.Inc()
		return fmt.Errorf("cache unmarshal error: %w", err)
	}

	return nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	start := time.Now()
	defer func() {
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	data, err := json.Marshal(value)
	if err != nil {
		c.metrics.errors.Inc()
		return fmt.Errorf("cache marshal error: %w", err)
	}

	fullKey := c.buildKey(key)

	if err := c.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		c.metrics.errors.Inc()
		return fmt.Errorf("cache set error: %w", err)
	}

	return nil
}

func (c *RedisCache) SetWithTags(ctx context.Context, key string, value interface{}, ttl time.Duration, tags []string) error {
	if err := c.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	for _, tag := range tags {
		if keysInterface, ok := c.tagIndex.Load(tag); ok {
			if keys, ok := keysInterface.(map[string]bool); ok {
				keys[key] = true
				c.tagIndex.Store(tag, keys)
			}
		} else {
			keys := make(map[string]bool)
			keys[key] = true
			c.tagIndex.Store(tag, keys)
		}
	}

	return nil
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	start := time.Now()
	defer func() {
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)

	if err := c.client.Del(ctx, fullKey).Err(); err != nil {
		c.metrics.errors.Inc()
		return fmt.Errorf("cache delete error: %w", err)
	}

	return nil
}

func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	start := time.Now()
	defer func() {
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	fullPattern := c.buildKey(pattern)

	keys, err := c.client.Keys(ctx, fullPattern).Result()
	if err != nil {
		c.metrics.errors.Inc()
		return fmt.Errorf("cache keys scan error: %w", err)
	}

	if len(keys) > 0 {
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			c.metrics.errors.Inc()
			return fmt.Errorf("cache pattern delete error: %w", err)
		}
	}

	return nil
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()
	defer func() {
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)

	exists, err := c.client.Exists(ctx, fullKey).Result()
	if err != nil {
		c.metrics.errors.Inc()
		return false, fmt.Errorf("cache exists error: %w", err)
	}

	return exists > 0, nil
}

func (c *RedisCache) GetMetrics() CacheMetrics {
	return CacheMetrics{}
}

func (c *RedisCache) Warm(ctx context.Context, keys []WarmupKey) error {
	for _, warmupKey := range keys {
		if err := c.SetWithTags(ctx, warmupKey.Key, warmupKey.Value, warmupKey.TTL, warmupKey.Tags); err != nil {
			return fmt.Errorf("cache warmup error for key %s: %w", warmupKey.Key, err)
		}
	}
	return nil
}

func (c *RedisCache) InvalidateByTags(ctx context.Context, tags []string) error {
	var keysToDelete []string

	for _, tag := range tags {
		if keysInterface, ok := c.tagIndex.Load(tag); ok {
			if keys, ok := keysInterface.(map[string]bool); ok {
				for key := range keys {
					keysToDelete = append(keysToDelete, key)
				}
				c.tagIndex.Delete(tag)
			}
		}
	}

	for _, key := range keysToDelete {
		if err := c.Delete(ctx, key); err != nil {
			return fmt.Errorf("cache invalidation error for key %s: %w", key, err)
		}
	}

	return nil
}

func (c *RedisCache) buildKey(key string) string {
	if c.keyPrefix == "" {
		return key
	}
	return c.keyPrefix + ":" + key
}

var (
	ErrCacheKeyNotFound = fmt.Errorf("cache key not found")
)

type CacheKeyBuilder struct {
	prefix string
}

func NewCacheKeyBuilder(prefix string) *CacheKeyBuilder {
	return &CacheKeyBuilder{prefix: prefix}
}

func (b *CacheKeyBuilder) UserKey(userID string) string {
	return fmt.Sprintf("%s:user:%s", b.prefix, userID)
}

func (b *CacheKeyBuilder) TestKey(testID string) string {
	return fmt.Sprintf("%s:test:%s", b.prefix, testID)
}

func (b *CacheKeyBuilder) UserTestsKey(userID string) string {
	return fmt.Sprintf("%s:user:%s:tests", b.prefix, userID)
}

func (b *CacheKeyBuilder) LeaderboardKey(category string) string {
	return fmt.Sprintf("%s:leaderboard:%s", b.prefix, category)
}

func (b *CacheKeyBuilder) StatsKey(userID string, period string) string {
	return fmt.Sprintf("%s:stats:%s:%s", b.prefix, userID, period)
}
