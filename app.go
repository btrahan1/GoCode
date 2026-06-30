package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	db         *DBClient
	toolSystem *ToolSystem
	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

func NewApp(db *DBClient) *App {
	return &App{
		db:         db,
		toolSystem: NewToolSystem(db.db),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.ImportFreeCodeSettings()
}

func (a *App) ImportFreeCodeSettings() {
	// Only import if settings are empty
	existingKey, _ := a.db.GetSetting("gemini_api_key")
	if existingKey != "" {
		return
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return
	}

	freecodePath := filepath.Join(localAppData, "FreeCode", "settings.json")
	if _, err := os.Stat(freecodePath); err != nil {
		return
	}

	fileBytes, err := os.ReadFile(freecodePath)
	if err != nil {
		return
	}

	var fcSettings struct {
		GeminiApiKey       string   `json:"GeminiApiKey"`
		OpenCodeApiKey     string   `json:"OpenCodeApiKey"`
		OpenRouterApiKey   string   `json:"OpenRouterApiKey"`
		UseNativeToolCalls bool     `json:"UseNativeToolCalls"`
		FavoriteModels     []string `json:"FavoriteModels"`
	}

	if err := json.Unmarshal(fileBytes, &fcSettings); err != nil {
		return
	}

	// Save to DB
	if fcSettings.GeminiApiKey != "" {
		_ = a.db.SaveSetting("gemini_api_key", fcSettings.GeminiApiKey)
	}
	if fcSettings.OpenCodeApiKey != "" {
		_ = a.db.SaveSetting("open_code_api_key", fcSettings.OpenCodeApiKey)
	}
	if fcSettings.OpenRouterApiKey != "" {
		_ = a.db.SaveSetting("open_router_api_key", fcSettings.OpenRouterApiKey)
	}
	if fcSettings.UseNativeToolCalls {
		_ = a.db.SaveSetting("use_native_tool_calls", "true")
	}
	if len(fcSettings.FavoriteModels) > 0 {
		favBytes, _ := json.Marshal(fcSettings.FavoriteModels)
		_ = a.db.SaveSetting("favorite_models", string(favBytes))
	}
}

// SelectWorkspace opens a directory selector dialog.
func (a *App) SelectWorkspace() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Workspace Folder",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Children []*FileNode `json:"children"`
}

func (a *App) GetDirectoryTree(workspacePath string) (*FileNode, error) {
	if workspacePath == "" {
		return nil, fmt.Errorf("workspace path is empty")
	}

	_, err := os.Stat(workspacePath)
	if err != nil {
		return nil, err
	}

	rootNode := &FileNode{
		Name:  filepath.Base(workspacePath),
		Path:  "",
		IsDir: true,
		Children: []*FileNode{},
	}

	var buildTree func(dir string, parentNode *FileNode) error
	buildTree = func(dir string, parentNode *FileNode) error {
		files, err := os.ReadDir(dir)
		if err != nil {
			return err
		}

		for _, file := range files {
			name := file.Name()
			fullPath := filepath.Join(dir, name)
			
			// Use our ToolSystem's IsIgnored logic to filter out noise
			if a.toolSystem.IsIgnored(fullPath, workspacePath) {
				continue
			}

			relPath, err := filepath.Rel(workspacePath, fullPath)
			if err != nil {
				continue
			}

			node := &FileNode{
				Name:  name,
				Path:  relPath,
				IsDir: file.IsDir(),
			}

			if file.IsDir() {
				node.Children = []*FileNode{}
				if err := buildTree(fullPath, node); err != nil {
					return err
				}
			}

			parentNode.Children = append(parentNode.Children, node)
		}
		return nil
	}

	if err := buildTree(workspacePath, rootNode); err != nil {
		return nil, err
	}

	return rootNode, nil
}

