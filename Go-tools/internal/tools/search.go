package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// search_text — regex/plain text search across files
// ---------------------------------------------------------------------------

type SearchTextInput struct {
	Query         string `json:"query"`
	PathsGlob     string `json:"paths_glob"`
	CaseSensitive bool   `json:"case_sensitive"`
	MaxResults    int    `json:"max_results"`
	RootPath      string `json:"root_path"`
}

type SearchMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

type SearchTextOutput struct {
	Matches    []SearchMatch `json:"matches"`
	TotalCount int           `json:"total_count"`
	Truncated  bool          `json:"truncated"`
}

type SearchTextTool struct{}

func (t *SearchTextTool) Name() string        { return "search_text" }
func (t *SearchTextTool) Description() string {
	return "Searches for text (regex or plain) across files with optional glob filter."
}

func (t *SearchTextTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in SearchTextInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Query == "" {
		return nil, errors.New("query required")
	}

	if in.MaxResults <= 0 {
		in.MaxResults = 200
	}

	root := in.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	// Build regex
	pattern := in.Query
	if !in.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	// Collect files to search
	globPattern := in.PathsGlob
	if globPattern == "" {
		globPattern = "**/*"
	}

	files, err := collectFiles(root, globPattern)
	if err != nil {
		return nil, err
	}

	var matches []SearchMatch
	total := 0

	for _, filePath := range files {
		if ctx.Err() != nil {
			break
		}

		fileMatches, err := searchInFile(filePath, re, root)
		if err != nil {
			continue
		}

		for _, m := range fileMatches {
			total++
			if len(matches) < in.MaxResults {
				matches = append(matches, m)
			}
		}
	}

	out := SearchTextOutput{
		Matches:    matches,
		TotalCount: total,
		Truncated:  total > in.MaxResults,
	}

	return json.Marshal(out)
}

func searchInFile(filePath string, re *regexp.Regexp, root string) ([]SearchMatch, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Skip binary files by checking first 512 bytes
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return nil, nil // binary file
		}
	}
	// Seek back to start
	f.Seek(0, 0)

	var matches []SearchMatch
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if re.MatchString(line) {
			relPath, _ := filepath.Rel(root, filePath)
			if relPath == "" {
				relPath = filePath
			}
			matches = append(matches, SearchMatch{
				File:    relPath,
				Line:    lineNum,
				Content: line,
			})
		}
	}

	return matches, nil
}

// collectFiles walks the root and returns files matching a glob pattern.
func collectFiles(root, globPattern string) ([]string, error) {
	var files []string

	// Handle simple patterns like "*.go" or "**/*.go"
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip hidden directories and common non-code dirs
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(root, path)

		// Strip leading **/ for matching
		matchPattern := globPattern
		matchPattern = strings.TrimPrefix(matchPattern, "**/")

		matched, _ := filepath.Match(matchPattern, filepath.Base(relPath))
		if !matched {
			// Try matching against full relative path
			matched, _ = filepath.Match(globPattern, relPath)
		}
		// If the pattern is a wildcard-all, match everything
		if globPattern == "**/*" || globPattern == "*" {
			matched = true
		}

		if matched {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// ---------------------------------------------------------------------------
// search_symbol — symbol definition + references (best-effort)
// ---------------------------------------------------------------------------

type SearchSymbolInput struct {
	Symbol   string `json:"symbol"`
	Language string `json:"language"`
	Scope    string `json:"scope"` // "definition", "references", "all"
	RootPath string `json:"root_path"`
}

type SymbolMatch struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Content    string `json:"content"`
	Kind       string `json:"kind"` // "definition", "reference"
	SymbolType string `json:"symbol_type,omitempty"`
}

type SearchSymbolOutput struct {
	Definitions []SymbolMatch `json:"definitions"`
	References  []SymbolMatch `json:"references"`
}

type SearchSymbolTool struct{}

func (t *SearchSymbolTool) Name() string        { return "search_symbol" }
func (t *SearchSymbolTool) Description() string {
	return "Searches for symbol definitions and references across the codebase."
}

func (t *SearchSymbolTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in SearchSymbolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Symbol == "" {
		return nil, errors.New("symbol required")
	}

	root := in.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	scope := in.Scope
	if scope == "" {
		scope = "all"
	}

	lang := in.Language
	if lang == "" {
		lang = detectDominantLanguage(root)
	}

	out := SearchSymbolOutput{}

	// Language-specific definition patterns
	defPatterns := buildDefPatterns(in.Symbol, lang)
	refPattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(in.Symbol) + `\b`)

	ext := languageExtension(lang)
	files, _ := collectFiles(root, "*."+ext)

	for _, filePath := range files {
		if ctx.Err() != nil {
			break
		}

		relPath, _ := filepath.Rel(root, filePath)

		f, err := os.Open(filePath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			// Check definitions
			if scope == "all" || scope == "definition" {
				for _, dp := range defPatterns {
					if dp.re.MatchString(line) {
						out.Definitions = append(out.Definitions, SymbolMatch{
							File:       relPath,
							Line:       lineNum,
							Content:    strings.TrimSpace(line),
							Kind:       "definition",
							SymbolType: dp.kind,
						})
						break
					}
				}
			}

			// Check references
			if scope == "all" || scope == "references" {
				if refPattern.MatchString(line) {
					// Don't duplicate if already counted as definition
					isDef := false
					for _, dp := range defPatterns {
						if dp.re.MatchString(line) {
							isDef = true
							break
						}
					}
					if !isDef {
						out.References = append(out.References, SymbolMatch{
							File:    relPath,
							Line:    lineNum,
							Content: strings.TrimSpace(line),
							Kind:    "reference",
						})
					}
				}
			}
		}

		f.Close()
	}

	return json.Marshal(out)
}

