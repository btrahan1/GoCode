package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ToolSystem struct {
	ignoredDirs       []string
	ignoredExtensions []string
	db                *sql.DB // Reference to Postgres DB for executing SQL queries
}

func NewToolSystem(db *sql.DB) *ToolSystem {
	return &ToolSystem{
		ignoredDirs:       []string{".git", "bin", "obj", ".vs", "node_modules", "lib", "ebwebview"},
		ignoredExtensions: []string{".map", ".png", ".ico", ".jpg", ".jpeg", ".gif", ".pdf", ".zip", ".tar", ".gz", ".log", ".exe", ".dll", ".pdb", ".db", ".sqlite", ".docx", ".xlsx"},
		db:                db,
	}
}

func (ts *ToolSystem) IsIgnored(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for _, part := range parts {
		for _, ignored := range ts.ignoredDirs {
			if strings.EqualFold(part, ignored) {
				return true
			}
		}
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, ignoredExt := range ts.ignoredExtensions {
		if ext == ignoredExt {
			return true
		}
	}
	return false
}

func (ts *ToolSystem) ListDirectory(root string) string {
	var sb strings.Builder
	sb.WriteString("WORKSPACE DIRECTORY STRUCTURE:\n")
	
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ts.IsIgnored(path, root) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				sb.WriteString(fmt.Sprintf("- %s\n", rel))
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Sprintf("Error listing directory: %v", err)
	}
	return sb.String()
}

func (ts *ToolSystem) ReadFile(filePath string, startLine, endLine int) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return fmt.Sprintf("[FILE CONTENT OF %s is empty]", filepath.Base(filePath))
	}

	start := 1
	if startLine > 0 {
		start = startLine
	}
	end := len(lines)
	if endLine > 0 {
		end = endLine
	}

	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		start = len(lines)
	}
	if end < start {
		end = start
	}
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[FILE CONTENT OF %s (Lines %d-%d of %d)]\n", filepath.Base(filePath), start, end, len(lines)))
	for i := start; i <= end; i++ {
		sb.WriteString(fmt.Sprintf("%d: %s\n", i, lines[i-1]))
	}

	return sb.String()
}

func (ts *ToolSystem) WriteFile(filePath, content string) string {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("Error creating directories: %v", err)
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err)
	}

	return fmt.Sprintf("Success: Wrote '%s' successfully.", filepath.Base(filePath))
}

func (ts *ToolSystem) ReplaceText(filePath, target, replacement string, startLine, endLine int) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("Error: File '%s' does not exist.", filePath)
	}

	originalContent := string(data)
	normalizedOriginal := strings.ReplaceAll(originalContent, "\r\n", "\n")
	lines := strings.Split(normalizedOriginal, "\n")

	searchArea := normalizedOriginal
	startCharIdx := 0
	searchAreaLength := len(normalizedOriginal)

	isScoped := startLine > 0 && endLine > 0
	if isScoped {
		startIdx := startLine - 1
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx >= len(lines) {
			startIdx = len(lines) - 1
		}
		endIdx := endLine - 1
		if endIdx < startIdx {
			endIdx = startIdx
		}
		if endIdx >= len(lines) {
			endIdx = len(lines) - 1
		}

		var sbSearch strings.Builder
		for i := startIdx; i <= endIdx; i++ {
			sbSearch.WriteString(lines[i] + "\n")
		}
		searchArea = sbSearch.String()
		searchAreaLength = len(searchArea)

		for i := 0; i < startIdx; i++ {
			startCharIdx += len(lines[i]) + 1 // +1 for the \n delimiter
		}
	}

	// Exact Match
	matchIdx := strings.Index(searchArea, target)
	if matchIdx != -1 {
		lastMatchIdx := strings.LastIndex(searchArea, target)
		if matchIdx != lastMatchIdx {
			return "Error: Target text matches multiple locations. Please specify a narrower range."
		}

		newSearchArea := searchArea[:matchIdx] + replacement + searchArea[matchIdx+len(target):]
		
		var finalContent string
		if isScoped {
			// Ensure boundaries
			if startCharIdx > len(originalContent) {
				startCharIdx = len(originalContent)
			}
			endOffset := startCharIdx + searchAreaLength
			if endOffset > len(originalContent) {
				endOffset = len(originalContent)
			}
			finalContent = originalContent[:startCharIdx] + newSearchArea + originalContent[endOffset:]
		} else {
			finalContent = newSearchArea
		}

		if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err)
		}
		return fmt.Sprintf("Success: Replaced target text block in '%s'.", filepath.Base(filePath))
	}

	// Fuzzy Line Matcher
	if fuzzySearchArea, ok := ts.tryFuzzyReplace(searchArea, target, replacement); ok {
		var finalContent string
		if isScoped {
			endOffset := startCharIdx + searchAreaLength
			if endOffset > len(originalContent) {
				endOffset = len(originalContent)
			}
			finalContent = originalContent[:startCharIdx] + fuzzySearchArea + originalContent[endOffset:]
		} else {
			finalContent = fuzzySearchArea
		}

		if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err)
		}
		return fmt.Sprintf("Success: Replaced target text block using normalized fuzzy line matching in '%s'.", filepath.Base(filePath))
	}

	return "Error: Edit failed. Target text block not found. Verify the target content."
}

