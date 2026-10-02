package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"scriberr/internal/transcription/interfaces"
	"scriberr/pkg/logger"
)

// ModelRegistry manages all available model adapters with auto-discovery
type ModelRegistry struct {
	mu                    sync.RWMutex
	transcriptionAdapters map[string]interfaces.TranscriptionAdapter
	diarizationAdapters   map[string]interfaces.DiarizationAdapter
	compositeAdapters     map[string]interfaces.CompositeAdapter
	capabilities          map[string]interfaces.ModelCapabilities
	initialized           bool
	// preparedModels records models that an explicit selection prepared
	// successfully. Failed models are deliberately absent so a later call
	// retries them.
	preparedModels map[string]bool
}

// Global registry instance
var globalRegistry *ModelRegistry
var registryOnce sync.Once

// GetRegistry returns the global model registry instance
func GetRegistry() *ModelRegistry {
	registryOnce.Do(func() {
		globalRegistry = &ModelRegistry{
			transcriptionAdapters: make(map[string]interfaces.TranscriptionAdapter),
			diarizationAdapters:   make(map[string]interfaces.DiarizationAdapter),
			compositeAdapters:     make(map[string]interfaces.CompositeAdapter),
			capabilities:          make(map[string]interfaces.ModelCapabilities),
			preparedModels:        make(map[string]bool),
		}
	})
	return globalRegistry
}

// RegisterTranscriptionAdapter registers a transcription model adapter
func RegisterTranscriptionAdapter(modelID string, adapter interfaces.TranscriptionAdapter) {
	registry := GetRegistry()
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.transcriptionAdapters[modelID] = adapter
	registry.capabilities[modelID] = adapter.GetCapabilities()

	logger.Debug("Registered transcription adapter",
		"model_id", modelID,
		"family", adapter.GetCapabilities().ModelFamily,
		"display_name", adapter.GetCapabilities().DisplayName)
}

// RegisterDiarizationAdapter registers a diarization model adapter
func RegisterDiarizationAdapter(modelID string, adapter interfaces.DiarizationAdapter) {
	registry := GetRegistry()
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.diarizationAdapters[modelID] = adapter
	registry.capabilities[modelID] = adapter.GetCapabilities()

	logger.Debug("Registered diarization adapter",
		"model_id", modelID,
		"family", adapter.GetCapabilities().ModelFamily,
		"display_name", adapter.GetCapabilities().DisplayName)
}

// RegisterCompositeAdapter registers a composite (transcription + diarization) adapter
func RegisterCompositeAdapter(modelID string, adapter interfaces.CompositeAdapter) {
	registry := GetRegistry()
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.compositeAdapters[modelID] = adapter
	registry.capabilities[modelID] = adapter.GetCapabilities()

	logger.Debug("Registered composite adapter",
		"model_id", modelID,
		"family", adapter.GetCapabilities().ModelFamily,
		"display_name", adapter.GetCapabilities().DisplayName)
}

// GetTranscriptionAdapter retrieves a transcription adapter by ID
func (r *ModelRegistry) GetTranscriptionAdapter(modelID string) (interfaces.TranscriptionAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if adapter, exists := r.transcriptionAdapters[modelID]; exists {
		return adapter, nil
	}

	// Check if it's available as a composite adapter
	if adapter, exists := r.compositeAdapters[modelID]; exists {
		return adapter, nil
	}

	return nil, fmt.Errorf("transcription adapter not found: %s", modelID)
}

// GetDiarizationAdapter retrieves a diarization adapter by ID
func (r *ModelRegistry) GetDiarizationAdapter(modelID string) (interfaces.DiarizationAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if adapter, exists := r.diarizationAdapters[modelID]; exists {
		return adapter, nil
	}

	// Check if it's available as a composite adapter
	if adapter, exists := r.compositeAdapters[modelID]; exists {
		return adapter, nil
	}

	return nil, fmt.Errorf("diarization adapter not found: %s", modelID)
}

// GetCompositeAdapter retrieves a composite adapter by ID
func (r *ModelRegistry) GetCompositeAdapter(modelID string) (interfaces.CompositeAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if adapter, exists := r.compositeAdapters[modelID]; exists {
		return adapter, nil
	}

	return nil, fmt.Errorf("composite adapter not found: %s", modelID)
}

