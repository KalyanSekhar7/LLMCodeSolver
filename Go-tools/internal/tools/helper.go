package tools

import (
	"os"
	"os/exec"
	"path/filepath"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectBinary(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, str := range input {
		if _, exists := seen[str]; !exists {
			seen[str] = struct{}{}
			result = append(result, str)
		}
	}
	return result
}

func DetectLanguages(root string) []string {
	var languages []string

	check := func(file string, lang string) {
		if fileExists(filepath.Join(root, file)) {
			languages = append(languages, lang)
		}
	}

	check("go.mod", "go")
	check("package.json", "javascript")
	check("pyproject.toml", "python")
	check("requirements.txt", "python")
	check("Cargo.toml", "rust")
	check("tsconfig.json", "typescript")

	return uniqueStrings(languages)

}
