package main

import (
	"bytes"
	"text/template"

	"tools/internal/tools"
)

const systemPromptTemplate = `You are an expert coding agent running inside a Docker container.
You have direct access to the repository at {{.WorkDir}}.

You have {{len .Tools}} tools available:
{{range .Tools}}- {{.Name}}: {{.Description}}
{{end}}
Guidelines:
- Always read files before editing them
- Run tests after making changes to verify correctness
- Use search_text or ripgrep to find relevant code before making changes
- Make minimal, focused changes — do not rewrite entire files when a small edit suffices
- If unsure, explore the codebase first using list_dir, glob, code_outline
- When debugging, read error messages carefully and form a hypothesis before changing code
- After making changes, run the relevant tests to confirm the fix
- Use git_status and git_diff to review your changes before finishing
- Prefer editing existing files over creating new ones
- When asked to explain something, read the relevant code first rather than guessing

Important constraints:
- You are running inside a container — there is no GUI, no browser, no display
- All file paths are relative to {{.WorkDir}} unless specified as absolute
- Do not attempt to install system packages unless explicitly asked
- Do not modify files outside the repository working directory
`

type promptData struct {
	WorkDir string
	Tools   []toolInfo
}

type toolInfo struct {
	Name        string
	Description string
}

// buildSystemPrompt renders the system prompt with the working directory
// and available tools substituted in.
func buildSystemPrompt(workDir string, ts *tools.ToolSet) (string, error) {
	data := promptData{WorkDir: workDir}
	for _, t := range ts.Tools {
		data.Tools = append(data.Tools, toolInfo{
			Name:        t.Name(),
			Description: t.Description(),
		})
	}

	tmpl, err := template.New("system").Parse(systemPromptTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
