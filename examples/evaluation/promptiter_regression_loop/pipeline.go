//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	astructure "trpc.group/trpc-go/trpc-agent-go/agent/structure"
	"trpc.group/trpc-go/trpc-agent-go/evaluation"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evalresult"
	evalresultlocal "trpc.group/trpc-go/trpc-agent-go/evaluation/evalresult/local"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	evalsetlocal "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset/local"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/registry"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	metriclocal "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/local"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/aggregator"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/backwarder"
	promptiterengine "trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/engine"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/optimizer"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const (
	appName                  = "promptiter-regression-loop-app"
	candidateRunnerAppName   = "promptiter-regression-loop-candidate"
	candidateAgentName       = "candidate"
	exampleDirName           = "promptiter_regression_loop"
	trainEvalSetID           = "train"
	validationEvalSetID      = "validation"
	sharedMetricFileName     = "metrics.json"
	fakeMode                 = "fake"
	initialToolDescription   = "Look up a traveler loyalty-profile record."
	optimizedToolDescription = "Use lookup_record to query flight status, delay, departure time, and gate information."
)

// RunConfig contains CLI-configurable paths for the Phase 1 v2 demo.
type RunConfig struct {
	Mode       string
	DataDir    string
	OutputDir  string
	PromptPath string
	ConfigPath string
}

type promptIterFileConfig struct {
	MaxRounds    int      `json:"maxRounds"`
	MinScoreGain float64  `json:"minScoreGain"`
	TargetScore  *float64 `json:"targetScore,omitempty"`
}

type PipelineResult struct {
	Run                *promptiterengine.RunResult
	Report             *OptimizationReport
	ModelObservations  fakeModelObservations
	ReportJSONPath     string
	ReportMarkdownPath string
}

type promptIterRuntime struct {
	engine    promptiterengine.Engine
	evaluator evaluation.AgentEvaluator
	runner    runner.Runner
	model     *fakeModel
}

type sharedMetricLocator struct{}

func runFakePipeline(ctx context.Context, cfg RunConfig) (*PipelineResult, error) {
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = fakeMode
	}
	if cfg.Mode != fakeMode {
		return nil, fmt.Errorf("unsupported mode %q: Phase 1 v2 only supports -mode fake", cfg.Mode)
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "./output"
	}
	if cfg.PromptPath == "" {
		cfg.PromptPath = "./config/baseline_prompt.txt"
	}
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = "./config/promptiter.json"
	}
	cfg.DataDir = resolveExamplePath(cfg.DataDir)
	cfg.OutputDir = resolveExamplePath(cfg.OutputDir)
	cfg.PromptPath = resolveExamplePath(cfg.PromptPath)
	cfg.ConfigPath = resolveExamplePath(cfg.ConfigPath)
	promptText, promptHash, err := readPrompt(cfg.PromptPath)
	if err != nil {
		return nil, err
	}
	fileConfig, err := readPromptIterConfig(cfg.ConfigPath)
	if err != nil {
		return nil, err
	}
	targetSurfaceID := defaultTargetSurfaceID()
	runtime, err := buildFakeRuntime(ctx, cfg, promptText, targetSurfaceID)
	if err != nil {
		return nil, err
	}
	defer runtime.close()
	if err := ensureTargetSurface(ctx, runtime.engine, targetSurfaceID); err != nil {
		return nil, err
	}
	runResult, err := runtime.engine.Run(ctx, buildRunRequest(fileConfig, targetSurfaceID))
	if err != nil {
		return nil, fmt.Errorf("run promptiter: %w", err)
	}
	report, err := newOptimizationReport(runResult, ReportContext{
		Mode:             cfg.Mode,
		TargetSurfaceIDs: []string{targetSurfaceID},
		PromptPath:       cfg.PromptPath,
		PromptSHA256:     promptHash,
	})
	if err != nil {
		return nil, err
	}
	jsonPath, markdownPath, err := writeOptimizationReport(cfg.OutputDir, report)
	if err != nil {
		return nil, err
	}
	return &PipelineResult{
		Run:                runResult,
		Report:             report,
		ModelObservations:  runtime.model.observations(),
		ReportJSONPath:     jsonPath,
		ReportMarkdownPath: markdownPath,
	}, nil
}

