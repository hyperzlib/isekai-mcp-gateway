package outbound

import (
	"sync/atomic"

	"isekai-mcp-gateway/internal/config"
)

type Manager struct{ active atomic.Pointer[Bundle] }

func (m *Manager) Get() *Bundle { return m.active.Load() }

func (m *Manager) Publish(b *Bundle) { m.active.Store(b); config.Publish(b.Config) }
