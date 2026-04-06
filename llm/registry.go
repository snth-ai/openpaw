package llm

import (
	"fmt"
	"sync"
)

// ModelInfo describes an available model within a provider.
type ModelInfo struct {
	ID       string  // "grok-4.20-0309-non-reasoning"
	Display  string  // "Grok 4.20"
	PriceIn  float64 // $ per 1M input tokens
	PriceOut float64 // $ per 1M output tokens
}

// ProviderEntry describes a configured provider with its available models.
type ProviderEntry struct {
	Name        string      // "openrouter", "xai"
	Display     string      // "OpenRouter", "x.AI"
	Kind        ProviderType
	APIKey      string
	BaseURL     string
	Models      []ModelInfo
	DefaultModel string     // default model ID for this provider
}

// ProviderRegistry manages multiple providers and supports runtime switching.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]*ProviderEntry // keyed by Name
	active    struct {
		providerName string
		modelID      string
		provider     Provider
	}
	order []string // provider names in display order
}

// NewProviderRegistry creates an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]*ProviderEntry),
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(entry ProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[entry.Name] = &entry
	r.order = append(r.order, entry.Name)
}

// SetActive switches the active provider and model.
func (r *ProviderRegistry) SetActive(providerName, modelID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider: %s", providerName)
	}

	// Validate model exists
	if modelID == "" {
		modelID = entry.DefaultModel
	}
	found := false
	for _, m := range entry.Models {
		if m.ID == modelID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("model %q not found in provider %s", modelID, providerName)
	}

	// Create provider instance
	var provider Provider
	switch entry.Kind {
	case ProviderOpenRouter:
		provider = NewOpenRouter(entry.APIKey, modelID)
	case ProviderXAI:
		provider = NewXAI(entry.APIKey, modelID)
	case ProviderAnthropic:
		provider = NewAnthropic(entry.APIKey, modelID)
	default:
		provider = NewGenericOpenAI(entry.APIKey, modelID, entry.BaseURL, entry.Name)
	}

	r.active.providerName = providerName
	r.active.modelID = modelID
	r.active.provider = provider

	return nil
}

// Active returns the current active provider.
func (r *ProviderRegistry) Active() Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active.provider
}

// ActiveInfo returns the current provider name, model ID, and service name for logging.
func (r *ProviderRegistry) ActiveInfo() (providerName, modelID, serviceName string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry := r.providers[r.active.providerName]
	svc := r.active.providerName
	if entry != nil {
		svc = entry.Name
	}
	return r.active.providerName, r.active.modelID, svc
}

// Providers returns all registered providers in order.
func (r *ProviderRegistry) Providers() []ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []ProviderEntry
	for _, name := range r.order {
		if e, ok := r.providers[name]; ok {
			result = append(result, *e)
		}
	}
	return result
}

// GetProvider returns a specific provider entry.
func (r *ProviderRegistry) GetProvider(name string) (*ProviderEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.providers[name]
	return e, ok
}

// ChatStream delegates to the active provider (implements Provider interface).
func (r *ProviderRegistry) ChatStream(messages []Message, tools []map[string]any, onChunk func(string)) (*Response, error) {
	p := r.Active()
	if p == nil {
		return nil, fmt.Errorf("no active provider configured")
	}
	return p.ChatStream(messages, tools, onChunk)
}