// GetCapabilities returns the capabilities of a model
func (r *ModelRegistry) GetCapabilities(modelID string) (interfaces.ModelCapabilities, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if capabilities, exists := r.capabilities[modelID]; exists {
		return capabilities, nil
	}

	return interfaces.ModelCapabilities{}, fmt.Errorf("model not found: %s", modelID)
}

// GetAllCapabilities returns capabilities for all registered models
func (r *ModelRegistry) GetAllCapabilities() map[string]interfaces.ModelCapabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a copy to avoid concurrent access issues
	result := make(map[string]interfaces.ModelCapabilities)
	for id, cap := range r.capabilities {
		result[id] = cap
	}
	return result
}

// GetTranscriptionModels returns all available transcription model IDs
func (r *ModelRegistry) GetTranscriptionModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var models []string
	for id := range r.transcriptionAdapters {
		models = append(models, id)
	}
	for id := range r.compositeAdapters {
		models = append(models, id)
	}

	sort.Strings(models)
	return models
}

// GetDiarizationModels returns all available diarization model IDs
func (r *ModelRegistry) GetDiarizationModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var models []string
	for id := range r.diarizationAdapters {
		models = append(models, id)
	}
	for id := range r.compositeAdapters {
		models = append(models, id)
	}

	sort.Strings(models)
	return models
}

// ModelScore represents a model's suitability score for given requirements
type ModelScore struct {
	ModelID string
	Score   float64
	Reasons []string
}

// SelectBestTranscriptionModel finds the best transcription model for given requirements
func (r *ModelRegistry) SelectBestTranscriptionModel(requirements interfaces.ModelRequirements) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []ModelScore

	// Score transcription adapters
	for modelID, adapter := range r.transcriptionAdapters {
		if score, reasons := r.scoreModel(adapter.GetCapabilities(), requirements); score > 0 {
			candidates = append(candidates, ModelScore{
				ModelID: modelID,
				Score:   score,
				Reasons: reasons,
			})
		}
	}

	// Score composite adapters
	for modelID, adapter := range r.compositeAdapters {
		if score, reasons := r.scoreModel(adapter.GetCapabilities(), requirements); score > 0 {
			candidates = append(candidates, ModelScore{
				ModelID: modelID,
				Score:   score,
				Reasons: reasons,
			})
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no suitable transcription model found for requirements: %+v", requirements)
	}

	// Sort by score (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	bestModel := candidates[0]
	logger.Info("Selected best transcription model",
		"model_id", bestModel.ModelID,
		"score", bestModel.Score,
		"reasons", strings.Join(bestModel.Reasons, ", "))

	return bestModel.ModelID, nil
}

// SelectBestDiarizationModel finds the best diarization model for given requirements
func (r *ModelRegistry) SelectBestDiarizationModel(requirements interfaces.ModelRequirements) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []ModelScore

	// Score diarization adapters
	for modelID, adapter := range r.diarizationAdapters {
		if score, reasons := r.scoreModel(adapter.GetCapabilities(), requirements); score > 0 {
			candidates = append(candidates, ModelScore{
				ModelID: modelID,
				Score:   score,
				Reasons: reasons,
			})
		}
	}

	// Score composite adapters
	for modelID, adapter := range r.compositeAdapters {
		if score, reasons := r.scoreModel(adapter.GetCapabilities(), requirements); score > 0 {
			candidates = append(candidates, ModelScore{
				ModelID: modelID,
				Score:   score,
				Reasons: reasons,
			})
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no suitable diarization model found for requirements: %+v", requirements)
	}

	// Sort by score (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	bestModel := candidates[0]
	logger.Info("Selected best diarization model",
		"model_id", bestModel.ModelID,
		"score", bestModel.Score,
		"reasons", strings.Join(bestModel.Reasons, ", "))

	return bestModel.ModelID, nil
}

