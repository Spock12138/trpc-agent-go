//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	promptiterengine "trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/engine"
)

var phase1Pending = []string{
	"final_gate",
	"validation_delta",
	"failure_attribution",
	"trace_smoke",
	"candidate_train_regression",
	"design_doc",
}

type ReportContext struct {
	Mode             string
	TargetSurfaceIDs []string
	PromptPath       string
	PromptSHA256     string
}

type OptimizationReport struct {
	Phase            string          `json:"phase"`
	Mode             string          `json:"mode"`
	SingleRound      bool            `json:"singleRound"`
	TargetSurfaceIDs []string        `json:"targetSurfaceIds"`
	PromptPath       string          `json:"promptPath"`
	PromptSHA256     string          `json:"promptSha256"`
	Baseline         ReportCandidate `json:"baseline"`
	Candidate        ReportCandidate `json:"candidate"`
	Rounds           []ReportRound   `json:"rounds"`
	Phase1Pending    []string        `json:"phase1Pending"`
}

type ReportCandidate struct {
	Train      *EvaluationSummary `json:"train"`
	Validation *EvaluationSummary `json:"validation"`
}

type ReportRound struct {
	Round            int                `json:"round"`
	Accepted         bool               `json:"accepted"`
	AcceptanceReason string             `json:"acceptanceReason,omitempty"`
	ScoreDelta       float64            `json:"scoreDelta"`
	Train            *EvaluationSummary `json:"train"`
	Validation       *EvaluationSummary `json:"validation"`
	Patches          []PatchSummary     `json:"patches"`
}

type PatchSummary struct {
	SurfaceID string `json:"surfaceId"`
	Reason    string `json:"reason"`
}

type EvaluationSummary struct {
	OverallScore float64          `json:"overallScore"`
	EvalSets     []EvalSetSummary `json:"evalSets"`
}

type EvalSetSummary struct {
	EvalSetID    string        `json:"evalSetId"`
	OverallScore float64       `json:"overallScore"`
	Cases        []CaseSummary `json:"cases"`
}

type CaseSummary struct {
	EvalCaseID string          `json:"evalCaseId"`
	Metrics    []MetricSummary `json:"metrics"`
}

type MetricSummary struct {
	MetricName string  `json:"metricName"`
	Score      float64 `json:"score"`
	Status     string  `json:"status"`
	Reason     string  `json:"reason,omitempty"`
}

func newOptimizationReport(result *promptiterengine.RunResult, ctx ReportContext) (*OptimizationReport, error) {
	if result == nil {
		return nil, errors.New("run result is nil")
	}
	if len(result.Rounds) == 0 {
		return nil, errors.New("run result has no rounds")
	}
	rounds := make([]ReportRound, 0, len(result.Rounds))
	for _, round := range result.Rounds {
		rounds = append(rounds, reportRound(round))
	}
	firstRound := result.Rounds[0]
	lastRound := result.Rounds[len(result.Rounds)-1]
	return &OptimizationReport{
		Phase:            "phase1v2",
		Mode:             ctx.Mode,
		SingleRound:      true,
		TargetSurfaceIDs: append([]string(nil), ctx.TargetSurfaceIDs...),
		PromptPath:       ctx.PromptPath,
		PromptSHA256:     ctx.PromptSHA256,
		Baseline: ReportCandidate{
			// See buildRunRequest: result.Rounds[0].Train is the baseline train
			// evaluation while InitialProfile remains nil for this Phase 1 run.
			Train:      evaluationSummary(firstRound.Train),
			Validation: evaluationSummary(result.BaselineValidation),
		},
		Candidate: ReportCandidate{
			Train:      nil,
			Validation: evaluationSummary(lastRound.Validation),
		},
		Rounds:        rounds,
		Phase1Pending: append([]string(nil), phase1Pending...),
	}, nil
}

