package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// JSON-RPC 2.0 types for MCP protocol

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolHandler func(args map[string]interface{}) (string, error)

// Server is a minimal MCP server over stdio.
type Server struct {
	name     string
	version  string
	tools    map[string]ToolDef
	handlers map[string]ToolHandler
}

func NewServer(name, version string) *Server {
	return &Server{
		name:     name,
		version:  version,
		tools:    make(map[string]ToolDef),
		handlers: make(map[string]ToolHandler),
	}
}

func (s *Server) AddTool(name, description string, schema interface{}, handler ToolHandler) {
	s.tools[name] = ToolDef{
		Name:        name,
		Description: description,
		InputSchema: schema,
	}
	s.handlers[name] = handler
}

// CallTool invokes a registered tool handler by name. Used by the HTTP server
// to reuse the same business logic without duplicating it.
func (s *Server) CallTool(name string, args map[string]interface{}) (string, error) {
	handler, ok := s.handlers[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return handler(args)
}

// HasTool checks if a tool is registered.
func (s *Server) HasTool(name string) bool {
	_, ok := s.handlers[name]
	return ok
}

// Run starts the MCP server on stdio.
func (s *Server) Run() {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Printf("Read error: %v", err)
			return
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handle(req)
		data, _ := json.Marshal(resp)
		fmt.Fprintf(writer, "%s\n", data)
	}
}

func (s *Server) handle(req Request) Response {
	switch req.Method {
	case "initialize":
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    s.name,
					"version": s.version,
				},
			},
		}

	case "notifications/initialized":
		// No response needed for notifications
		return Response{JSONRPC: "2.0"}

	case "tools/list":
		var toolList []ToolDef
		for _, t := range s.tools {
			toolList = append(toolList, t)
		}
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": toolList,
			},
		}

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: -32602, Message: "Invalid params"},
			}
		}

		handler, ok := s.handlers[params.Name]
		if !ok {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("Unknown tool: %s", params.Name)},
			}
		}

		result, err := handler(params.Arguments)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"content": []TextContent{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
					"isError": true,
				},
			}
		}

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []TextContent{{Type: "text", Text: result}},
			},
		}

	default:
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("Unknown method: %s", req.Method)},
		}
	}
}
