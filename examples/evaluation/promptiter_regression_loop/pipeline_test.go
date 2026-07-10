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
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	astructure "trpc.group/trpc-go/trpc-agent-go/agent/structure"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/aggregator"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/backwarder"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/optimizer"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestRunFakePipelineEndToEnd(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	outputDir := t.TempDir()
	result, err := runFakePipeline(context.Background(), RunConfig{
		Mode:       fakeMode,
		DataDir:    "./data",
		OutputDir:  outputDir,
		PromptPath: "./config/baseline_prompt.txt",
		ConfigPath: "./config/promptiter.json",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Run)
	require.Len(t, result.Run.Rounds, 1)
	require.NotNil(t, result.Run.Rounds[0].Acceptance)
	require.True(t, result.Run.Rounds[0].Acceptance.Accepted)
	require.Greater(t, result.Report.Candidate.Validation.OverallScore, result.Report.Baseline.Validation.OverallScore)
	require.FileExists(t, result.ReportJSONPath)
	require.FileExists(t, result.ReportMarkdownPath)
	require.Contains(t, result.ModelObservations.ToolDescriptions, initialToolDescription)
	require.Contains(t, result.ModelObservations.ToolDescriptions, optimizedToolDescription)
}

func TestPromptReadHashAndNeutralDefaultPrompt(t *testing.T) {
	defaultPrompt, _, err := readPrompt("./config/baseline_prompt.txt")
	require.NoError(t, err)
	defaultLower := strings.ToLower(defaultPrompt)
	require.NotContains(t, defaultLower, "flight")
	require.NotContains(t, defaultLower, "record")
	require.NotContains(t, defaultLower, "lookup")
	customPrompt := filepath.Join(t.TempDir(), "prompt.txt")
	require.NoError(t, os.WriteFile(customPrompt, []byte("You are a helpful assistant. CUSTOM_TRACE_TOKEN\n"), 0o644))
	_, defaultHash, err := readPrompt("./config/baseline_prompt.txt")
	require.NoError(t, err)
	_, customHash, err := readPrompt(customPrompt)
	require.NoError(t, err)
	require.NotEqual(t, defaultHash, customHash)
	result, err := runFakePipeline(context.Background(), RunConfig{
		Mode:       fakeMode,
		DataDir:    "./data",
		OutputDir:  t.TempDir(),
		PromptPath: customPrompt,
		ConfigPath: "./config/promptiter.json",
	})
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(result.ModelObservations.Instructions, func(instruction string) bool {
		return strings.Contains(instruction, "CUSTOM_TRACE_TOKEN")
	}))
}

