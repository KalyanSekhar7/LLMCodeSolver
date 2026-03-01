package tools

import (
	"context"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Tool interface {
	Name() string
	Description() string
	InputType() any
	Execute(ctx context.Context, input []byte) ([]byte, error)
}

type ToolSet struct {
	Tools []Tool
}

func (ts *ToolSet) RegisterAll(server *mcp.Server) {
	for _, t := range ts.Tools {
		tool := t
		inputType := reflect.TypeOf(tool.InputType())
		schema, err := jsonschema.ForType(inputType, &jsonschema.ForOptions{})
		if err != nil {
			panic("failed to generate schema for tool " + tool.Name() + ": " + err.Error())
		}

		server.AddTool(&mcp.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: schema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := tool.Execute(ctx, req.Params.Arguments)
			if err != nil {
				var errRes mcp.CallToolResult
				errRes.IsError = true
				errRes.Content = []mcp.Content{&mcp.TextContent{Text: err.Error()}}
				return &errRes, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
			}, nil
		})
	}
}

func AllTools() *ToolSet {
	return &ToolSet{
		Tools: []Tool{
			// file tools
			&ReadFileTool{},
			&ReadManyFilesTool{},
			&WriteFileTool{},
			&AppendFileTool{},
			&DeleteFileTool{},
			&MoveFileTool{},
			&CopyFileTool{},
			&EditFileRangesTool{},
			&ApplyUnifiedDiffTool{},
			&FileStatTool{},
			&HashFileTool{},

			// git tools
			&GitStatusTool{},
			&GitDiffTool{},
			&GitDiffCachedTool{},
			&GitLogTool{},
			&GitShowTool{},
			&GitBlameTool{},
			&GitCreateBranchTool{},
			&GitSwitchBranchTool{},
			&GitCheckoutTool{},
			&GitAddTool{},
			&GitCommitTool{},
			&GitResetTool{},
			&GitRestoreTool{},
			&GitApplyTool{},
			&GitStashPushTool{},
			&GitStashPopTool{},
			&GitRemoteInfoTool{},

			// shell tools
			&RunCommandTool{},
			&RunScriptTool{},
			&WhichTool{},
			&SetEnvTool{},
			&GetEnvTool{},

			// search tools
			&SearchTextTool{},
			&SearchSymbolTool{},
			&RipgrepTool{},
			&CtagsGenerateTool{},
			&CtagsQueryTool{},
			&AstParseTool{},
			&CodeOutlineTool{},

			// workspace tools
			&WorkspaceInfoTool{},
			&ListDirTool{},
			&GlobTool{},
			&ProjectDetectTool{},
		},
	}
}