// scoreModel calculates how well a model matches the requirements
//
//nolint:gocyclo // Scoring logic involves many factors
func (r *ModelRegistry) scoreModel(capabilities interfaces.ModelCapabilities, requirements interfaces.ModelRequirements) (float64, []string) {
	score := 0.0
	var reasons []string

	// Language support (critical)
	if requirements.Language != "" {
		languageSupported := false
		for _, lang := range capabilities.SupportedLanguages {
			if lang == requirements.Language || lang == "auto" || lang == "*" {
				languageSupported = true
				break
			}
		}
		if !languageSupported {
			return 0, []string{"language not supported"}
		}
		score += 20
		reasons = append(reasons, "language supported")
	}

	// Required features (high priority)
	for _, feature := range requirements.Features {
		if supported, exists := capabilities.Features[feature]; exists && supported {
			score += 15
			reasons = append(reasons, fmt.Sprintf("supports %s", feature))
		} else {
			score -= 10
			reasons = append(reasons, fmt.Sprintf("missing %s", feature))
		}
	}

	// Memory requirements
	if requirements.MaxMemoryMB > 0 && capabilities.MemoryRequirement > requirements.MaxMemoryMB {
		score -= 20
		reasons = append(reasons, "exceeds memory limit")
	} else if requirements.MaxMemoryMB > 0 {
		score += 5
		reasons = append(reasons, "within memory limit")
	}

	// GPU requirements
	if requirements.RequireGPU != nil {
		if *requirements.RequireGPU && !capabilities.RequiresGPU {
			score -= 15
			reasons = append(reasons, "GPU required but not used")
		} else if !*requirements.RequireGPU && capabilities.RequiresGPU {
			score -= 10
			reasons = append(reasons, "GPU not preferred but required")
		} else {
			score += 10
			reasons = append(reasons, "GPU preference matched")
		}
	}

	// Preferred family
	if requirements.PreferredFamily != nil && capabilities.ModelFamily == *requirements.PreferredFamily {
		score += 15
		reasons = append(reasons, "preferred family")
	}

	// Quality preference
	switch requirements.Quality {
	case "fast":
		if strings.Contains(strings.ToLower(capabilities.ModelID), "fast") ||
			strings.Contains(strings.ToLower(capabilities.ModelID), "tiny") ||
			strings.Contains(strings.ToLower(capabilities.ModelID), "small") {
			score += 10
			reasons = append(reasons, "optimized for speed")
		}
	case "best":
		if strings.Contains(strings.ToLower(capabilities.ModelID), "large") ||
			strings.Contains(strings.ToLower(capabilities.ModelID), "xl") ||
			strings.Contains(strings.ToLower(capabilities.ModelID), "turbo") {
			score += 10
			reasons = append(reasons, "optimized for quality")
		}
	case "good":
		if strings.Contains(strings.ToLower(capabilities.ModelID), "medium") ||
			strings.Contains(strings.ToLower(capabilities.ModelID), "base") {
			score += 10
			reasons = append(reasons, "balanced quality/speed")
		}
	}

	// Apply constraints
	for key, value := range requirements.Constraints {
		if metaValue, exists := capabilities.Metadata[key]; exists && metaValue == value {
			score += 5
			reasons = append(reasons, fmt.Sprintf("constraint %s=%s met", key, value))
		}
	}

	return score, reasons
}

// InitializeModels ensures all registered models are ready to use (parallel)
func (r *ModelRegistry) InitializeModels(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	logger.Info("Initializing registered models in parallel...")

	var wg sync.WaitGroup
	initErrors := make(chan error, len(r.transcriptionAdapters)+len(r.diarizationAdapters)+len(r.compositeAdapters))

	// Helper function to initialize an adapter
	initAdapter := func(id string, adapter interface {
		PrepareEnvironment(context.Context) error
	}, typeName string) {
		defer wg.Done()
		logger.Debug(fmt.Sprintf("Initializing %s model", typeName), "model_id", id)
		if err := adapter.PrepareEnvironment(ctx); err != nil {
			logger.Error(fmt.Sprintf("Failed to initialize %s model", typeName),
				"model_id", id, "error", err)
			initErrors <- fmt.Errorf("%s model %s: %w", typeName, id, err)
		} else {
			logger.Info(fmt.Sprintf("%s model initialized", typeName), "model_id", id)
		}
	}

	// Initialize transcription adapters
	for modelID, adapter := range r.transcriptionAdapters {
		wg.Add(1)
		go initAdapter(modelID, adapter, "transcription")
	}

	// Initialize diarization adapters
	for modelID, adapter := range r.diarizationAdapters {
		wg.Add(1)
		go initAdapter(modelID, adapter, "diarization")
	}

	// Initialize composite adapters
	for modelID, adapter := range r.compositeAdapters {
		wg.Add(1)
		go initAdapter(modelID, adapter, "composite")
	}

	// Wait for all initializations to complete
	wg.Wait()
	close(initErrors)

	// Collect any errors (but don't fail completely)
	var errorList []error
	for err := range initErrors {
		errorList = append(errorList, err)
	}

	if len(errorList) > 0 {
		logger.Warn("Some models failed to initialize", "error_count", len(errorList))
		for _, err := range errorList {
			logger.Warn("Model initialization error", "error", err)
		}
	}

	r.initialized = true
	logger.Info("Model initialization completed")
	return nil
}

