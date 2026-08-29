// Package config loads the tier caps.
//
// Caps are configuration, never compile-time constants: raising CIRCLE from 5
// to 500 is an env change and nothing else. This is already done for you - do
// not reintroduce a hardcoded cap in the enforcement path.
package config

import (
	"os"
	"strconv"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

// Load reads caps from the environment, falling back to the brief's defaults.
func Load() capacity.Caps {
	return capacity.Caps{
		Budget: intEnv("CAP_BUDGET", 8),
		PerTier: map[capacity.Tier]int{
			capacity.Partner: intEnv("CAP_PARTNER", 1),
			capacity.Crew:    intEnv("CAP_CREW", 3),
			capacity.Circle:  intEnv("CAP_CIRCLE", 5),
		},
	}
}

// MongoURI points at the local replica set from docker compose.
func MongoURI() string {
	return strEnv("MONGO_URI", "mongodb://localhost:27117/?replicaSet=rs0&directConnection=true")
}

func Port() string { return strEnv("PORT", "8080") }

func strEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func intEnv(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
