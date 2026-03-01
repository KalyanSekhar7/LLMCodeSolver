package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"tools/internal/tools"
)

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "orchestration-tools",
		Version: "1.0.0",
	}, nil)

	ts := tools.AllTools()
	ts.RegisterAll(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}