// startupAdapter is the minimal adapter surface used to prepare a model
// environment at startup.
type startupAdapter interface {
	PrepareEnvironment(context.Context) error
}

// startupSelection is one resolved entry of an explicit startup selection
type startupSelection struct {
	id       string
	typeName string
	adapter  startupAdapter
}

// InitializeSelectedModels prepares only the named models and reports failures.
//
// InitializeModels prepares every registered adapter and downgrades preparation
// failures to warnings, which is useful for a "make everything available" start
// but pulls in dependencies the operator may not want. This entry point is for
// an explicit selection instead: the whole list is validated before any adapter
// is touched, so an unknown or empty model ID fails without side effects, and
// preparation errors are returned rather than swallowed.
//
// Only models that prepared successfully are remembered, so a later call still
// retries a failed model and still prepares a different selection. The legacy
// initialize-all flag is intentionally left untouched because a subset was
// prepared.
func (r *ModelRegistry) InitializeSelectedModels(ctx context.Context, modelIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	selected, err := r.resolveStartupSelection(modelIDs)
	if err != nil {
		return err
	}

	if r.preparedModels == nil {
		r.preparedModels = make(map[string]bool)
	}

	pending := make([]startupSelection, 0, len(selected))
	for _, sel := range selected {
		if r.preparedModels[sel.id] {
			logger.Debug("Selected model already prepared, skipping", "model_id", sel.id)
			continue
		}
		pending = append(pending, sel)
	}

	if len(pending) == 0 {
		logger.Info("All selected models already prepared", "model_count", len(selected))
		return nil
	}

	// Prepared sequentially: selections are small, and this keeps logs and
	// aggregated errors deterministic.
	var prepErrors []error
	for _, sel := range pending {
		logger.Info("Preparing selected model", "model_id", sel.id, "type", sel.typeName)
		if err := sel.adapter.PrepareEnvironment(ctx); err != nil {
			logger.Error("Failed to prepare selected model",
				"model_id", sel.id, "type", sel.typeName, "error", err)
			prepErrors = append(prepErrors, fmt.Errorf("%s model %s: %w", sel.typeName, sel.id, err))
			continue
		}
		r.preparedModels[sel.id] = true
		logger.Info("Selected model initialized", "model_id", sel.id, "type", sel.typeName)
	}

	if len(prepErrors) > 0 {
		return fmt.Errorf("failed to prepare selected models: %w", errors.Join(prepErrors...))
	}

	logger.Info("Selected model initialization completed", "model_count", len(selected))
	return nil
}

// resolveStartupSelection validates an explicit startup selection and maps each
// ID to its adapter. Nothing is prepared here: validation must complete before
// any environment side effects. Callers must hold r.mu.
func (r *ModelRegistry) resolveStartupSelection(modelIDs []string) ([]startupSelection, error) {
	if len(modelIDs) == 0 {
		return nil, fmt.Errorf("no models selected for initialization")
	}

	var (
		selected []startupSelection
		unknown  []string
	)
	seen := make(map[string]bool, len(modelIDs))

	for _, rawID := range modelIDs {
		modelID := strings.TrimSpace(rawID)
		if modelID == "" {
			return nil, fmt.Errorf("selected model list contains an empty model id")
		}
		if seen[modelID] {
			continue
		}
		seen[modelID] = true

		if adapter, exists := r.transcriptionAdapters[modelID]; exists {
			selected = append(selected, startupSelection{id: modelID, typeName: "transcription", adapter: adapter})
			continue
		}
		if adapter, exists := r.diarizationAdapters[modelID]; exists {
			selected = append(selected, startupSelection{id: modelID, typeName: "diarization", adapter: adapter})
			continue
		}
		if adapter, exists := r.compositeAdapters[modelID]; exists {
			selected = append(selected, startupSelection{id: modelID, typeName: "composite", adapter: adapter})
			continue
		}

		unknown = append(unknown, modelID)
	}

	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown selected model id(s): %s (registered: %s)",
			strings.Join(unknown, ", "), strings.Join(r.registeredModelIDsLocked(), ", "))
	}

	return selected, nil
}

