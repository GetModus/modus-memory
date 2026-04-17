package mcp

import (
	"encoding/json"
	"testing"

	"github.com/GetModus/modus-memory/internal/vault"
)

func TestServerHandleInitializeToolsListAndCallMemoryCapture(t *testing.T) {
	v := vault.New(t.TempDir(), nil)
	srv := NewServer("homing", "0.6.0")
	RegisterMemoryTools(srv, v, true)

	initResp := srv.handle(Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	})
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}
	initResult, ok := initResp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("initialize result type = %T", initResp.Result)
	}
	serverInfo, ok := initResult["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("serverInfo type = %T", initResult["serverInfo"])
	}
	if got := serverInfo["name"]; got != "homing" {
		t.Fatalf("serverInfo.name = %v, want homing", got)
	}

	listResp := srv.handle(Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	})
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %v", listResp.Error)
	}
	listResult, ok := listResp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("tools/list result type = %T", listResp.Result)
	}
	toolsRaw, ok := listResult["tools"].([]ToolDef)
	if !ok {
		t.Fatalf("tools/list tools type = %T", listResult["tools"])
	}
	foundCapture := false
	for _, tool := range toolsRaw {
		if tool.Name == "memory_capture" {
			foundCapture = true
			break
		}
	}
	if !foundCapture {
		t.Fatal("expected memory_capture in tools/list")
	}

	params, err := json.Marshal(map[string]interface{}{
		"name": "memory_capture",
		"arguments": map[string]interface{}{
			"text":       "The General prefers local-first tools for long-running work.",
			"subject":    "General",
			"event_kind": "interaction",
			"policy":     "balanced",
			"facts": []map[string]string{
				{"subject": "General", "predicate": "prefers", "value": "local-first tools for long-running work"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	callResp := srv.handle(Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  params,
	})
	if callResp.Error != nil {
		t.Fatalf("tools/call error: %v", callResp.Error)
	}
	callResult, ok := callResp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("tools/call result type = %T", callResp.Result)
	}
	contentRaw, ok := callResult["content"].([]TextContent)
	if !ok {
		t.Fatalf("tools/call content type = %T", callResult["content"])
	}
	if len(contentRaw) != 1 {
		t.Fatalf("content length = %d, want 1", len(contentRaw))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(contentRaw[0].Text), &payload); err != nil {
		t.Fatalf("parse memory_capture payload: %v", err)
	}
	if got := payload["decision"]; got != "episode_and_facts" {
		t.Fatalf("decision = %v, want episode_and_facts", got)
	}
}