func buildFakeRuntime(
	ctx context.Context,
	cfg RunConfig,
	promptText string,
	targetSurfaceID string,
) (*promptIterRuntime, error) {
	candidateModel := newFakeModel()
	candidateAgent, err := newCandidateAgent(candidateModel, promptText, initialToolDescription)
	if err != nil {
		return nil, err
	}
	candidateRunner := runner.NewRunner(candidateRunnerAppName, candidateAgent)
	evalSetManager := evalsetlocal.New(evalset.WithBaseDir(cfg.DataDir))
	metricManager := metriclocal.New(
		metric.WithBaseDir(cfg.DataDir),
		metric.WithLocator(sharedMetricLocator{}),
	)
	evalResultManager := evalresultlocal.New(evalresult.WithBaseDir(cfg.OutputDir))
	agentEvaluator, err := evaluation.New(
		appName,
		candidateRunner,
		evaluation.WithEvalSetManager(evalSetManager),
		evaluation.WithMetricManager(metricManager),
		evaluation.WithEvalResultManager(evalResultManager),
		evaluation.WithRegistry(registry.New()),
	)
	if err != nil {
		candidateRunner.Close()
		return nil, fmt.Errorf("create evaluator: %w", err)
	}
	engineInstance, err := promptiterengine.New(
		ctx,
		promptiterengine.WithAgent(candidateAgent),
		promptiterengine.WithAgentEvaluator(agentEvaluator),
		promptiterengine.WithBackwarder(&fakeBackwarder{targetSurfaceID: targetSurfaceID}),
		promptiterengine.WithAggregator(&fakeAggregator{}),
		promptiterengine.WithOptimizer(&fakeOptimizer{description: optimizedToolDescription}),
	)
	if err != nil {
		agentEvaluator.Close()
		candidateRunner.Close()
		return nil, fmt.Errorf("create promptiter engine: %w", err)
	}
	return &promptIterRuntime{
		engine:    engineInstance,
		evaluator: agentEvaluator,
		runner:    candidateRunner,
		model:     candidateModel,
	}, nil
}

func (r *promptIterRuntime) close() {
	if r == nil {
		return
	}
	if r.evaluator != nil {
		_ = r.evaluator.Close()
	}
	if r.runner != nil {
		r.runner.Close()
	}
}

func newCandidateAgent(m model.Model, instruction, lookupDescription string) (agent.Agent, error) {
	generationConfig := model.GenerationConfig{
		MaxTokens:   intPtr(1024),
		Temperature: floatPtr(0),
		Stream:      false,
	}
	return llmagent.New(
		candidateAgentName,
		llmagent.WithModel(m),
		llmagent.WithInstruction(instruction),
		llmagent.WithTools(newLookupTools(lookupDescription)),
		llmagent.WithGenerationConfig(generationConfig),
	), nil
}

func buildRunRequest(cfg promptIterFileConfig, targetSurfaceID string) *promptiterengine.RunRequest {
	maxRounds := cfg.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}
	return &promptiterengine.RunRequest{
		Train: []promptiterengine.EvalSetInput{
			{EvalSetID: trainEvalSetID},
		},
		Validation: []promptiterengine.EvalSetInput{
			{EvalSetID: validationEvalSetID},
		},
		// Phase 1 sets InitialProfile to nil. Under the current engine semantics,
		// round 1 InputProfile is the normalized initial profile, so result.Rounds[0].Train
		// is the baseline train evaluation used by the report.
		InitialProfile:   nil,
		Judge:            nil,
		MaxRounds:        maxRounds,
		TargetSurfaceIDs: []string{targetSurfaceID},
		AcceptancePolicy: promptiterengine.AcceptancePolicy{
			MinScoreGain: cfg.MinScoreGain,
		},
		StopPolicy: promptiterengine.StopPolicy{
			TargetScore: cfg.TargetScore,
		},
	}
}