// registeredModelIDsLocked returns every registered model ID, sorted.
// Callers must hold r.mu.
func (r *ModelRegistry) registeredModelIDsLocked() []string {
	unique := make(map[string]bool, len(r.transcriptionAdapters)+len(r.diarizationAdapters)+len(r.compositeAdapters))
	for id := range r.transcriptionAdapters {
		unique[id] = true
	}
	for id := range r.diarizationAdapters {
		unique[id] = true
	}
	for id := range r.compositeAdapters {
		unique[id] = true
	}

	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GetModelStatus returns the status of all registered models
func (r *ModelRegistry) GetModelStatus(ctx context.Context) map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]bool)

	// Check transcription adapters
	for modelID, adapter := range r.transcriptionAdapters {
		status[modelID] = adapter.IsReady(ctx)
	}

	// Check diarization adapters
	for modelID, adapter := range r.diarizationAdapters {
		status[modelID] = adapter.IsReady(ctx)
	}

	// Check composite adapters
	for modelID, adapter := range r.compositeAdapters {
		status[modelID] = adapter.IsReady(ctx)
	}

	return status
}

// GetEstimatedProcessingTime estimates processing time for given input and model
func (r *ModelRegistry) GetEstimatedProcessingTime(modelID string, input interfaces.AudioInput) (time.Duration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check transcription adapters
	if adapter, exists := r.transcriptionAdapters[modelID]; exists {
		return adapter.GetEstimatedProcessingTime(input), nil
	}

	// Check diarization adapters
	if adapter, exists := r.diarizationAdapters[modelID]; exists {
		return adapter.GetEstimatedProcessingTime(input), nil
	}

	// Check composite adapters
	if adapter, exists := r.compositeAdapters[modelID]; exists {
		return adapter.GetEstimatedProcessingTime(input), nil
	}

	return 0, fmt.Errorf("model not found: %s", modelID)
}

// ValidateModelParameters validates parameters for a specific model
func (r *ModelRegistry) ValidateModelParameters(modelID string, params map[string]interface{}) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check transcription adapters
	if adapter, exists := r.transcriptionAdapters[modelID]; exists {
		return adapter.ValidateParameters(params)
	}

	// Check diarization adapters
	if adapter, exists := r.diarizationAdapters[modelID]; exists {
		return adapter.ValidateParameters(params)
	}

	// Check composite adapters
	if adapter, exists := r.compositeAdapters[modelID]; exists {
		return adapter.ValidateParameters(params)
	}

	return fmt.Errorf("model not found: %s", modelID)
}

// GetParameterSchema returns the parameter schema for a specific model
func (r *ModelRegistry) GetParameterSchema(modelID string) ([]interfaces.ParameterSchema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check transcription adapters
	if adapter, exists := r.transcriptionAdapters[modelID]; exists {
		return adapter.GetParameterSchema(), nil
	}

	// Check diarization adapters
	if adapter, exists := r.diarizationAdapters[modelID]; exists {
		return adapter.GetParameterSchema(), nil
	}

	// Check composite adapters
	if adapter, exists := r.compositeAdapters[modelID]; exists {
		return adapter.GetParameterSchema(), nil
	}

	return nil, fmt.Errorf("model not found: %s", modelID)
}

// Test helper functions

// ClearRegistry clears all registered adapters (for testing only)
func ClearRegistry() {
	registry := GetRegistry()
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.transcriptionAdapters = make(map[string]interfaces.TranscriptionAdapter)
	registry.diarizationAdapters = make(map[string]interfaces.DiarizationAdapter)
	registry.compositeAdapters = make(map[string]interfaces.CompositeAdapter)
	registry.capabilities = make(map[string]interfaces.ModelCapabilities)
	registry.preparedModels = make(map[string]bool)
	registry.initialized = false
}

// GetTranscriptionAdapters returns all registered transcription adapters (for testing)
func GetTranscriptionAdapters() map[string]interfaces.TranscriptionAdapter {
	registry := GetRegistry()
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	result := make(map[string]interfaces.TranscriptionAdapter)
	for id, adapter := range registry.transcriptionAdapters {
		result[id] = adapter
	}
	return result
}

// GetDiarizationAdapters returns all registered diarization adapters (for testing)
func GetDiarizationAdapters() map[string]interfaces.DiarizationAdapter {
	registry := GetRegistry()
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	result := make(map[string]interfaces.DiarizationAdapter)
	for id, adapter := range registry.diarizationAdapters {
		result[id] = adapter
	}
	return result
}