func (a *App) GetFileContent(workspacePath string, relPath string) (string, error) {
	fullPath := filepath.Join(workspacePath, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) SaveFileContent(workspacePath string, relPath string, content string) error {
	fullPath := filepath.Join(workspacePath, relPath)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// Settings definition
type AppSettings struct {
	GeminiApiKey        string `json:"geminiApiKey"`
	OllamaEndpoint      string `json:"ollamaEndpoint"`
	OpenCodeApiKey      string `json:"openCodeApiKey"`
	OpenRouterApiKey    string `json:"openRouterApiKey"`
	UseNativeToolCalls  bool   `json:"useNativeToolCalls"`
	SidebarWidth        int    `json:"sidebarWidth"`
	LogPanelWidth       int    `json:"logPanelWidth"`
}

func (a *App) LoadSettings() (AppSettings, error) {
	geminiKey, _ := a.db.GetSetting("gemini_api_key")
	ollamaEnd, _ := a.db.GetSetting("ollama_endpoint")
	openCodeKey, _ := a.db.GetSetting("open_code_api_key")
	openRouterKey, _ := a.db.GetSetting("open_router_api_key")
	useNativeStr, _ := a.db.GetSetting("use_native_tool_calls")
	sidebarWidthStr, _ := a.db.GetSetting("sidebar_width")
	logPanelWidthStr, _ := a.db.GetSetting("log_panel_width")
	
	if ollamaEnd == "" {
		ollamaEnd = "http://localhost:11434"
	}

	sidebarWidth := 280
	if sidebarWidthStr != "" {
		var val int
		if _, err := fmt.Sscanf(sidebarWidthStr, "%d", &val); err == nil {
			sidebarWidth = val
		}
	}

	logPanelWidth := 320
	if logPanelWidthStr != "" {
		var val int
		if _, err := fmt.Sscanf(logPanelWidthStr, "%d", &val); err == nil {
			logPanelWidth = val
		}
	}

	return AppSettings{
		GeminiApiKey:       geminiKey,
		OllamaEndpoint:     ollamaEnd,
		OpenCodeApiKey:     openCodeKey,
		OpenRouterApiKey:   openRouterKey,
		UseNativeToolCalls: useNativeStr == "true",
		SidebarWidth:        sidebarWidth,
		LogPanelWidth:       logPanelWidth,
	}, nil
}

func (a *App) SaveSettings(settings AppSettings) error {
	_ = a.db.SaveSetting("gemini_api_key", settings.GeminiApiKey)
	_ = a.db.SaveSetting("ollama_endpoint", settings.OllamaEndpoint)
	_ = a.db.SaveSetting("open_code_api_key", settings.OpenCodeApiKey)
	_ = a.db.SaveSetting("open_router_api_key", settings.OpenRouterApiKey)
	_ = a.db.SaveSetting("sidebar_width", fmt.Sprintf("%d", settings.SidebarWidth))
	_ = a.db.SaveSetting("log_panel_width", fmt.Sprintf("%d", settings.LogPanelWidth))
	useNativeVal := "false"
	if settings.UseNativeToolCalls {
		useNativeVal = "true"
	}
	return a.db.SaveSetting("use_native_tool_calls", useNativeVal)
}

type OpenWorkspacesResult struct {
	Workspaces      []string `json:"workspaces"`
	ActiveWorkspace string   `json:"activeWorkspace"`
}

func (a *App) GetOpenWorkspaces() (OpenWorkspacesResult, error) {
	wsJSON, _ := a.db.GetSetting("open_workspaces")
	activeWS, _ := a.db.GetSetting("last_active_workspace")

	var list []string
	if wsJSON != "" {
		_ = json.Unmarshal([]byte(wsJSON), &list)
	}

	if list == nil {
		list = []string{}
	}

	return OpenWorkspacesResult{
		Workspaces:      list,
		ActiveWorkspace: activeWS,
	}, nil
}

func (a *App) SaveOpenWorkspaces(workspaces []string, activeWorkspace string) error {
	wsBytes, err := json.Marshal(workspaces)
	if err != nil {
		return err
	}

	_ = a.db.SaveSetting("open_workspaces", string(wsBytes))
	_ = a.db.SaveSetting("last_active_workspace", activeWorkspace)
	return nil
}

type ModelItem struct {
	Name       string `json:"name"`
	IsFavorite bool   `json:"isFavorite"`
}

func (a *App) GetModelList() ([]ModelItem, error) {
	staticModels := []string{
		"gemini-2.5-flash",
		"gemini-2.5-pro",
		"gemini-2.0-flash",
		"gemini-1.5-flash",
		"gemini-1.5-pro",
		"deepseek-v4-flash-free",
		"qwen3.6-plus-free",
		"nemotron-3-ultra-free",
		"big-pickle",
		"deepseek-v4-flash",
		"z-ai/glm-5.2",
		"z-ai/glm-5.1",
		"z-ai/glm-5-turbo",
		"meta-llama/llama-3.1-8b-instruct:free",
		"meta-llama/llama-3.3-70b-instruct:free",
		"qwen/qwen-2.5-72b-instruct:free",
		"qwen/qwen3-coder:free",
		"nousresearch/hermes-3-llama-3.1-405b:free",
		"liquid/lfm-2.5-1.2b-thinking:free",
		"liquid/lfm-2.5-1.2b-instruct:free",
		"openrouter/free",
	}

	// Fetch favorites from database
	favJSON, _ := a.db.GetSetting("favorite_models")
	favorites := make(map[string]bool)
	if favJSON != "" {
		var list []string
		if err := json.Unmarshal([]byte(favJSON), &list); err == nil {
			for _, m := range list {
				favorites[m] = true
			}
		}
	}

	// Discover Ollama models with a short timeout
	discovered := []string{}
	client := &http.Client{Timeout: 2 * time.Second}
	
	// Query endpoint
	resp, err := client.Get("http://127.0.0.1:11434/api/tags")
	if err == nil {
		defer resp.Body.Close()
		var res struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			for _, m := range res.Models {
				discovered = append(discovered, m.Name)
			}
		}
	}

	// Merge static and discovered models (deduplicate)
	seen := make(map[string]bool)
	merged := []string{}
	
	for _, m := range staticModels {
		if !seen[m] {
			seen[m] = true
			merged = append(merged, m)
		}
	}
	for _, m := range discovered {
		if !seen[m] {
			seen[m] = true
			merged = append(merged, m)
		}
	}

	// Build result with favorites sorted to the top
	var favItems []ModelItem
	var normalItems []ModelItem

	for _, m := range merged {
		isFav := favorites[m]
		item := ModelItem{Name: m, IsFavorite: isFav}
		if isFav {
			favItems = append(favItems, item)
		} else {
			normalItems = append(normalItems, item)
		}
	}

	result := append(favItems, normalItems...)
	return result, nil
}

func (a *App) ToggleModelFavorite(modelName string) error {
	favJSON, _ := a.db.GetSetting("favorite_models")
	var list []string
	if favJSON != "" {
		_ = json.Unmarshal([]byte(favJSON), &list)
	}
	if list == nil {
		list = []string{}
	}

	// Toggle existence
	foundIdx := -1
	for idx, m := range list {
		if m == modelName {
			foundIdx = idx
			break
		}
	}

	if foundIdx != -1 {
		// Remove
		list = append(list[:foundIdx], list[foundIdx+1:]...)
	} else {
		// Add
		list = append(list, modelName)
	}

	bytes, err := json.Marshal(list)
	if err != nil {
		return err
	}

	return a.db.SaveSetting("favorite_models", string(bytes))
}

func (a *App) LoadConversation(workspacePath string) (*SavedConversation, error) {
	conv, err := a.db.LoadConversation(workspacePath)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		// Return empty default conversation
		conv = &SavedConversation{
			SessionID:   fmt.Sprintf("session-%d", time.Now().UnixNano()),
			ActiveModel: "gemini-2.5-flash",
			AgentMode:   "coder",
			YoloMode:    false,
			Messages:    []ChatMessage{},
		}
	}
	return conv, nil
}

