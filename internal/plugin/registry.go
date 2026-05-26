// Package plugin implements the central registry, credential resolver, and
// action dispatcher for Warden's integration plugins. Plugins register
// themselves at process init() time by calling Register from their own
// init() function; the registry is treated as read-only thereafter.
package plugin

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aechrok/warden/internal/domain"
)

// Registry holds all registered plugins keyed by Plugin.ID().
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]domain.Plugin
}

// global is the process-wide registry consulted by Register, Get, and All.
// Tests that need an isolated registry should construct their own via
// NewRegistry rather than mutating the global.
var global = &Registry{plugins: make(map[string]domain.Plugin)}

// NewRegistry returns an empty, isolated Registry. Primarily useful in tests.
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]domain.Plugin)}
}

// Register adds a plugin to the global registry. Registering the same ID
// twice panics: plugin IDs must be unique, and a duplicate registration
// almost always indicates a programming error in a plugin's init().
func Register(p domain.Plugin) {
	if p == nil {
		panic("plugin.Register: nil plugin")
	}
	global.add(p)
}

// Get returns the plugin with the given ID and true, or nil and false if no
// plugin with that ID is registered.
func Get(id string) (domain.Plugin, bool) {
	return global.get(id)
}

// All returns every registered plugin in ID-sorted order. The slice is a
// snapshot and may be mutated by the caller without affecting the registry.
func All() []domain.Plugin {
	return global.all()
}

// Add registers a plugin on this Registry instance. See Register for the
// global equivalent.
func (r *Registry) Add(p domain.Plugin) { r.add(p) }

// Get returns the plugin with the given ID and true, or nil and false.
func (r *Registry) Get(id string) (domain.Plugin, bool) { return r.get(id) }

// All returns every plugin registered on this Registry in ID-sorted order.
func (r *Registry) All() []domain.Plugin { return r.all() }

func (r *Registry) add(p domain.Plugin) {
	id := p.ID()
	if id == "" {
		panic("plugin.Register: plugin returned empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.plugins[id]; dup {
		panic(fmt.Sprintf("plugin.Register: duplicate plugin id %q", id))
	}
	r.plugins[id] = p
}

func (r *Registry) get(id string) (domain.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

func (r *Registry) all() []domain.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}