type defPattern struct {
	re   *regexp.Regexp
	kind string
}

func buildDefPatterns(symbol, lang string) []defPattern {
	escaped := regexp.QuoteMeta(symbol)
	var patterns []defPattern

	switch lang {
	case "go":
		patterns = append(patterns,
			defPattern{regexp.MustCompile(`func\s+` + escaped + `\s*\(`), "function"},
			defPattern{regexp.MustCompile(`func\s+\([^)]+\)\s*` + escaped + `\s*\(`), "method"},
			defPattern{regexp.MustCompile(`type\s+` + escaped + `\s+`), "type"},
			defPattern{regexp.MustCompile(`var\s+` + escaped + `\s`), "variable"},
			defPattern{regexp.MustCompile(`const\s+` + escaped + `\s`), "constant"},
			defPattern{regexp.MustCompile(escaped + `\s*:=`), "variable"},
		)
	case "python":
		patterns = append(patterns,
			defPattern{regexp.MustCompile(`def\s+` + escaped + `\s*\(`), "function"},
			defPattern{regexp.MustCompile(`class\s+` + escaped + `[\s:(]`), "class"},
			defPattern{regexp.MustCompile(escaped + `\s*=`), "variable"},
		)
	case "javascript", "typescript":
		patterns = append(patterns,
			defPattern{regexp.MustCompile(`function\s+` + escaped + `\s*\(`), "function"},
			defPattern{regexp.MustCompile(`(const|let|var)\s+` + escaped + `\s*=`), "variable"},
			defPattern{regexp.MustCompile(`class\s+` + escaped + `[\s{]`), "class"},
			defPattern{regexp.MustCompile(`interface\s+` + escaped + `[\s{]`), "interface"},
			defPattern{regexp.MustCompile(`type\s+` + escaped + `\s*=`), "type"},
			defPattern{regexp.MustCompile(escaped + `\s*\([^)]*\)\s*[:{]`), "function"},
		)
	case "rust":
		patterns = append(patterns,
			defPattern{regexp.MustCompile(`fn\s+` + escaped + `\s*[(<]`), "function"},
			defPattern{regexp.MustCompile(`struct\s+` + escaped + `[\s{]`), "struct"},
			defPattern{regexp.MustCompile(`enum\s+` + escaped + `[\s{]`), "enum"},
			defPattern{regexp.MustCompile(`trait\s+` + escaped + `[\s{]`), "trait"},
			defPattern{regexp.MustCompile(`(let|const)\s+(mut\s+)?` + escaped + `\s*[=:]`), "variable"},
		)
	default:
		// Generic fallback
		patterns = append(patterns,
			defPattern{regexp.MustCompile(`func(tion)?\s+` + escaped + `\s*\(`), "function"},
			defPattern{regexp.MustCompile(`(class|type|struct|interface)\s+` + escaped), "type"},
			defPattern{regexp.MustCompile(`(var|let|const)\s+` + escaped + `\s*[=:]`), "variable"},
		)
	}

	return patterns
}

