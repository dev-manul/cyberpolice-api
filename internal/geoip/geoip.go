package geoip

import (
	"fmt"
	"net"
	"strings"

	"github.com/oschwald/maxminddb-golang"

	"cyberpolice-api/internal/config"
)

type Location struct {
	Country string
	City    string
}

type Resolver struct {
	db *maxminddb.Reader
}

type mmdbCity struct {
	Country struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

func NewResolver(cfg config.Config) (*Resolver, error) {
	if strings.TrimSpace(cfg.GeoIPDBPath) == "" {
		return &Resolver{}, nil
	}
	db, err := maxminddb.Open(cfg.GeoIPDBPath)
	if err != nil {
		return nil, fmt.Errorf("open geoip db: %w", err)
	}
	return &Resolver{db: db}, nil
}

func (r *Resolver) Lookup(ipStr string) (Location, bool) {
	if r == nil || r.db == nil {
		return Location{}, false
	}
	parsed := net.ParseIP(strings.TrimSpace(ipStr))
	if parsed == nil {
		return Location{}, false
	}

	var record mmdbCity
	if err := r.db.Lookup(parsed, &record); err != nil {
		return Location{}, false
	}

	loc := Location{
		Country: record.Country.Names["en"],
		City:    record.City.Names["en"],
	}
	if loc.Country == "" && loc.City == "" {
		return Location{}, false
	}
	return loc, true
}