func ensureTargetSurface(ctx context.Context, engine promptiterengine.Engine, targetSurfaceID string) error {
	snapshot, err := engine.Describe(ctx)
	if err != nil {
		return fmt.Errorf("describe promptiter engine: %w", err)
	}
	for _, surface := range snapshot.Surfaces {
		if surface.SurfaceID == targetSurfaceID {
			return nil
		}
	}
	return fmt.Errorf("target surface %q not found in promptiter structure", targetSurfaceID)
}

func defaultTargetSurfaceID() string {
	return astructure.SurfaceID(candidateAgentName, astructure.SurfaceTypeTool, "lookup_record")
}

func readPrompt(path string) (string, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read prompt %s: %w", path, err)
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return "", "", fmt.Errorf("prompt %s is empty", path)
	}
	sum := sha256.Sum256([]byte(text))
	return text, hex.EncodeToString(sum[:]), nil
}

func readPromptIterConfig(path string) (promptIterFileConfig, error) {
	cfg := promptIterFileConfig{
		MaxRounds:    1,
		MinScoreGain: 0.01,
	}
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return cfg, errors.New("promptiter config is empty")
	}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config %s: %w", path, err)
	}
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 1
	}
	return cfg, nil
}

func resolveExamplePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	alt := filepath.Join(exampleDirName, cleanRelativePath(path))
	if _, err := os.Stat(alt); err == nil {
		return alt
	}
	return path
}

func cleanRelativePath(path string) string {
	clean := filepath.Clean(path)
	if clean == "." {
		return ""
	}
	prefix := "." + string(filepath.Separator)
	return strings.TrimPrefix(clean, prefix)
}

func (sharedMetricLocator) Build(baseDir, appName, _ string) string {
	return filepath.Join(baseDir, appName, sharedMetricFileName)
}

func newLookupTools(description string) []tool.Tool {
	return []tool.Tool{
		function.NewFunctionTool(
			lookupRecord,
			function.WithName("lookup_record"),
			function.WithDescription(description),
		),
	}
}

type lookupRecordArgs struct {
	Query string `json:"query" jsonschema:"description=Record key to look up,required"`
}

type lookupRecordResult struct {
	RecordID           string `json:"recordId"`
	State              string `json:"state"`
	DelayMinutes       int    `json:"delayMinutes"`
	Gate               string `json:"gate,omitempty"`
	ScheduledDeparture string `json:"scheduledDeparture"`
	UpdatedDeparture   string `json:"updatedDeparture,omitempty"`
}

func lookupRecord(_ context.Context, args lookupRecordArgs) (lookupRecordResult, error) {
	record, ok := flightRecords()[strings.ToUpper(strings.TrimSpace(args.Query))]
	if !ok {
		return lookupRecordResult{
			RecordID: strings.TrimSpace(args.Query),
			State:    "unknown",
		}, nil
	}
	return record, nil
}

func flightRecords() map[string]lookupRecordResult {
	return map[string]lookupRecordResult{
		"TR123": {
			RecordID:           "TR123",
			State:              "delayed",
			DelayMinutes:       35,
			Gate:               "B12",
			ScheduledDeparture: "10:10",
			UpdatedDeparture:   "10:45",
		},
		"TR456": {
			RecordID:           "TR456",
			State:              "delayed",
			DelayMinutes:       15,
			Gate:               "A07",
			ScheduledDeparture: "12:30",
			UpdatedDeparture:   "12:45",
		},
		"TR789": {
			RecordID:           "TR789",
			State:              "cancelled",
			ScheduledDeparture: "18:00",
		},
		"TR654": {
			RecordID:           "TR654",
			State:              "boarding",
			Gate:               "D18",
			ScheduledDeparture: "16:05",
			UpdatedDeparture:   "16:05",
		},
	}
}

func intPtr(value int) *int {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

var (
	_ backwarder.Backwarder = (*fakeBackwarder)(nil)
	_ aggregator.Aggregator = (*fakeAggregator)(nil)
	_ optimizer.Optimizer   = (*fakeOptimizer)(nil)
	_ model.Model           = (*fakeModel)(nil)
	_ metric.Locator        = sharedMetricLocator{}
)