func languageExtension(lang string) string {
	switch lang {
	case "go":
		return "go"
	case "python":
		return "py"
	case "javascript":
		return "js"
	case "typescript":
		return "ts"
	case "rust":
		return "rs"
	case "java":
		return "java"
	case "c":
		return "c"
	case "cpp":
		return "cpp"
	default:
		return "*"
	}
}

func detectDominantLanguage(root string) string {
	markers := map[string]string{
		"go.mod":           "go",
		"Cargo.toml":       "rust",
		"package.json":     "javascript",
		"tsconfig.json":    "typescript",
		"pyproject.toml":   "python",
		"requirements.txt": "python",
	}
	for file, lang := range markers {
		if _, err := os.Stat(filepath.Join(root, file)); err == nil {
			return lang
		}
	}
	return "go" // default
}

// ---------------------------------------------------------------------------
// ripgrep — wrapper around `rg` with Go fallback
// ---------------------------------------------------------------------------

type RipgrepInput struct {
	Query         string `json:"query"`
	Path          string `json:"path"`
	CaseSensitive bool   `json:"case_sensitive"`
	FileGlob      string `json:"file_glob"`
	MaxResults    int    `json:"max_results"`
	ContextLines  int    `json:"context_lines"`
}

type RipgrepOutput struct {
	Matches  []SearchMatch `json:"matches"`
	UsedRg   bool          `json:"used_rg"`
	RawOutput string       `json:"raw_output,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type RipgrepTool struct{}

func (t *RipgrepTool) Name() string        { return "ripgrep" }
func (t *RipgrepTool) Description() string {
	return "Fast text search using ripgrep (rg) if available, with Go fallback."
}

func (t *RipgrepTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in RipgrepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Query == "" {
		return nil, errors.New("query required")
	}

	if in.MaxResults <= 0 {
		in.MaxResults = 200
	}

	searchPath := in.Path
	if searchPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		searchPath = cwd
	}

	// Try using rg first
	rgPath, err := exec.LookPath("rg")
	if err == nil {
		return executeRipgrep(ctx, rgPath, in, searchPath)
	}

	// Fallback to Go-based search
	return fallbackSearch(ctx, in, searchPath)
}

func executeRipgrep(ctx context.Context, rgPath string, in RipgrepInput, searchPath string) ([]byte, error) {
	args := []string{"--line-number", "--no-heading", "--color", "never"}

	if !in.CaseSensitive {
		args = append(args, "-i")
	}

	if in.MaxResults > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", in.MaxResults))
	}

	if in.ContextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", in.ContextLines))
	}

	if in.FileGlob != "" {
		args = append(args, "--glob", in.FileGlob)
	}

	args = append(args, in.Query, searchPath)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	output, err := cmd.CombinedOutput()

	// rg returns exit code 1 for "no matches" — that's not an error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return json.Marshal(RipgrepOutput{
				Matches: []SearchMatch{},
				UsedRg:  true,
			})
		}
		// Exit code 2+ is a real error
		return json.Marshal(RipgrepOutput{
			Matches: []SearchMatch{},
			UsedRg:  true,
			Error:   string(output),
		})
	}

	matches := parseRgOutput(string(output), searchPath)

	return json.Marshal(RipgrepOutput{
		Matches:   matches,
		UsedRg:    true,
		RawOutput: string(output),
	})
}

func parseRgOutput(output, root string) []SearchMatch {
	var matches []SearchMatch
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		// Format: file:line:content
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}

		lineNum := 0
		fmt.Sscanf(parts[1], "%d", &lineNum)

		relPath, _ := filepath.Rel(root, parts[0])
		if relPath == "" {
			relPath = parts[0]
		}

		matches = append(matches, SearchMatch{
			File:    relPath,
			Line:    lineNum,
			Content: parts[2],
		})
	}

	return matches
}

func fallbackSearch(ctx context.Context, in RipgrepInput, searchPath string) ([]byte, error) {
	// Reuse search_text logic
	stInput := SearchTextInput{
		Query:         in.Query,
		PathsGlob:     in.FileGlob,
		CaseSensitive: in.CaseSensitive,
		MaxResults:    in.MaxResults,
		RootPath:      searchPath,
	}

	payload, _ := json.Marshal(stInput)

	tool := &SearchTextTool{}
	result, err := tool.Execute(ctx, payload)
	if err != nil {
		return nil, err
	}

	var stOut SearchTextOutput
	json.Unmarshal(result, &stOut)

	out := RipgrepOutput{
		Matches: stOut.Matches,
		UsedRg:  false,
	}

	return json.Marshal(out)
}

// ---------------------------------------------------------------------------
// ctags_generate — generate ctags for a project
// ---------------------------------------------------------------------------

type CtagsGenerateInput struct {
	RootPath   string   `json:"root_path"`
	Languages  []string `json:"languages"`
	OutputFile string   `json:"output_file"`
}

type CtagsGenerateOutput struct {
	Success    bool   `json:"success"`
	OutputFile string `json:"output_file"`
	TagCount   int    `json:"tag_count"`
	Error      string `json:"error,omitempty"`
}

type CtagsGenerateTool struct{}

func (t *CtagsGenerateTool) Name() string        { return "ctags_generate" }
func (t *CtagsGenerateTool) Description() string {
	return "Generates ctags index for faster symbol lookup."
}

func (t *CtagsGenerateTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in CtagsGenerateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	root := in.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	outputFile := in.OutputFile
	if outputFile == "" {
		outputFile = filepath.Join(root, ".tags")
	}

	// Check for ctags
	ctagsBin, err := exec.LookPath("ctags")
	if err != nil {
		// Try universal-ctags
		ctagsBin, err = exec.LookPath("universal-ctags")
		if err != nil {
			return json.Marshal(CtagsGenerateOutput{
				Success: false,
				Error:   "ctags not found in PATH; install universal-ctags",
			})
		}
	}

	args := []string{"-R", "--output-format=u-ctags", "-f", outputFile}

	if len(in.Languages) > 0 {
		args = append(args, "--languages="+strings.Join(in.Languages, ","))
	}

	args = append(args, root)

	cmd := exec.CommandContext(ctx, ctagsBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return json.Marshal(CtagsGenerateOutput{
			Success: false,
			Error:   fmt.Sprintf("ctags failed: %s: %s", err.Error(), string(output)),
		})
	}

	// Count tags
	tagCount := 0
	if data, err := os.ReadFile(outputFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" && !strings.HasPrefix(line, "!") {
				tagCount++
			}
		}
	}

	return json.Marshal(CtagsGenerateOutput{
		Success:    true,
		OutputFile: outputFile,
		TagCount:   tagCount,
	})
}

// ---------------------------------------------------------------------------
// ctags_query — query generated ctags
// ---------------------------------------------------------------------------

type CtagsQueryInput struct {
	Symbol   string `json:"symbol"`
	TagFile  string `json:"tag_file"`
	RootPath string `json:"root_path"`
}

type CtagEntry struct {
	Symbol  string `json:"symbol"`
	File    string `json:"file"`
	Pattern string `json:"pattern"`
	Kind    string `json:"kind"`
	Line    int    `json:"line,omitempty"`
}

type CtagsQueryOutput struct {
	Entries []CtagEntry `json:"entries"`
	Error   string      `json:"error,omitempty"`
}

type CtagsQueryTool struct{}

func (t *CtagsQueryTool) Name() string        { return "ctags_query" }
func (t *CtagsQueryTool) Description() string {
	return "Queries a ctags index for symbol definitions."
}

func (t *CtagsQueryTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in CtagsQueryInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Symbol == "" {
		return nil, errors.New("symbol required")
	}

	root := in.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	tagFile := in.TagFile
	if tagFile == "" {
		tagFile = filepath.Join(root, ".tags")
	}

	data, err := os.ReadFile(tagFile)
	if err != nil {
		return json.Marshal(CtagsQueryOutput{
			Error: fmt.Sprintf("cannot read tag file: %s — run ctags_generate first", err.Error()),
		})
	}

	var entries []CtagEntry
	symbolLower := strings.ToLower(in.Symbol)

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}

		tagName := parts[0]
		if strings.ToLower(tagName) != symbolLower && tagName != in.Symbol {
			continue
		}

		entry := CtagEntry{
			Symbol:  tagName,
			File:    parts[1],
			Pattern: parts[2],
		}

		// Parse extended fields
		for _, field := range parts[3:] {
			if strings.HasPrefix(field, "kind:") {
				entry.Kind = strings.TrimPrefix(field, "kind:")
			}
			if strings.HasPrefix(field, "line:") {
				fmt.Sscanf(strings.TrimPrefix(field, "line:"), "%d", &entry.Line)
			}
		}

		entries = append(entries, entry)
	}

	return json.Marshal(CtagsQueryOutput{Entries: entries})
}

// ---------------------------------------------------------------------------
// ast_parse — Go AST-based parsing for outlines
// ---------------------------------------------------------------------------

type AstParseInput struct {
	Path string `json:"path"`
}

type AstSymbol struct {
	Name      string      `json:"name"`
	Kind      string      `json:"kind"` // "function", "method", "type", "struct", "interface", "const", "var", "import"
	Line      int         `json:"line"`
	EndLine   int         `json:"end_line,omitempty"`
	Signature string      `json:"signature,omitempty"`
	Children  []AstSymbol `json:"children,omitempty"`
}

type AstParseOutput struct {
	File    string      `json:"file"`
	Package string      `json:"package"`
	Imports []string    `json:"imports"`
	Symbols []AstSymbol `json:"symbols"`
	Error   string      `json:"error,omitempty"`
}

type AstParseTool struct{}

func (t *AstParseTool) Name() string        { return "ast_parse" }
func (t *AstParseTool) Description() string {
	return "Parses a Go file and returns its structural outline (functions, types, imports)."
}

func (t *AstParseTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in AstParseInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	ext := strings.ToLower(filepath.Ext(in.Path))
	if ext == ".go" {
		return parseGoFile(in.Path)
	}

	// For non-Go files, use regex-based fallback
	return parseGenericFile(in.Path)
}

func parseGoFile(path string) ([]byte, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return json.Marshal(AstParseOutput{
			File:  path,
			Error: err.Error(),
		})
	}

	out := AstParseOutput{
		File:    path,
		Package: node.Name.Name,
	}

	// Extract imports
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil {
			importPath = imp.Name.Name + " " + importPath
		}
		out.Imports = append(out.Imports, importPath)
	}

	// Extract declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := AstSymbol{
				Name:    d.Name.Name,
				Kind:    "function",
				Line:    fset.Position(d.Pos()).Line,
				EndLine: fset.Position(d.End()).Line,
			}

			// Build signature
			sig := "func "
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = "method"
				recv := d.Recv.List[0]
				sig += "(" + formatFieldType(recv) + ") "
			}
			sig += d.Name.Name + "("
			if d.Type.Params != nil {
				sig += formatFieldList(d.Type.Params)
			}
			sig += ")"
			if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
				sig += " "
				if len(d.Type.Results.List) > 1 {
					sig += "(" + formatFieldList(d.Type.Results) + ")"
				} else {
					sig += formatFieldList(d.Type.Results)
				}
			}
			sym.Signature = sig

			out.Symbols = append(out.Symbols, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := AstSymbol{
						Name: s.Name.Name,
						Line: fset.Position(s.Pos()).Line,
					}

					switch st := s.Type.(type) {
					case *ast.StructType:
						sym.Kind = "struct"
						sym.EndLine = fset.Position(st.End()).Line
						// Extract struct fields as children
						if st.Fields != nil {
							for _, field := range st.Fields.List {
								for _, name := range field.Names {
									child := AstSymbol{
										Name: name.Name,
										Kind: "field",
										Line: fset.Position(field.Pos()).Line,
									}
									sym.Children = append(sym.Children, child)
								}
							}
						}
					case *ast.InterfaceType:
						sym.Kind = "interface"
						sym.EndLine = fset.Position(st.End()).Line
						// Extract interface methods
						if st.Methods != nil {
							for _, method := range st.Methods.List {
								for _, name := range method.Names {
									child := AstSymbol{
										Name: name.Name,
										Kind: "method",
										Line: fset.Position(method.Pos()).Line,
									}
									sym.Children = append(sym.Children, child)
								}
							}
						}
					default:
						sym.Kind = "type"
					}

					out.Symbols = append(out.Symbols, sym)

				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						out.Symbols = append(out.Symbols, AstSymbol{
							Name: name.Name,
							Kind: kind,
							Line: fset.Position(name.Pos()).Line,
						})
					}
				}
			}
		}
	}

	return json.Marshal(out)
}

func formatFieldType(field *ast.Field) string {
	if field == nil || field.Type == nil {
		return ""
	}
	switch t := field.Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
		return "*..."
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	default:
		return "..."
	}
}

func formatFieldList(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	var parts []string
	for _, field := range fl.List {
		typeStr := formatFieldType(field)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				parts = append(parts, name.Name+" "+typeStr)
			}
		} else {
			parts = append(parts, typeStr)
		}
	}
	return strings.Join(parts, ", ")
}

func parseGenericFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out := AstParseOutput{
		File: path,
	}

	lines := strings.Split(string(data), "\n")

	// Generic regex patterns for common languages
	funcRe := regexp.MustCompile(`^\s*(pub\s+)?(fn|func|def|function|async\s+function|async\s+fn)\s+(\w+)`)
	classRe := regexp.MustCompile(`^\s*(pub\s+)?(class|struct|enum|trait|interface|type)\s+(\w+)`)
	importRe := regexp.MustCompile(`^\s*(import|from|use|require|include)\s+(.+)`)

	for i, line := range lines {
		lineNum := i + 1

		if m := funcRe.FindStringSubmatch(line); m != nil {
			out.Symbols = append(out.Symbols, AstSymbol{
				Name:      m[3],
				Kind:      "function",
				Line:      lineNum,
				Signature: strings.TrimSpace(line),
			})
		}

		if m := classRe.FindStringSubmatch(line); m != nil {
			out.Symbols = append(out.Symbols, AstSymbol{
				Name: m[3],
				Kind: m[2], // struct, class, enum, etc.
				Line: lineNum,
			})
		}

		if m := importRe.FindStringSubmatch(line); m != nil {
			out.Imports = append(out.Imports, strings.TrimSpace(m[2]))
		}
	}

	return json.Marshal(out)
}

// ---------------------------------------------------------------------------
// code_outline — summarized outlines for multiple files
// ---------------------------------------------------------------------------

type CodeOutlineInput struct {
	Paths    []string `json:"paths"`
	RootPath string   `json:"root_path"`
}

type FileOutline struct {
	File    string      `json:"file"`
	Package string      `json:"package,omitempty"`
	Symbols []AstSymbol `json:"symbols"`
	Error   string      `json:"error,omitempty"`
}

type CodeOutlineOutput struct {
	Outlines []FileOutline `json:"outlines"`
}

type CodeOutlineTool struct{}

func (t *CodeOutlineTool) Name() string        { return "code_outline" }
func (t *CodeOutlineTool) Description() string {
	return "Returns summarized structural outlines for multiple files."
}

func (t *CodeOutlineTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in CodeOutlineInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if len(in.Paths) == 0 {
		return nil, errors.New("paths required")
	}

	root := in.RootPath
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	// Expand globs in paths
	var resolvedPaths []string
	for _, p := range in.Paths {
		if strings.Contains(p, "*") {
			matches, err := filepath.Glob(p)
			if err == nil {
				resolvedPaths = append(resolvedPaths, matches...)
			}
		} else {
			// If relative, resolve against root
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			resolvedPaths = append(resolvedPaths, p)
		}
	}

	out := CodeOutlineOutput{}

	astTool := &AstParseTool{}

	for _, filePath := range resolvedPaths {
		if ctx.Err() != nil {
			break
		}

		payload, _ := json.Marshal(AstParseInput{Path: filePath})
		result, err := astTool.Execute(ctx, payload)
		if err != nil {
			relPath, _ := filepath.Rel(root, filePath)
			out.Outlines = append(out.Outlines, FileOutline{
				File:  relPath,
				Error: err.Error(),
			})
			continue
		}

		var parsed AstParseOutput
		json.Unmarshal(result, &parsed)

		relPath, _ := filepath.Rel(root, filePath)
		if relPath == "" {
			relPath = filePath
		}

		outline := FileOutline{
			File:    relPath,
			Package: parsed.Package,
			Symbols: parsed.Symbols,
			Error:   parsed.Error,
		}

		out.Outlines = append(out.Outlines, outline)
	}

	return json.Marshal(out)
}
