package txmanager

import (
	"errors"
	"fmt"
	"sync"
	"tcc/component"
)

type registryCenter struct {
	mu         sync.RWMutex
	components map[string]component.TCCComponent
}

func newRegistryCenter() *registryCenter {
	return &registryCenter{
		mu:         sync.RWMutex{},
		components: make(map[string]component.TCCComponent),
	}
}

func (r *registryCenter) register(component component.TCCComponent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.components[component.ID()]; ok {
		return errors.New("repeat component id")
	}
	r.components[component.ID()] = component
	return nil
}

func (r *registryCenter) getComponents(componentIDs ...string) ([]component.TCCComponent, error) {
	components := make([]component.TCCComponent, 0, len(componentIDs))

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, componentID := range componentIDs {
		component, ok := r.components[componentID]
		if !ok {
			return nil, fmt.Errorf("component id: %s not existed", componentID)
		}
		components = append(components, component)
	}
	return components, nil
}