func (a *App) ListConversations(workspacePath string) ([]SavedConversation, error) {
	return a.db.ListConversations(workspacePath)
}

func (a *App) LoadSpecificConversation(sessionID string) (*SavedConversation, error) {
	return a.db.LoadSpecificConversation(sessionID)
}

func (a *App) CreateNewConversation(workspacePath string, activeModel string, agentMode string, yoloMode bool) (SavedConversation, error) {
	conv := SavedConversation{
		SessionID:   fmt.Sprintf("session-%d", time.Now().UnixNano()),
		ActiveModel: activeModel,
		AgentMode:   agentMode,
		YoloMode:    yoloMode,
		Messages:    []ChatMessage{},
	}
	err := a.db.SaveConversation(workspacePath, &conv)
	return conv, err
}

func (a *App) SaveConversation(workspacePath string, conv SavedConversation) error {
	return a.db.SaveConversation(workspacePath, &conv)
}

func (a *App) DeleteConversation(workspacePath string) error {
	return a.db.DeleteConversation(workspacePath)
}

func (a *App) CancelAgent() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelFunc != nil {
		a.cancelFunc()
		a.cancelFunc = nil
	}
	runtime.EventsEmit(a.ctx, "agent:status", map[string]interface{}{
		"status": "Ready",
		"color":  "green",
	})
	runtime.EventsEmit(a.ctx, "agent:log", "Agent cancelled by user.")
}