func (ts *ToolSystem) tryFuzzyReplace(fileContent, targetText, replacementText string) (string, bool) {
	targetLines := []string{}
	for _, l := range strings.Split(strings.ReplaceAll(targetText, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			targetLines = append(targetLines, trimmed)
		}
	}
	if len(targetLines) == 0 {
		return fileContent, false
	}

	sourceLines := strings.Split(strings.ReplaceAll(fileContent, "\r", ""), "\n")
	rawSourceLines := strings.Split(fileContent, "\n")

	for i := 0; i <= len(sourceLines)-len(targetLines); i++ {
		match := true
		sourceOffset := 0

		for j := 0; j < len(targetLines); j++ {
			for i+sourceOffset < len(sourceLines) && strings.TrimSpace(sourceLines[i+sourceOffset]) == "" {
				sourceOffset++
			}

			if i+sourceOffset >= len(sourceLines) {
				match = false
				break
			}

			srcLine := strings.TrimSpace(sourceLines[i+sourceOffset])
			tgtLine := targetLines[j]

			if srcLine != tgtLine {
				match = false
				break
			}
			sourceOffset++
		}

		if match {
			charStart := 0
			for k := 0; k < i; k++ {
				charStart += len(rawSourceLines[k]) + 1
			}

			charEnd := charStart
			for k := i; k < i+sourceOffset; k++ {
				if k < len(rawSourceLines) {
					charEnd += len(rawSourceLines[k]) + 1
				}
			}

			if charEnd > len(fileContent) {
				charEnd = len(fileContent)
			}

			modified := fileContent[:charStart] + replacementText + fileContent[charEnd:]
			return modified, true
		}
	}

	return fileContent, false
}

func (ts *ToolSystem) RunCommand(ctx context.Context, command, workingDir string, onOutput func(string)) (string, error) {
	if _, err := os.Stat(workingDir); os.IsNotExist(err) {
		return "Error: Working directory does not exist.", nil
	}

	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", command)
	cmd.Dir = workingDir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var sb strings.Builder
	outputChan := make(chan string)

	readPipe := func(r io.Reader, prefix string) {
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				txt := string(buf[:n])
				outputChan <- prefix + txt
			}
			if err != nil {
				break
			}
		}
	}

	go readPipe(stdoutPipe, "")
	go readPipe(stderrPipe, "[ERROR] ")

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	for {
		select {
		case txt := <-outputChan:
			onOutput(txt)
			sb.WriteString(txt)
		case err := <-done:
			if err != nil {
				return sb.String(), err
			}
			return sb.String(), nil
		case <-ctx.Done():
			return sb.String() + "\n[Command Canceled by User]", ctx.Err()
		}
	}
}

func (ts *ToolSystem) ExecuteSQL(query string) string {
	rows, err := ts.db.Query(query)
	if err != nil {
		return fmt.Sprintf("Error executing SQL command: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("Error reading columns: %v", err)
	}

	if len(cols) == 0 {
		return "Success: Query executed successfully (no columns returned)."
	}

	var sb strings.Builder
	sb.WriteString("| " + strings.Join(cols, " | ") + " |\n")
	sb.WriteString("| " + strings.Repeat("--- | ", len(cols)) + "\n")

	rawResult := make([][]byte, len(cols))
	dest := make([]interface{}, len(cols))
	for i := range rawResult {
		dest[i] = &rawResult[i]
	}

	rowCount := 0
	for rows.Next() {
		rowCount++
		if err := rows.Scan(dest...); err != nil {
			return fmt.Sprintf("Error scanning row: %v", err)
		}

		rowVals := make([]string, len(cols))
		for idx, raw := range rawResult {
			if raw == nil {
				rowVals[idx] = "NULL"
			} else {
				rowVals[idx] = string(raw)
			}
		}
		sb.WriteString("| " + strings.Join(rowVals, " | ") + " |\n")
	}

	if rowCount == 0 {
		return "(0 rows returned)"
	}

	return sb.String()
}

func (ts *ToolSystem) SearchCode(root, query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SEARCH RESULTS FOR '%s':\n", query))
	matchCount := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ts.IsIgnored(path, root) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
					matchCount++
					rel, _ := filepath.Rel(root, path)
					sb.WriteString(fmt.Sprintf("%s:%d: %s\n", rel, i+1, strings.TrimSpace(line)))
					if matchCount >= 50 {
						sb.WriteString("Capped at 50 results. Please narrow down your search.\n")
						return io.EOF
					}
				}
			}
		}
		return nil
	})

	if err != nil && err != io.EOF {
		return fmt.Sprintf("Error performing search: %v", err)
	}

	if matchCount == 0 {
		sb.WriteString("(No matches found)\n")
	}
	return sb.String()
}

func (ts *ToolSystem) WebFetch(url string) string {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Sprintf("Error creating request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error fetching URL: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error reading body: %v", err)
	}

	htmlContent := string(bodyBytes)
	
	// Strip script tags
	reScript := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`)
	htmlContent = reScript.ReplaceAllString(htmlContent, "")

	// Strip style tags
	reStyle := regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`)
	htmlContent = reStyle.ReplaceAllString(htmlContent, "")

	// Strip HTML tags
	reTags := regexp.MustCompile(`<[^>]+>`)
	plainText := reTags.ReplaceAllString(htmlContent, " ")

	// Normalize spaces
	reSpaces := regexp.MustCompile(`\s+`)
	plainText = reSpaces.ReplaceAllString(plainText, " ")
	plainText = strings.TrimSpace(plainText)

	if len(plainText) > 8000 {
		plainText = plainText[:8000] + "\n\n[Content truncated due to length limits]"
	}

	return plainText
}