func reportRound(round promptiterengine.RoundResult) ReportRound {
	out := ReportRound{
		Round:      round.Round,
		Train:      evaluationSummary(round.Train),
		Validation: evaluationSummary(round.Validation),
	}
	if round.Acceptance != nil {
		out.Accepted = round.Acceptance.Accepted
		out.AcceptanceReason = round.Acceptance.Reason
		out.ScoreDelta = round.Acceptance.ScoreDelta
	}
	if round.Patches != nil {
		out.Patches = make([]PatchSummary, 0, len(round.Patches.Patches))
		for _, patch := range round.Patches.Patches {
			out.Patches = append(out.Patches, PatchSummary{
				SurfaceID: patch.SurfaceID,
				Reason:    patch.Reason,
			})
		}
	}
	return out
}

func evaluationSummary(result *promptiterengine.EvaluationResult) *EvaluationSummary {
	if result == nil {
		return nil
	}
	out := &EvaluationSummary{
		OverallScore: result.OverallScore,
		EvalSets:     make([]EvalSetSummary, 0, len(result.EvalSets)),
	}
	for _, evalSet := range result.EvalSets {
		evalSetSummary := EvalSetSummary{
			EvalSetID:    evalSet.EvalSetID,
			OverallScore: evalSet.OverallScore,
			Cases:        make([]CaseSummary, 0, len(evalSet.Cases)),
		}
		for _, evalCase := range evalSet.Cases {
			caseSummary := CaseSummary{
				EvalCaseID: evalCase.EvalCaseID,
				Metrics:    make([]MetricSummary, 0, len(evalCase.Metrics)),
			}
			for _, metric := range evalCase.Metrics {
				caseSummary.Metrics = append(caseSummary.Metrics, MetricSummary{
					MetricName: metric.MetricName,
					Score:      metric.Score,
					Status:     string(metric.Status),
					Reason:     metric.Reason,
				})
			}
			evalSetSummary.Cases = append(evalSetSummary.Cases, caseSummary)
		}
		out.EvalSets = append(out.EvalSets, evalSetSummary)
	}
	return out
}

func writeOptimizationReport(outputDir string, report *OptimizationReport) (string, string, error) {
	if report == nil {
		return "", "", errors.New("report is nil")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create output dir %s: %w", outputDir, err)
	}
	jsonPath := filepath.Join(outputDir, "optimization_report.json")
	markdownPath := filepath.Join(outputDir, "optimization_report.md")
	jsonContent, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal report json: %w", err)
	}
	jsonContent = append(jsonContent, '\n')
	if err := os.WriteFile(jsonPath, jsonContent, 0o644); err != nil {
		return "", "", fmt.Errorf("write report json: %w", err)
	}
	if err := os.WriteFile(markdownPath, renderMarkdownReport(report), 0o644); err != nil {
		return "", "", fmt.Errorf("write report markdown: %w", err)
	}
	return jsonPath, markdownPath, nil
}

func renderMarkdownReport(report *OptimizationReport) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Phase 1 v2 PromptIter Regression Loop\n\n")
	fmt.Fprintf(&buf, "Mode: `%s`\n\n", report.Mode)
	fmt.Fprintf(&buf, "Single round: `%t`\n\n", report.SingleRound)
	fmt.Fprintf(&buf, "Target surface: `%s`\n\n", report.TargetSurfaceIDs[0])
	fmt.Fprintf(&buf, "Baseline validation overall score: `%.4f`\n\n", scoreOf(report.Baseline.Validation))
	fmt.Fprintf(&buf, "Candidate validation overall score: `%.4f`\n\n", scoreOf(report.Candidate.Validation))
	for _, round := range report.Rounds {
		fmt.Fprintf(&buf, "## Round %d\n\n", round.Round)
		fmt.Fprintf(&buf, "- Accepted: `%t`\n", round.Accepted)
		fmt.Fprintf(&buf, "- Score delta: `%.4f`\n", round.ScoreDelta)
		fmt.Fprintf(&buf, "- Reason: %s\n", round.AcceptanceReason)
	}
	fmt.Fprintf(&buf, "\n## Phase 1 Pending\n\n")
	for _, item := range report.Phase1Pending {
		fmt.Fprintf(&buf, "- `%s`\n", item)
	}
	return buf.Bytes()
}

func scoreOf(summary *EvaluationSummary) float64 {
	if summary == nil {
		return 0
	}
	return summary.OverallScore
}
