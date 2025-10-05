package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type streamToolInfo struct {
	defaultToolInfo
	ops *mcpPolicies
}

func (c *streamToolInfo) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "stream",
			Title: aids.New("Get Stream text"),
		},
		Description: aids.New("Get Stream text"),
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"text": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"Description": aids.New("The stream text"),
				},
			},
			Required: []string{"text"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           aids.New("Get Stream text"),
			ReadOnlyHint:    aids.New(true),
			DestructiveHint: aids.New(false),
			IdempotentHint:  aids.New(true),
			OpenWorldHint:   aids.New(false),
		},
		Meta: mcp.Meta{"sensitive": "true"},
	}
}

// This type block defines the tool-specific tool call resource types
type (
	streamToolCallResult struct {
		Text []string `json:"text"`
	}
)

func (c *streamToolInfo) Create(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes, pm toolcall.PhaseMgr) bool {
	tc.Result = aids.MustMarshal(streamToolCallResult{Text: []string{}})
	tc.Status = aids.New(mcp.StatusRunning)
	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfNoneMatch: svrcore.ETagAnyPtr}); se != nil {
		return r.WriteServerError(se, nil, nil)
	}
	if se := pm.StartPhase(ctx, tc); se != nil {
		return r.WriteServerError(se, nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *streamToolInfo) Get(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

// ProcessPhase advanced the tool call's current phase to its next phase.
// Return nil to have the updated tc persisted to the tool call Store.
func (c *streamToolInfo) ProcessPhase(_ context.Context, _ toolcall.PhaseProcessor, tc *toolcall.Resource) {
	time.Sleep(10 * time.Second)                                  // Simulate doing work
	result := aids.MustUnmarshal[streamToolCallResult](tc.Result) // Update the result
	result.Text = append(result.Text, text[len(result.Text)])
	tc.Result = aids.MustMarshal(result)
	if len(result.Text) == len(text) {
		tc.Status, tc.Phase = aids.New(mcp.StatusSuccess), nil
	}
	se := c.ops.store.Put(context.TODO(), tc, svrcore.AccessConditions{IfMatch: tc.ETag})
	aids.Assert(se == nil, fmt.Errorf("failed to put tool call resource: %w", se))
}

var text = []string{`
Artificial Intelligence (AI) refers to computer systems designed to perform tasks that typically require
human intelligence, such as learning, reasoning, problem-solving, and decision-making. Modern AI
encompasses various approaches, from rule-based systems to machine learning models that improve through
experience and training on large datasets. Machine learning, particularly deep learning using artificial
neural networks, has driven recent breakthroughs by allowing computers to identify complex patterns in
data and generalize to handle new information without being explicitly programmed for every task.
`,

	`
AI applications have become increasingly integrated into daily life and industry. From recommendation 
systems on streaming platforms to virtual assistants, autonomous vehicles, and medical diagnostic tools,
AI powers countless services we use regularly. In business, it enables fraud detection, predictive
maintenance, search engines, and translation services. The technology has even shown remarkable 
capabilities in creative domains, generating art, writing, and music while excelling at tasks like
image recognition and natural language processing.
`,

	`
Despite rapid advancement, AI faces significant challenges and limitations. Current systems typically
excel in narrow domains but lack the general intelligence and contextual understanding humans possess
naturally. Ongoing concerns include bias in AI systems, potential job displacement, privacy issues,
and ethical implications of autonomous decision-making. As the field progresses, researchers and
policymakers are working to ensure AI development remains safe, reliable, and beneficial to 
society while addressing these complex challenges.
`,
}
