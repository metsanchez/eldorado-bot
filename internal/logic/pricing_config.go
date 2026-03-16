package logic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"eldorado-bot/internal/logger"
)

// PriceConfig holds user-configurable pricing. Loaded from pricing.json, hot-reloaded at runtime.
type PriceConfig struct {
	DivisionPrice     map[string]float64 `json:"division_price"`
	NetWinPrice       map[string]float64 `json:"net_win_price"`
	NetWinOverride    map[string]float64 `json:"net_win_override"`
	PointPrice        map[string]float64 `json:"point_price"`
	RRDiscountPer25   map[string]float64 `json:"rr_discount_per_25"`
	HoursPerDivision  map[string]float64 `json:"hours_per_division"`
}

// PriceManager loads and hot-reloads pricing config.
type PriceManager struct {
	path     string
	config   *PriceConfig
	mu       sync.RWMutex
	log      *logger.Logger
	lastLoad time.Time
}

// NewPriceManager creates a manager that loads from the given path.
func NewPriceManager(path string, log *logger.Logger) *PriceManager {
	pm := &PriceManager{path: path, log: log}
	pm.load()
	return pm
}

// Get returns a copy of the current config (safe for concurrent read).
func (pm *PriceManager) Get() *PriceConfig {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.config == nil {
		return nil
	}
	return pm.config
}

// Load reads pricing.json from disk. Safe to call concurrently.
func (pm *PriceManager) Load() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.loadLocked()
}

func (pm *PriceManager) load() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.loadLocked()
}

func (pm *PriceManager) loadLocked() bool {
	absPath := pm.path
	if !filepath.IsAbs(absPath) {
		if wd, err := os.Getwd(); err == nil {
			absPath = filepath.Join(wd, pm.path)
		}
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if pm.config == nil {
			pm.log.Errorf("pricing: cannot read %s: %v (using defaults)", pm.path, err)
			pm.config = defaultPriceConfig()
		}
		return false
	}

	var cfg PriceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		pm.log.Errorf("pricing: invalid JSON in %s: %v", pm.path, err)
		return false
	}

	// Merge with defaults for missing keys
	cfg = mergeWithDefaults(cfg)
	pm.config = &cfg
	pm.lastLoad = time.Now()
	pm.log.Infof("pricing: loaded from %s (%d division prices)", pm.path, len(cfg.DivisionPrice))
	return true
}

func defaultPriceConfig() *PriceConfig {
	return &PriceConfig{
		DivisionPrice: map[string]float64{
			"Iron I": 3, "Iron II": 3, "Iron III": 3,
			"Bronze I": 3, "Bronze II": 3, "Bronze III": 3,
			"Silver I": 3, "Silver II": 4, "Silver III": 4,
			"Gold I": 4, "Gold II": 4, "Gold III": 4,
			"Platinum I": 5, "Platinum II": 5, "Platinum III": 8,
			"Diamond I": 10, "Diamond II": 10, "Diamond III": 11,
			"Ascendant I": 15, "Ascendant II": 17, "Ascendant III": 19,
			"Immortal I": 33, "Immortal II": 65, "Immortal III": 50,
		},
		NetWinPrice: map[string]float64{
			"iron": 5, "bronze": 5, "silver": 5, "gold": 5,
			"platinum": 5, "diamond": 5, "ascendant": 8, "immortal": 11,
		},
		NetWinOverride: map[string]float64{
			"Immortal I": 11, "Ascendant I": 8, "Ascendant II": 8, "Ascendant III": 8,
		},
		PointPrice: map[string]float64{
			"iron": 1, "bronze": 1, "silver": 1, "gold": 2,
			"platinum": 3, "diamond": 4, "ascendant": 5, "immortal": 10,
		},
		RRDiscountPer25: map[string]float64{
			"diamond": 1.5, "ascendant": 2, "immortal": 5,
		},
		HoursPerDivision: map[string]float64{
			"iron": 4, "bronze": 4, "silver": 4, "gold": 4, "platinum": 4,
			"diamond": 4, "ascendant": 7, "immortal": 24,
		},
	}
}

func mergeWithDefaults(cfg PriceConfig) PriceConfig {
	def := defaultPriceConfig()
	if cfg.DivisionPrice == nil {
		cfg.DivisionPrice = def.DivisionPrice
	} else {
		for k, v := range def.DivisionPrice {
			if _, ok := cfg.DivisionPrice[k]; !ok {
				cfg.DivisionPrice[k] = v
			}
		}
	}
	if cfg.NetWinPrice == nil {
		cfg.NetWinPrice = def.NetWinPrice
	} else {
		for k, v := range def.NetWinPrice {
			if _, ok := cfg.NetWinPrice[k]; !ok {
				cfg.NetWinPrice[k] = v
			}
		}
	}
	if cfg.NetWinOverride == nil {
		cfg.NetWinOverride = def.NetWinOverride
	}
	if cfg.PointPrice == nil {
		cfg.PointPrice = def.PointPrice
	} else {
		for k, v := range def.PointPrice {
			if _, ok := cfg.PointPrice[k]; !ok {
				cfg.PointPrice[k] = v
			}
		}
	}
	if cfg.RRDiscountPer25 == nil {
		cfg.RRDiscountPer25 = def.RRDiscountPer25
	}
	if cfg.HoursPerDivision == nil {
		cfg.HoursPerDivision = def.HoursPerDivision
	}
	return cfg
}
