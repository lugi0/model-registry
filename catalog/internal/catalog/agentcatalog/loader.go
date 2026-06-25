package agentcatalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/golang/glog"
	"github.com/kubeflow/hub/catalog/internal/catalog/basecatalog"
)

// AgentLoader handles loading agent data from YAML configuration files.
type AgentLoader struct {
	state basecatalog.LoaderState

	Sources  *AgentSourceCollection
	services Services

	closerMu sync.Mutex
	closer   func()
}

func (l *AgentLoader) setCloser(closer func()) {
	l.closerMu.Lock()
	defer l.closerMu.Unlock()
	if l.closer != nil {
		l.closer()
	}
	l.closer = closer
}

func NewAgentLoader(services Services, state basecatalog.LoaderState) *AgentLoader {
	paths := state.Paths()
	return &AgentLoader{
		state:    state,
		Sources:  NewAgentSourceCollection(paths...),
		services: services,
	}
}

func (l *AgentLoader) ParseAllConfigs() error {
	glog.Infof("Initializing %s loader - parsing configs", "agent")

	for _, path := range l.state.Paths() {
		if err := l.parseAndMerge(path); err != nil {
			return fmt.Errorf("failed to parse agent config %s: %w", path, err)
		}
	}

	glog.Infof("%s loader config parsing complete", "agent")
	return nil
}

func (l *AgentLoader) PerformLeaderOperations(ctx context.Context, allKnownSourceIDs mapset.Set[string]) error {
	glog.Infof("%s loader performing leader operations", "agent")

	ctx, cancel := context.WithCancel(ctx)
	l.setCloser(cancel)

	allSources := l.Sources.AllSources()

	for id, source := range allSources {
		if !source.IsEnabled() {
			basecatalog.SaveSourceStatus(l.services.CatalogSourceRepository, id, basecatalog.SourceStatusDisabled, "")
			continue
		}

		if source.Type != "yaml" {
			glog.Warningf("unknown %s provider type: %s", "agent", source.Type)
			basecatalog.SaveSourceStatus(l.services.CatalogSourceRepository, id, basecatalog.SourceStatusError, "unknown provider type: "+source.Type)
			continue
		}

		if err := l.loadFromYAML(ctx, id, source); err != nil {
			glog.Errorf("error loading %s from source %s: %v", "agent", id, err)
			basecatalog.SaveSourceStatus(l.services.CatalogSourceRepository, id, basecatalog.SourceStatusError, err.Error())
			continue
		}

		basecatalog.SaveSourceStatus(l.services.CatalogSourceRepository, id, basecatalog.SourceStatusAvailable, "")
	}

	glog.Infof("%s loader leader operations complete", "agent")
	return nil
}

func (l *AgentLoader) loadFromYAML(ctx context.Context, sourceID string, source basecatalog.PluginSource) error {
	yamlPath, err := resolveYAMLPath(source)
	if err != nil {
		return err
	}

	catalog, err := readYAMLAgentCatalog(yamlPath)
	if err != nil {
		return err
	}

	for _, ya := range catalog.Agents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		entity := yamlAgentToEntity(ya, sourceID)
		if _, err := l.services.AgentRepository.Save(entity); err != nil {
			glog.Errorf("error saving agent %s from source %s: %v", ya.Name, sourceID, err)
			continue
		}
	}

	return nil
}

func (l *AgentLoader) ReloadParsing() error {
	var errs []error
	for _, path := range l.state.Paths() {
		if err := l.parseAndMerge(path); err != nil {
			errs = append(errs, fmt.Errorf("unable to reload agent sources from %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func (l *AgentLoader) parseAndMerge(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %v", path, err)
	}

	config, err := basecatalog.ReadSourceConfig(path)
	if err != nil {
		return err
	}

	return l.updateSources(path, config)
}

func (l *AgentLoader) updateSources(path string, config *basecatalog.SourceConfig) error {
	sources := make(map[string]basecatalog.PluginSource, len(config.AgentCatalogs))

	for _, source := range config.AgentCatalogs {
		glog.Infof("reading agent catalog config type %s...", source.Type)
		if source.GetId() == "" {
			return fmt.Errorf("invalid agent source: missing id")
		}
		if _, exists := sources[source.GetId()]; exists {
			return fmt.Errorf("invalid agent source: duplicate id %s", source.GetId())
		}

		source.Origin = path
		sources[source.GetId()] = source
		glog.Infof("loaded agent source %s of type %s", source.GetId(), source.Type)
	}

	return l.Sources.Merge(path, sources)
}
