package tools

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ReadFileInput struct {
	Path      string `json:"path" jsonschema:"Absolute or relative file path to read"`
	StartLine int    `json:"start_line,omitzero" jsonschema:"1-based start line (0 means from beginning)"`
	EndLine   int    `json:"end_line,omitzero" jsonschema:"1-based end line (0 means to end of file)"`
	MaxBytes  int64  `json:"max_bytes,omitzero" jsonschema:"Maximum bytes to read (0 for no limit)"`
}

type ReadFileOutput struct {
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	Encoding  string `json:"encoding"`
	Truncated bool   `json:"truncated"`
}

type ReadFileTool struct{}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Reads file content with optional line range and byte limit."
}
func (t *ReadFileTool) InputType() any { return ReadFileInput{} }

func (t *ReadFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in ReadFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, err
	}

	size := int64(len(data))
	content := string(data)

	// Line slicing
	if in.StartLine > 0 || in.EndLine > 0 {
		lines := strings.Split(content, "\n")

		start := in.StartLine - 1
		if start < 0 {
			start = 0
		}
		end := in.EndLine
		if end <= 0 || end > len(lines) {
			end = len(lines)
		}

		if start < len(lines) {
			content = strings.Join(lines[start:end], "\n")
		} else {
			content = ""
		}
	}

	truncated := false

	// Byte limit
	if in.MaxBytes > 0 && int64(len(content)) > in.MaxBytes {
		content = content[:in.MaxBytes]
		truncated = true
	}

	out := ReadFileOutput{
		Content:   content,
		Size:      size,
		Encoding:  "utf-8",
		Truncated: truncated,
	}

	return json.Marshal(out)
}

type ReadManyFilesInput struct {
	Paths    []string `json:"paths" jsonschema:"List of file paths to read"`
	MaxBytes int64    `json:"max_bytes,omitzero" jsonschema:"Maximum bytes per file (0 for no limit)"`
}

type ReadManyFilesOutput struct {
	Files map[string]ReadFileOutput `json:"files"`
}

type ReadManyFilesTool struct{}

func (t *ReadManyFilesTool) Name() string {
	return "read_many_files"
}

func (t *ReadManyFilesTool) Description() string {
	return "Reads multiple files at once."
}
func (t *ReadManyFilesTool) InputType() any { return ReadManyFilesInput{} }

func (t *ReadManyFilesTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in ReadManyFilesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	files := make(map[string]ReadFileOutput)

	readTool := ReadFileTool{}

	for _, path := range in.Paths {
		payload, _ := json.Marshal(ReadFileInput{
			Path:     path,
			MaxBytes: in.MaxBytes,
		})

		resBytes, err := readTool.Execute(ctx, payload)
		if err != nil {
			continue
		}

		var out ReadFileOutput
		json.Unmarshal(resBytes, &out)
		files[path] = out
	}

	return json.Marshal(ReadManyFilesOutput{Files: files})
}

type WriteFileInput struct {
	Path       string `json:"path" jsonschema:"File path to write to"`
	Content    string `json:"content" jsonschema:"Content to write to the file"`
	CreateDirs bool   `json:"create_dirs,omitzero" jsonschema:"Create parent directories if they do not exist"`
	Mode       uint32 `json:"mode,omitzero" jsonschema:"File permission mode (e.g. 0644)"`
}

type WriteFileOutput struct {
	Success  bool   `json:"success"`
	Checksum string `json:"checksum"`
}

type WriteFileTool struct{}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Writes content to a file."
}
func (t *WriteFileTool) InputType() any { return WriteFileInput{} }