func (a *App) SendUserMessage(workspacePath string, sessionID string, messageText string, activeModel string, agentMode string, yoloMode bool) error {
	a.mu.Lock()
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFunc = cancel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.cancelFunc = nil
			a.mu.Unlock()
		}()

		log.Printf("Starting agent execution loop for session: %s", sessionID)

		// 1. Load conversation from DB
		var conv *SavedConversation
		var err error
		if sessionID != "" {
			conv, err = a.db.LoadSpecificConversation(sessionID)
		} else {
			conv, err = a.db.LoadConversation(workspacePath)
		}

		if err != nil {
			runtime.EventsEmit(a.ctx, "agent:error", fmt.Sprintf("Failed to load conversation: %v", err))
			return
		}
		if conv == nil {
			sid := sessionID
			if sid == "" {
				sid = fmt.Sprintf("session-%d", time.Now().UnixNano())
			}
			conv = &SavedConversation{
				SessionID:   sid,
				ActiveModel: activeModel,
				AgentMode:   agentMode,
				YoloMode:    yoloMode,
				Messages:    []ChatMessage{},
			}
		}

		// Update fields
		conv.ActiveModel = activeModel
		conv.AgentMode = agentMode
		conv.YoloMode = yoloMode

		// Append new user message
		userMsg := ChatMessage{Sender: "user", Text: messageText}
		conv.Messages = append(conv.Messages, userMsg)
		_ = a.db.SaveConversation(workspacePath, conv)

		runtime.EventsEmit(a.ctx, "agent:log", fmt.Sprintf("User: %s", messageText))

		// Get LLM credentials
		settings, err := a.LoadSettings()
		if err != nil {
			runtime.EventsEmit(a.ctx, "agent:error", fmt.Sprintf("Failed to load settings: %v", err))
			return
		}

		// Initialize Client
		var client LLMClient
		if strings.Contains(strings.ToLower(activeModel), "gemini") {
			if settings.GeminiApiKey == "" {
				errMsg := "Gemini API Key is not configured in Settings."
				runtime.EventsEmit(a.ctx, "agent:message", ChatMessage{Sender: "assistant", Text: errMsg})
				return
			}
			client = NewGeminiClient(settings.GeminiApiKey, activeModel)
		} else if strings.Contains(activeModel, "/") {
			// OpenRouter API model
			if settings.OpenRouterApiKey == "" {
				errMsg := "OpenRouter API Key is not configured in Settings."
				runtime.EventsEmit(a.ctx, "agent:message", ChatMessage{Sender: "assistant", Text: errMsg})
				return
			}
			client = NewOpenCodeZenClient(settings.OpenRouterApiKey, activeModel, "https://openrouter.ai/api/v1/chat/completions")
		} else if activeModel == "big-pickle" || activeModel == "deepseek-v4-flash-free" || activeModel == "qwen3.6-plus-free" || activeModel == "nemotron-3-ultra-free" || activeModel == "deepseek-v4-flash" {
			// OpenCode Zen API model
			if settings.OpenCodeApiKey == "" {
				errMsg := "OpenCode Zen API Key is not configured in Settings."
				runtime.EventsEmit(a.ctx, "agent:message", ChatMessage{Sender: "assistant", Text: errMsg})
				return
			}
			client = NewOpenCodeZenClient(settings.OpenCodeApiKey, activeModel, "https://opencode.ai/zen/v1/chat/completions")
		} else {
			// Local Ollama model
			client = NewOllamaClient(settings.OllamaEndpoint, activeModel)
		}

		// Main agent execution loop
		for {
			select {
			case <-ctx.Done():
				runtime.EventsEmit(a.ctx, "agent:status", map[string]interface{}{"status": "Ready", "color": "green"})
				return
			default:
			}

			// Generate system instructions containing directory layout
			dirLayout := a.toolSystem.ListDirectory(workspacePath)
			systemPromptRaw := `You are GoCode, an autonomous coding assistant connected to my developer workspace.
You can read and modify files, scan directories, and run commands.
My active workspace folder is: %s

Here is the current directory structure:
%s

### TIGHT TOOLKIT SPECIFICATIONS:
You can invoke the following tools using XML blocks. Output ONLY one tool block at a time, wait for the response (which will be returned as '### TOOL OUTPUT:'), and then decide the next step.

1. List workspace directory files:
___xml
<tool name="list_directory"></tool>
___

2. Read file contents:
___xml
<tool name="read_file">
  <path>relative/path/to/file.ext</path>
  <start_line>10</start_line>
  <end_line>50</end_line>
</tool>
___

3. Write a new file or fully overwrite an existing file:
___xml
<tool name="write_file">
  <path>relative/path/to/file.ext</path>
  <content>
  full file content here
  </content>
</tool>
___

4. Replace a unique text block in an existing file:
___xml
<tool name="replace_text">
  <path>relative/path/to/file.ext</path>
  <start_line>10</start_line>
  <end_line>50</end_line>
  <target>
  exact existing text block to replace
  </target>
  <replacement>
  new text block replacement
  </replacement>
</tool>
___

5. Run a shell command in the terminal:
___xml
<tool name="run_command">
  <command>go build</command>
</tool>
___

6. Execute SQL queries:
___xml
<tool name="execute_sql">
  <command>SELECT * FROM conversations LIMIT 10</command>
</tool>
___

7. Search codebase:
___xml
<tool name="search_code">
  <command>SearchQueryString</command>
</tool>
___

8. Fetch a URL:
___xml
<tool name="web_fetch">
  <command>https://example.com/docs</command>
</tool>
___

### RULES & GUIDELINES:
- **One Action per Message**: Do not combine multiple tool calls in a single message.
- **Wrap in Markdown Code Blocks**: Always wrap your XML tool block in a ___xml ... ___ code block.
- **No placeholders**: Do not use placeholders or comments like '// rest of code remains the same'. You must output full segments.

### YOUR RESPONSE FORMAT:
If you want to use a tool, you MUST output a tool XML block.
If you have finished the task, output a clear wrap-up explanation without any tool blocks.`
			
			systemPrompt := fmt.Sprintf(strings.ReplaceAll(systemPromptRaw, "___", "```"), workspacePath, dirLayout)

			runtime.EventsEmit(a.ctx, "agent:status", map[string]interface{}{"status": "Thinking...", "color": "blue"})

			// Request response from LLM
			resp, err := client.GenerateResponse(conv.Messages, systemPrompt)
			if err != nil {
				runtime.EventsEmit(a.ctx, "agent:message", ChatMessage{Sender: "assistant", Text: fmt.Sprintf("Error: %v", err)})
				runtime.EventsEmit(a.ctx, "agent:status", map[string]interface{}{"status": "Ready", "color": "green"})
				return
			}

			// Check for tool calls
			toolCall := ParseToolCall(resp)
			if toolCall != nil {
				// Emit the assistant thinking/tool block text to frontend
				runtime.EventsEmit(a.ctx, "agent:message", ChatMessage{Sender: "assistant", Text: resp})
				conv.Messages = append(conv.Messages, ChatMessage{Sender: "assistant", Text: resp})
				_ = a.db.SaveConversation(workspacePath, conv)

				// Log and status update
				runtime.EventsEmit(a.ctx, "agent:status", map[string]interface{}{"status": fmt.Sprintf("Executing %s...", toolCall.Name), "color": "orange"})
				runtime.EventsEmit(a.ctx, "agent:log", fmt.Sprintf("Executing Tool: %s (Path: %s)", toolCall.Name, toolCall.Path))

				// Execute tool
				var toolOutput string
				switch toolCall.Name {
				case "list_directory":
					toolOutput = a.toolSystem.ListDirectory(workspacePath)
				case "read_file":
					fullPath := filepath.Join(workspacePath, toolCall.Path)
					toolOutput = a.toolSystem.ReadFile(fullPath, toolCall.StartLine, toolCall.EndLine)
				case "write_file":
					fullPath := filepath.Join(workspacePath, toolCall.Path)
					toolOutput = a.toolSystem.WriteFile(fullPath, toolCall.Content)
				case "replace_text":
					fullPath := filepath.Join(workspacePath, toolCall.Path)
					toolOutput = a.toolSystem.ReplaceText(fullPath, toolCall.Target, toolCall.Replacement, toolCall.StartLine, toolCall.EndLine)
				case "run_command":
					runtime.EventsEmit(a.ctx, "agent:log", fmt.Sprintf("Running shell command: %s", toolCall.Command))
					cmdOut, err := a.toolSystem.RunCommand(ctx, toolCall.Command, workspacePath, func(stream string) {
						runtime.EventsEmit(a.ctx, "agent:log_stream", stream)
					})
					if err != nil {
						toolOutput = fmt.Sprintf("Command failed with error: %v\nOutput: %s", err, cmdOut)
					} else {
						toolOutput = cmdOut
					}
				case "execute_sql":
					toolOutput = a.toolSystem.ExecuteSQL(toolCall.Command)
				case "search_code":
					toolOutput = a.toolSystem.SearchCode(workspacePath, toolCall.Command)
				case "web_fetch":
					toolOutput = a.toolSystem.WebFetch(toolCall.Command)
				default:
					toolOutput = fmt.Sprintf("Error: Unknown tool '%s'", toolCall.Name)
				}

				runtime.EventsEmit(a.ctx, "agent:log", fmt.Sprintf("Tool Output finished (%d chars).", len(toolOutput)))

				// Format tool response back to LLM
				toolResponseFormatted := fmt.Sprintf("### TOOL OUTPUT:\n%s\n\nProceed to next step.", toolOutput)
				
				// Collapse obsolete file history if tool is related to files
				if toolCall.Name == "read_file" || toolCall.Name == "write_file" || toolCall.Name == "replace_text" {
					conv.Messages = a.CollapseObsoleteFileHistory(conv.Messages, toolCall.Path)
				}

				// Add tool output to conversation history
				toolUserMsg := ChatMessage{Sender: "user", Text: toolResponseFormatted}
				conv.Messages = append(conv.Messages, toolUserMsg)
				_ = a.db.SaveConversation(workspacePath, conv)

				// Emit tool output representation to frontend
				runtime.EventsEmit(a.ctx, "agent:message", toolUserMsg)

				// Loop again for next model call
				continue
			}

			// No tool call: Model completed its response
			runtime.EventsEmit(a.ctx, "agent:message", ChatMessage{Sender: "assistant", Text: resp})
			conv.Messages = append(conv.Messages, ChatMessage{Sender: "assistant", Text: resp})
			_ = a.db.SaveConversation(workspacePath, conv)

			runtime.EventsEmit(a.ctx, "agent:status", map[string]interface{}{"status": "Ready", "color": "green"})
			runtime.EventsEmit(a.ctx, "agent:complete", nil)
			break
		}
	}()

	return nil
}

