package main

import (
	"context"
	"log"

	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	mcpsetup "github.com/kleio-build/kleio-cli/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	apiClient := client.New(cfg.APIURL, cfg.APIKey, cfg.WorkspaceID)
	s := mcpsetup.NewServer(apiClient)

	if err := s.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
