//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package replaytest

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

func TestBuildSnapshotNormalizesGeneratedEventFields(t *testing.T) {
	left := BuildSnapshot(replayTestSession("left", time.Unix(1, 0)), nil)
	right := BuildSnapshot(replayTestSession("right", time.Unix(2, 0)), nil)
	diffs := CompareSnapshots(
		"generated",
		"session-1",
		"left",
		"right",
		left,
		right,
		nil,
	)
	require.Empty(t, diffs)
}

func TestNormalizeStatePreservesJSONNumbersAndByteKinds(t *testing.T) {
	state := normalizeState(session.StateMap{
		"large":  []byte(`{"value":9007199254740993}`),
		"plain":  []byte("hello"),
		"quoted": []byte(`"hello"`),
		"nil":    nil,
	})
	large, ok := state["large"].(StateBytesSnapshot)
	require.True(t, ok)
	require.Equal(t, "json", large.Kind)
	require.Equal(t, json.Number("9007199254740993"), large.Value.(map[string]any)["value"])
	require.NotEqual(t, state["plain"], state["quoted"])
	require.Equal(t, StateBytesSnapshot{Kind: "nil"}, state["nil"])
}

func TestCompareSnapshotsAddsContextAndAppliesExplicitRule(t *testing.T) {
	left := BuildSnapshot(replayTestSession("same", time.Unix(1, 0)), nil)
	right := BuildSnapshot(replayTestSession("same", time.Unix(1, 0)), nil)
	right.Events[0]["author"] = "different"
	rules := []AllowedDiffRule{{
		Section: "events", Path: "$.events[0].author",
		BackendA: "left", BackendB: "right", Reason: "fixture drift",
	}}
	diffs := CompareSnapshots("case", "session-1", "left", "right", left, right, rules)
	require.Len(t, diffs, 1)
	require.True(t, diffs[0].Allowed)
	require.Equal(t, "fixture drift", diffs[0].Reason)
	require.Equal(t, 0, diffs[0].Context["event_index"])
	require.False(t, HasUnallowedDiffs(diffs))
}

func TestWriteReportUsesEmptyArrayForNilDiffs(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, WriteReport(&out, nil))
	require.Equal(t, "[]\n", out.String())
	require.Error(t, WriteReport(nil, nil))
}

func replayTestSession(generated string, timestamp time.Time) *session.Session {
	evt := event.Event{
		Response: &model.Response{
			ID:        "response-" + generated,
			Object:    model.ObjectTypeChatCompletion,
			Timestamp: timestamp,
			Done:      true,
			Choices: []model.Choice{{
				Index:   0,
				Message: model.NewAssistantMessage("same content"),
			}},
		},
		RequestID:    "request-1",
		InvocationID: "invocation-1",
		Author:       "agent",
		ID:           "event-" + generated,
		Timestamp:    timestamp,
		Branch:       "root",
		FilterKey:    "root",
		Version:      event.CurrentVersion,
	}
	return session.NewSession(
		"app",
		"user",
		"session-1",
		session.WithSessionEvents([]event.Event{evt}),
	)
}