func (t *WriteFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in WriteFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	if in.CreateDirs {
		dir := filepath.Dir(in.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	mode := os.FileMode(0644)
	if in.Mode != 0 {
		mode = os.FileMode(in.Mode)
	}

	if err := os.WriteFile(in.Path, []byte(in.Content), mode); err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(in.Content))

	out := WriteFileOutput{
		Success:  true,
		Checksum: hex.EncodeToString(hash[:]),
	}

	return json.Marshal(out)
}

type AppendFileInput struct {
	Path    string `json:"path" jsonschema:"File path to append to"`
	Content string `json:"content" jsonschema:"Content to append"`
}

type AppendFileOutput struct {
	Success bool `json:"success"`
}

type AppendFileTool struct{}

func (t *AppendFileTool) Name() string {
	return "append_file"
}

func (t *AppendFileTool) Description() string {
	return "Appends content to a file."
}
func (t *AppendFileTool) InputType() any { return AppendFileInput{} }

func (t *AppendFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in AppendFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	f, err := os.OpenFile(in.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.WriteString(in.Content); err != nil {
		return nil, err
	}

	return json.Marshal(AppendFileOutput{Success: true})
}

type DeleteFileInput struct {
	Path         string `json:"path" jsonschema:"File path to delete"`
	ConfirmToken string `json:"confirm_token" jsonschema:"Confirmation token to prevent accidental deletion"`
}

type DeleteFileOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type DeleteFileTool struct{}

func (t *DeleteFileTool) Name() string {
	return "delete_file"
}

func (t *DeleteFileTool) Description() string {
	return "Deletes a file. Requires confirm_token."
}
func (t *DeleteFileTool) InputType() any { return DeleteFileInput{} }

func (t *DeleteFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in DeleteFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	info, err := os.Stat(in.Path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, errors.New("refusing to delete directory")
	}

	if in.ConfirmToken == "" {
		return nil, errors.New("confirm_token required")
	}

	if err := os.Remove(in.Path); err != nil {
		return nil, err
	}

	return json.Marshal(DeleteFileOutput{
		Success: true,
		Message: "file deleted",
	})
}

type MoveCopyInput struct {
	Src       string `json:"src" jsonschema:"Source file path"`
	Dst       string `json:"dst" jsonschema:"Destination file path"`
	Overwrite bool   `json:"overwrite,omitzero" jsonschema:"Overwrite destination if it exists"`
}

type MoveCopyOutput struct {
	Success bool `json:"success"`
}

type MoveFileTool struct{}
type CopyFileTool struct{}

func (t *MoveFileTool) Name() string { return "move_file" }
func (t *CopyFileTool) Name() string { return "copy_file" }

func (t *MoveFileTool) Description() string {
	return "Moves a file from src to dst."
}
func (t *CopyFileTool) Description() string {
	return "Copies a file from src to dst."
}
func (t *MoveFileTool) InputType() any { return MoveCopyInput{} }
func (t *CopyFileTool) InputType() any { return MoveCopyInput{} }

func (t *MoveFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in MoveCopyInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Src == "" || in.Dst == "" {
		return nil, errors.New("src and dst required")
	}

	if !in.Overwrite {
		if _, err := os.Stat(in.Dst); err == nil {
			return nil, errors.New("destination exists")
		}
	}

	// Create destination directory if needed
	if dir := filepath.Dir(in.Dst); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	// Try os.Rename first (fast, same-filesystem move)
	if err := os.Rename(in.Src, in.Dst); err != nil {
		// Fallback: copy + delete (cross-filesystem move)
		srcFile, err := os.Open(in.Src)
		if err != nil {
			return nil, err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(in.Dst)
		if err != nil {
			return nil, err
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return nil, err
		}

		// Close before removing
		srcFile.Close()
		if err := os.Remove(in.Src); err != nil {
			return nil, fmt.Errorf("copied but failed to remove source: %w", err)
		}
	}

	return json.Marshal(MoveCopyOutput{Success: true})
}

func (t *CopyFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in MoveCopyInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if !in.Overwrite {
		if _, err := os.Stat(in.Dst); err == nil {
			return nil, errors.New("destination exists")
		}
	}

	srcFile, err := os.Open(in.Src)
	if err != nil {
		return nil, err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(in.Dst)
	if err != nil {
		return nil, err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return nil, err
	}

	return json.Marshal(MoveCopyOutput{Success: true})
}

type FileEdit struct {
	StartLine   int    `json:"start_line" jsonschema:"1-based start line of the range to replace"`
	EndLine     int    `json:"end_line" jsonschema:"1-based end line of the range to replace (inclusive)"`
	Replacement string `json:"replacement" jsonschema:"Replacement text for the specified line range"`
}

type EditFileRangesInput struct {
	Path  string     `json:"path" jsonschema:"File path to edit"`
	Edits []FileEdit `json:"edits" jsonschema:"List of line-range edits to apply"`
}

type EditFileRangesOutput struct {
	Success bool `json:"success"`
}

type EditFileRangesTool struct{}

func (t *EditFileRangesTool) Name() string {
	return "edit_file_ranges"
}

func (t *EditFileRangesTool) Description() string {
	return "Edits specific line ranges in a file."
}
func (t *EditFileRangesTool) InputType() any { return EditFileRangesInput{} }

func (t *EditFileRangesTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in EditFileRangesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")

	for _, edit := range in.Edits {
		start := edit.StartLine - 1
		end := edit.EndLine

		if start < 0 || start >= len(lines) {
			continue
		}
		if end > len(lines) {
			end = len(lines)
		}

		replacementLines := strings.Split(edit.Replacement, "\n")

		lines = append(lines[:start], append(replacementLines, lines[end:]...)...)
	}

	newContent := strings.Join(lines, "\n")

	if err := os.WriteFile(in.Path, []byte(newContent), 0644); err != nil {
		return nil, err
	}

	return json.Marshal(EditFileRangesOutput{Success: true})
}

type ApplyDiffInput struct {
	Diff string `json:"diff" jsonschema:"Unified diff content to apply"`
}

type ApplyDiffOutput struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error"`
}

type ApplyUnifiedDiffTool struct{}

func (t *ApplyUnifiedDiffTool) Name() string {
	return "apply_unified_diff"
}

func (t *ApplyUnifiedDiffTool) Description() string {
	return "Applies a unified diff patch."
}
func (t *ApplyUnifiedDiffTool) InputType() any { return ApplyDiffInput{} }

func (t *ApplyUnifiedDiffTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in ApplyDiffInput
	json.Unmarshal(input, &in)

	cmd := exec.CommandContext(ctx, "patch", "-p0")
	cmd.Stdin = strings.NewReader(in.Diff)

	out, err := cmd.CombinedOutput()

	result := ApplyDiffOutput{
		Success: err == nil,
		Output:  string(out),
	}

	if err != nil {
		result.Error = err.Error()
	}

	return json.Marshal(result)
}

type FileStatInput struct {
	Path string `json:"path" jsonschema:"File path to get metadata for"`
}

type FileStatOutput struct {
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Mode     string `json:"mode"`
	IsBinary bool   `json:"is_binary"`
}

type FileStatTool struct{}

func (t *FileStatTool) Name() string { return "file_stat" }
func (t *FileStatTool) Description() string {
	return "Returns metadata about a file."
}
func (t *FileStatTool) InputType() any { return FileStatInput{} }

func (t *FileStatTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in FileStatInput
	json.Unmarshal(input, &in)

	info, err := os.Stat(in.Path)
	if err != nil {
		return nil, err
	}

	data, _ := os.ReadFile(in.Path)
	isBinary := false
	for _, b := range data {
		if b == 0 {
			isBinary = true
			break
		}
	}

	out := FileStatOutput{
		Size:     info.Size(),
		Modified: info.ModTime().String(),
		Mode:     info.Mode().String(),
		IsBinary: isBinary,
	}

	return json.Marshal(out)
}

type HashFileInput struct {
	Path string `json:"path" jsonschema:"File path to hash"`
	Algo string `json:"algo,omitzero" jsonschema:"Hash algorithm: sha256 (default) or md5"`
}

type HashFileOutput struct {
	Hash string `json:"hash"`
}

type HashFileTool struct{}

func (t *HashFileTool) Name() string { return "hash_file" }
func (t *HashFileTool) Description() string {
	return "Computes file hash."
}
func (t *HashFileTool) InputType() any { return HashFileInput{} }

func (t *HashFileTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in HashFileInput
	json.Unmarshal(input, &in)

	data, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, err
	}

	switch in.Algo {
	case "md5":
		h := md5.Sum(data)
		return json.Marshal(HashFileOutput{Hash: hex.EncodeToString(h[:])})
	default:
		h := sha256.Sum256(data)
		return json.Marshal(HashFileOutput{Hash: hex.EncodeToString(h[:])})
	}
}