func (a *App) CollapseObsoleteFileHistory(messages []ChatMessage, relPath string) []ChatMessage {
	if relPath == "" {
		return messages
	}
	normTarget := strings.ToLower(strings.Trim(strings.ReplaceAll(relPath, "\\", "/"), "/"))

	for i := 1; i < len(messages); i++ {
		msg := messages[i]
		if msg.Sender != "user" || !strings.HasPrefix(msg.Text, "### TOOL OUTPUT:") {
			continue
		}

		prevMsg := messages[i-1]
		if prevMsg.Sender != "assistant" {
			continue
		}

		toolCall := ParseToolCall(prevMsg.Text)
		if toolCall == nil {
			continue
		}

		normToolPath := strings.ToLower(strings.Trim(strings.ReplaceAll(toolCall.Path, "\\", "/"), "/"))
		if normToolPath == normTarget {
			if toolCall.Name == "read_file" || toolCall.Name == "write_file" || toolCall.Name == "replace_text" {
				messages[i].Text = fmt.Sprintf("### TOOL OUTPUT:\n[Obsolete file content of '%s' collapsed to save context]", toolCall.Path)
			}
		}
	}
	return messages
}

func (a *App) OpenPathInExplorer(workspacePath, relPath string) error {
	fullPath := filepath.Clean(filepath.Join(workspacePath, relPath))
	
	info, err := os.Stat(fullPath)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if info.IsDir() {
		cmd = exec.Command("explorer.exe", fullPath)
	} else {
		cmd = exec.Command("explorer.exe", "/select,", fullPath)
	}
	return cmd.Run()
}


