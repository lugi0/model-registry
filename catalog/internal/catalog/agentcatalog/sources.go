package agentcatalog

import (
	"sync"

	"github.com/kubeflow/hub/catalog/internal/catalog/basecatalog"
	model "github.com/kubeflow/hub/catalog/pkg/openapi"
)

type agentOriginEntry struct {
	origin  string
	sources map[string]basecatalog.PluginSource
}

// AgentSourceCollection manages agent catalog sources from multiple origins with priority-based merging.
type AgentSourceCollection struct {
	mu      sync.RWMutex
	entries []agentOriginEntry
}

func NewAgentSourceCollection(originOrder ...string) *AgentSourceCollection {
	entries := make([]agentOriginEntry, len(originOrder))
	for i, origin := range originOrder {
		entries[i] = agentOriginEntry{origin: origin, sources: nil}
	}
	return &AgentSourceCollection{
		entries: entries,
	}
}

func (sc *AgentSourceCollection) Merge(origin string, sources map[string]basecatalog.PluginSource) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for i := range sc.entries {
		if sc.entries[i].origin == origin {
			sc.entries[i].sources = sources
			return nil
		}
	}

	sc.entries = append(sc.entries, agentOriginEntry{origin: origin, sources: sources})
	return nil
}

func (sc *AgentSourceCollection) merged() map[string]basecatalog.PluginSource {
	result := map[string]basecatalog.PluginSource{}

	for _, entry := range sc.entries {
		for id, source := range entry.sources {
			if existing, ok := result[id]; ok {
				result[id] = mergeAgentSources(existing, source)
			} else {
				result[id] = source
			}
		}
	}

	for id, source := range result {
		result[id] = applyAgentDefaults(source)
	}

	return result
}

func mergeAgentSources(base, override basecatalog.PluginSource) basecatalog.PluginSource {
	result := base

	common := basecatalog.MergeCommonSourceFields(
		basecatalog.CommonSourceFields{Name: base.Name, Enabled: base.Enabled, Labels: base.Labels, Type: base.Type, Properties: base.Properties, Origin: base.Origin, AssetType: base.AssetType},
		basecatalog.CommonSourceFields{Name: override.Name, Enabled: override.Enabled, Labels: override.Labels, Type: override.Type, Properties: override.Properties, Origin: override.Origin, AssetType: override.AssetType},
	)
	result.Name = common.Name
	result.Enabled = common.Enabled
	result.Labels = common.Labels
	result.Type = common.Type
	result.Properties = common.Properties
	result.Origin = common.Origin
	result.AssetType = common.AssetType

	return result
}

func applyAgentDefaults(source basecatalog.PluginSource) basecatalog.PluginSource {
	if source.Enabled == nil {
		source.Enabled = new(true)
	}
	if source.Labels == nil {
		source.Labels = []string{}
	}
	if source.AssetType == nil {
		source.AssetType = model.CATALOGASSETTYPE_AGENTS.Ptr()
	}
	return source
}

func (sc *AgentSourceCollection) AllSources() map[string]basecatalog.PluginSource {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.merged()
}
