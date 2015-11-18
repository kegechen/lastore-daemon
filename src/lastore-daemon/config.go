package main

import (
	"fmt"
	log "github.com/cihub/seelog"
	"internal/system"
	"time"
)

const MinCheckInterval = time.Minute * 5

type Config struct {
	AutoCheckUpdates bool
	MirrorSource     string
	CheckInterval    time.Duration
	AppstoreRegion   string

	fpath string
}

func NewConfig(fpath string) *Config {
	r := Config{
		CheckInterval:    time.Minute * 180,
		MirrorSource:     system.DefaultMirror.Id,
		AutoCheckUpdates: true,
		fpath:            fpath,
	}

	err := system.DecodeJson(fpath, &r)
	if err != nil {
		log.Warnf("Can't load config file: %v\n", err)
	}

	if r.CheckInterval < MinCheckInterval {
		r.CheckInterval = MinCheckInterval
	}
	if r.MirrorSource == "" {
		r.MirrorSource = system.DefaultMirror.Id
	}

	return &r
}

func (c *Config) SetAutoCheckUpdates(enable bool) error {
	c.AutoCheckUpdates = enable
	return c.save()
}

func (c *Config) SetMirrorSource(id string) error {
	c.MirrorSource = id
	return c.save()
}

func (c *Config) SetAppstoreRegion(region string) error {
	if region != "mainland" && region != "international" {
		return fmt.Errorf("the region of %q is not supported", region)
	}
	c.AppstoreRegion = region
	return c.save()
}

func (c *Config) save() error {
	return system.EncodeJson(c.fpath, c)
}