func TestFakeModelIntentUsesOnlyUserMessagesAndToolDescription(t *testing.T) {
	fake := newFakeModel()
	noRecord := fake.generate(&model.Request{
		Messages: []model.Message{model.NewUserMessage("Expected invocation would ask for status, but this user text has no record id.")},
		Tools:    toolMap(optimizedToolDescription),
	})
	require.Empty(t, noRecord.Choices[0].Message.ToolCalls)
	initialDescription := fake.generate(&model.Request{
		Messages: []model.Message{model.NewUserMessage("What is the status for TR123?")},
		Tools:    toolMap(initialToolDescription),
	})
	require.Empty(t, initialDescription.Choices[0].Message.ToolCalls)
	optimizedDescription := fake.generate(&model.Request{
		Messages: []model.Message{model.NewUserMessage("What is the status for TR123?")},
		Tools:    toolMap(optimizedToolDescription),
	})
	require.Len(t, optimizedDescription.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, "lookup_record", optimizedDescription.Choices[0].Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"query":"TR123"}`, string(optimizedDescription.Choices[0].Message.ToolCalls[0].Function.Arguments))
}

func TestFakeWorkersCoverCurrentPromptIterAPIs(t *testing.T) {
	target := defaultTargetSurfaceID()
	back := &fakeBackwarder{targetSurfaceID: target}
	empty, err := back.Backward(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, empty.Gradients)
	nonTarget, err := back.Backward(context.Background(), &backwarder.Request{
		EvalSetID:                 trainEvalSetID,
		EvalCaseID:                "case",
		StepID:                    "step",
		AllowedGradientSurfaceIDs: []string{"candidate#tool.other"},
	})
	require.NoError(t, err)
	require.Empty(t, nonTarget.Gradients)
	targeted, err := back.Backward(context.Background(), &backwarder.Request{
		EvalSetID:                 trainEvalSetID,
		EvalCaseID:                "case",
		StepID:                    "step",
		AllowedGradientSurfaceIDs: []string{target},
	})
	require.NoError(t, err)
	require.Len(t, targeted.Gradients, 1)
	require.Equal(t, promptiter.LossSeverityP1, targeted.Gradients[0].Severity)
	require.Equal(t, trainEvalSetID, targeted.Gradients[0].EvalSetID)
	require.Equal(t, "case", targeted.Gradients[0].EvalCaseID)
	require.Equal(t, "step", targeted.Gradients[0].StepID)
	agg := &fakeAggregator{}
	aggregated, err := agg.Aggregate(context.Background(), &aggregator.Request{
		SurfaceID: target,
		NodeID:    candidateAgentName,
		Type:      astructure.SurfaceTypeTool,
		Gradients: targeted.Gradients,
	})
	require.NoError(t, err)
	require.NotNil(t, aggregated.Gradient)
	require.Equal(t, target, aggregated.Gradient.SurfaceID)
	require.Equal(t, candidateAgentName, aggregated.Gradient.NodeID)
	require.Equal(t, astructure.SurfaceTypeTool, aggregated.Gradient.Type)
	require.Len(t, aggregated.Gradient.Gradients, 1)
	opt := &fakeOptimizer{description: optimizedToolDescription}
	patch, err := opt.Optimize(context.Background(), &optimizer.Request{
		Surface: &astructure.Surface{
			SurfaceID: target,
			NodeID:    candidateAgentName,
			Type:      astructure.SurfaceTypeTool,
			Value: astructure.SurfaceValue{
				Tools: []astructure.ToolRef{{ID: "lookup_record", Description: initialToolDescription}},
			},
		},
		Gradient: aggregated.Gradient,
	})
	require.NoError(t, err)
	require.NotNil(t, patch.Patch)
	require.Equal(t, target, patch.Patch.SurfaceID)
	require.Equal(t, "lookup_record", patch.Patch.Value.Tools[0].ID)
	require.Equal(t, optimizedToolDescription, patch.Patch.Value.Tools[0].Description)
}

func TestReportSchema(t *testing.T) {
	result, err := runFakePipeline(context.Background(), RunConfig{
		Mode:       fakeMode,
		DataDir:    "./data",
		OutputDir:  t.TempDir(),
		PromptPath: "./config/baseline_prompt.txt",
		ConfigPath: "./config/promptiter.json",
	})
	require.NoError(t, err)
	report := result.Report
	require.NotNil(t, report.Baseline.Train)
	require.NotNil(t, report.Baseline.Validation)
	require.Nil(t, report.Candidate.Train)
	require.NotNil(t, report.Candidate.Validation)
	require.NotEmpty(t, report.Rounds)
	require.NotNil(t, report.Rounds[0].Train)
	require.NotNil(t, report.Rounds[0].Validation)
	require.ElementsMatch(t, phase1Pending, report.Phase1Pending)
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "trainScore")
	require.NotContains(t, string(raw), "validationScore")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "baseline")
	require.Contains(t, decoded, "candidate")
	require.Contains(t, decoded, "rounds")
	require.Contains(t, decoded, "phase1Pending")
}

func toolMap(description string) map[string]tool.Tool {
	return map[string]tool.Tool{
		"lookup_record": toolForTest{description: description},
	}
}

type toolForTest struct {
	description string
}

func (t toolForTest) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "lookup_record",
		Description: t.description,
	}
}
