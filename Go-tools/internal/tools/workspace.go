package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

type WorkspaceInfo struct {
	RootPath string
}

type WorkspaceInfoInput struct {
	Path string `json:"path"`
}
type WorkspaceInfoOutput struct {
	RootPath          string            `json:"root_path"`
	CurrentDir        string            `json:"current_dir"`
	OS                string            `json:"os"`
	Arch              string            `json:"arch"`
	DetectedLanguages []string          `json:"detected_languages"`
	AvailableBinaries map[string]string `json:"available_binaries"`
}

type WorkspaceInfoTool struct{}

func (t *WorkspaceInfoTool) Name() string {
	return "workspace_info"
}

func (t *WorkspaceInfoTool) Description() string {
	return "Provides information about the current workspace, including the root path and available tools."
}

func (t *WorkspaceInfoTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in WorkspaceInfoInput
	_ = json.Unmarshal(input, &in)

	root := in.Path
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	languages := DetectLanguages(root)

	available := map[string]string{
		"go":     detectBinary("go"),
		"python": detectBinary("python"),
		"node":   detectBinary("node"),
		"rust":   detectBinary("rustc"),
		"cargo":  detectBinary("cargo"),
		"npm":    detectBinary("npm"),
	}

	out := WorkspaceInfoOutput{
		RootPath:          root,
		CurrentDir:        currentDir,
		OS:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		DetectedLanguages: languages,
		AvailableBinaries: available,
	}
	return json.Marshal(out)

}

type ListDirInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	MaxDepth  int    `json:"max_depth"`
}

type FileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type ListDirOutput struct {
	Entries []FileEntry `json:"entries"`
}

type ListDirTool struct{}

func (t *ListDirTool) Name() string {
	return "list_dir"
}
func (t *ListDirTool) Description() string {
	return "Lists files and directories in a specified path, with options for recursion and depth."
}

func (t *ListDirTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in ListDirInput
	_ = json.Unmarshal(input, &in)

	root := in.Path
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}
	maxDepth := in.MaxDepth

	if maxDepth <= 0 {
		maxDepth = 3
	}

	var entries []FileEntry

	if in.Recursive {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(root, path)
			depth := len(filepath.SplitList(relPath))

			if depth > maxDepth {
				return filepath.SkipDir
			}

			info, err := d.Info()
			entries = append(entries, FileEntry{
				Path:  relPath,
				IsDir: d.IsDir(),
				Size:  info.Size(),
			})
			return nil
		})
	} else {
		files, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			info, _ := file.Info()
			entries = append(entries, FileEntry{
				Path:  file.Name(),
				IsDir: file.IsDir(),
				Size:  info.Size(),
			})
		}
	}
	out := ListDirOutput{Entries: entries}
	return json.Marshal(out)

}

type ProjectDetectInput struct {
	Path string `json:"path"`
}

type RecommendedCommands struct {
	Build []string `json:"build"`
	Test  []string `json:"test"`
	Lint  []string `json:"lint"`
}

type ProjectDetectOutput struct {
	PrimaryLanguage     string              `json:"primary_language"`
	DetectedLanguages   []string            `json:"detected_languages"`
	RecommendedCommands RecommendedCommands `json:"recommended_commands"`
}

type ProjectDetectTool struct{}

func (t *ProjectDetectTool) Description() string {
	return "Detects project type and recommends build, test, and lint commands."
}

func (t *ProjectDetectTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in ProjectDetectInput
	_ = json.Unmarshal(input, &in)

	root := in.Path
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	detected := []string{}
	commands := RecommendedCommands{}

	check := func(file string) bool {
		_, err := os.Stat(filepath.Join(root, file))
		return err == nil
	}

	// Go
	if check("go.mod") {
		detected = append(detected, "go")
		commands.Build = []string{"go build ./..."}
		commands.Test = []string{"go test ./..."}
		commands.Lint = []string{"go vet ./..."}
	}

	// Node / JS
	if check("package.json") {
		detected = append(detected, "javascript")
		commands.Build = append(commands.Build, "npm run build")
		commands.Test = append(commands.Test, "npm test")
		commands.Lint = append(commands.Lint, "npm run lint")
	}

	// Python
	if check("pyproject.toml") || check("requirements.txt") {
		detected = append(detected, "python")
		commands.Test = append(commands.Test, "pytest")
		commands.Lint = append(commands.Lint, "ruff check .")
	}

	// Rust
	if check("Cargo.toml") {
		detected = append(detected, "rust")
		commands.Build = append(commands.Build, "cargo build")
		commands.Test = append(commands.Test, "cargo test")
		commands.Lint = append(commands.Lint, "cargo clippy")
	}

	primary := ""
	if len(detected) > 0 {
		primary = detected[0]
	}

	out := ProjectDetectOutput{
		PrimaryLanguage:     primary,
		DetectedLanguages:   detected,
		RecommendedCommands: commands,
	}

	return json.Marshal(out)
}
