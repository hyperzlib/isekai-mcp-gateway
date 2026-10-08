package config

import (
	"sync/atomic"
)

var current atomic.Pointer[Config]

func GetConfig() *Config { return current.Load() }

func Publish(c *Config) { current.Store(c) }
