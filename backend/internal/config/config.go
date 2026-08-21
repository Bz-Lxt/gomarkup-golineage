// Package config 集中加载与校验运行时配置。
//
// 所有配置均来自环境变量，缺省值面向本地 docker compose 环境。
// 加载阶段即完成边界校验，避免非法配置在运行期以难以定位的形式爆发。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 应用运行时配置。
type Config struct {
	// HTTP
	HTTPAddr        string
	CORSAllowOrigin []string

	// 数据库
	DBDSN            string
	DBMaxConns       int32
	DBConnectTimeout time.Duration

	// 日志
	LogLevel string

	// 种子数据
	SeedOnEmpty  bool
	SeedScenario string

	// 图算法安全上限
	GraphMaxDepth    int
	GraphMaxPaths    int
	GraphMaxNodes    int
	GraphQueryTimout time.Duration

	// 事件溯源
	SnapshotInterval int
}

// 合法的种子场景取值。
const (
	ScenarioAsset = "asset" // IT 资产血缘
	ScenarioFraud = "fraud" // 金融反欺诈人际网络
	ScenarioBoth  = "both"
	ScenarioNone  = "none"
)

// Load 从环境变量读取配置并执行校验。
func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		DBDSN:            env("DB_DSN", "postgres://golineage:golineage_dev_pwd@localhost:5432/golineage?sslmode=disable"),
		LogLevel:         env("LOG_LEVEL", "info"),
		SeedScenario:     strings.ToLower(env("SEED_SCENARIO", ScenarioBoth)),
		DBMaxConns:       int32(envInt("DB_MAX_CONNS", 10)),
		DBConnectTimeout: envDuration("DB_CONNECT_TIMEOUT", 30*time.Second),
		SeedOnEmpty:      envBool("SEED_ON_EMPTY", true),
		GraphMaxDepth:    envInt("GRAPH_MAX_DEPTH", 10),
		GraphMaxPaths:    envInt("GRAPH_MAX_PATHS", 1000),
		GraphMaxNodes:    envInt("GRAPH_MAX_NODES", 20000),
		GraphQueryTimout: envDuration("GRAPH_QUERY_TIMEOUT", 10*time.Second),
		SnapshotInterval: envInt("SNAPSHOT_INTERVAL", 500),
	}

	for _, o := range strings.Split(env("CORS_ALLOW_ORIGINS", "http://localhost:27430"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSAllowOrigin = append(c.CORSAllowOrigin, o)
		}
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("config: HTTP_ADDR 不能为空")
	}
	if c.DBDSN == "" {
		return fmt.Errorf("config: DB_DSN 不能为空")
	}
	if c.DBMaxConns <= 0 {
		return fmt.Errorf("config: DB_MAX_CONNS 必须为正数，当前 %d", c.DBMaxConns)
	}
	switch c.SeedScenario {
	case ScenarioAsset, ScenarioFraud, ScenarioBoth, ScenarioNone:
	default:
		return fmt.Errorf("config: SEED_SCENARIO 非法值 %q（可选 asset/fraud/both/none）", c.SeedScenario)
	}
	if c.GraphMaxDepth <= 0 || c.GraphMaxDepth > 64 {
		return fmt.Errorf("config: GRAPH_MAX_DEPTH 需在 1..64 之间，当前 %d", c.GraphMaxDepth)
	}
	if c.GraphMaxPaths <= 0 {
		return fmt.Errorf("config: GRAPH_MAX_PATHS 必须为正数，当前 %d", c.GraphMaxPaths)
	}
	if c.GraphMaxNodes <= 0 {
		return fmt.Errorf("config: GRAPH_MAX_NODES 必须为正数，当前 %d", c.GraphMaxNodes)
	}
	if c.GraphQueryTimout <= 0 {
		return fmt.Errorf("config: GRAPH_QUERY_TIMEOUT 必须为正数")
	}
	if c.SnapshotInterval < 0 {
		return fmt.Errorf("config: SNAPSHOT_INTERVAL 不能为负数，当前 %d", c.SnapshotInterval)
	}
	return nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
