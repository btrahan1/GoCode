package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ToolCallPayload struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	Command     string `json:"command"`
	Target      string `json:"target"`
	Replacement string `json:"replacement"`
	StartLine   int    `json:"startLine"`
	EndLine     int    `json:"endLine"`
}

type LLMClient interface {
	GenerateText(systemPrompt, userPrompt string) (string, error)
	GenerateResponse(history []ChatMessage, systemPrompt string) (string, error)
}

type GeminiClient struct {
	apiKey string
	model  string
	client *http.Client
}

func NewGeminiClient(apiKey, model string) *GeminiClient {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (g *GeminiClient) GenerateText(systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.model, g.apiKey)

	reqBody := map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"role": "user",
				"parts": []interface{}{
					map[string]interface{}{
						"text": systemPrompt + "\n\n" + userPrompt,
					},
				},
			},
		},
		"systemInstruction": map[string]interface{}{
			"parts": []interface{}{
				map[string]interface{}{
					"text": systemPrompt,
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := g.client.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no response text generated")
}

func (g *GeminiClient) GenerateResponse(history []ChatMessage, systemPrompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.model, g.apiKey)

	var contents []interface{}
	for _, msg := range history {
		role := msg.Sender
		if role == "assistant" {
			role = "model"
		}
		
		contents = append(contents, map[string]interface{}{
			"role": role,
			"parts": []interface{}{
				map[string]interface{}{
					"text": msg.Text,
				},
			},
		})
	}

	reqBody := map[string]interface{}{
		"contents": contents,
		"systemInstruction": map[string]interface{}{
			"parts": []interface{}{
				map[string]interface{}{
					"text": systemPrompt,
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := g.client.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no response text generated")
}

type OllamaClient struct {
	endpoint string
	model    string
	client   *http.Client
}

func NewOllamaClient(endpoint, model string) *OllamaClient {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &OllamaClient{
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *OllamaClient) GenerateText(systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("%s/api/chat", strings.TrimSuffix(o.endpoint, "/"))

	reqBody := map[string]interface{}{
		"model":  o.model,
		"stream": false,
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": systemPrompt},
			map[string]interface{}{"role": "user", "content": userPrompt},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := o.client.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	return result.Message.Content, nil
}

func (o *OllamaClient) GenerateResponse(history []ChatMessage, systemPrompt string) (string, error) {
	url := fmt.Sprintf("%s/api/chat", strings.TrimSuffix(o.endpoint, "/"))

	var messages []interface{}
	messages = append(messages, map[string]interface{}{"role": "system", "content": systemPrompt})

	for _, msg := range history {
		messages = append(messages, map[string]interface{}{
			"role":    msg.Sender,
			"content": msg.Text,
		})
	}

	reqBody := map[string]interface{}{
		"model":    o.model,
		"stream":   false,
		"messages": messages,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := o.client.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	return result.Message.Content, nil
}

// ParseToolCall parses the tool XML structure out of the assistant response.
func ParseToolCall(text string) *ToolCallPayload {
	// Normalize any DeepSeek-style custom ending tags
	text = regexp.MustCompile(`(?i)</[|｜]{2}DSML[|｜]{2}>`).ReplaceAllString(text, "</tool>")
	text = regexp.MustCompile(`(?i)</DSML>`).ReplaceAllString(text, "</tool>")

	// Find the start tag <tool name="...">
	startRegex := regexp.MustCompile(`(?i)<tool\s+name=["']?([^"'\s>]+)["']?\s*>`)
	startMatch := startRegex.FindStringSubmatchIndex(text)
	if startMatch == nil {
		// Try fallback JSON parsing
		firstBrace := strings.Index(text, "{")
		lastBrace := strings.LastIndex(text, "}")
		if firstBrace != -1 && lastBrace > firstBrace {
			possibleJSON := text[firstBrace : lastBrace+1]
			var fallback struct {
				Name      string `json:"name"`
				Function  string `json:"function"`
				Arguments struct {
					Path        string      `json:"path"`
					Content     string      `json:"content"`
					Command     string      `json:"command"`
					Target      string      `json:"target"`
					Replacement string      `json:"replacement"`
					StartLine   interface{} `json:"start_line"`
					EndLine     interface{} `json:"end_line"`
				} `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(possibleJSON), &fallback); err == nil {
				name := fallback.Name
				if name == "" {
					name = fallback.Function
				}
				if name != "" {
					sl, _ := getInt(fallback.Arguments.StartLine)
					el, _ := getInt(fallback.Arguments.EndLine)
					return &ToolCallPayload{
						Name:        name,
						Path:        fallback.Arguments.Path,
						Content:     fallback.Arguments.Content,
						Command:     fallback.Arguments.Command,
						Target:      fallback.Arguments.Target,
						Replacement: fallback.Arguments.Replacement,
						StartLine:   sl,
						EndLine:     el,
					}
				}
			}
		}
		return nil
	}

	toolName := text[startMatch[2]:startMatch[3]]
	toolStartIdx := startMatch[0]

	// Find the matching </tool> tag that is NOT nested inside tags like <content> or <replacement>
	// Find all <content>, <replacement>, <target> blocks
	nestedRanges := [][]int{}
	tags := []string{"target", "replacement", "content"}
	for _, tag := range tags {
		re := regexp.MustCompile(fmt.Sprintf(`(?i)<%s[^>]*>([\s\S]*?)</%s>`, tag, tag))
		matches := re.FindAllStringIndex(text, -1)
		if matches != nil {
			nestedRanges = append(nestedRanges, matches...)
		}
	}

	searchPos := startMatch[1]
	toolEndIdx := -1

	for {
		endTagIdx := strings.Index(strings.ToLower(text[searchPos:]), "</tool>")
		if endTagIdx == -1 {
			break
		}
		absoluteEndTagIdx := searchPos + endTagIdx

		isNested := false
		for _, r := range nestedRanges {
			if absoluteEndTagIdx > r[0] && absoluteEndTagIdx < r[1] {
				isNested = true
				break
			}
		}

		if !isNested {
			toolEndIdx = absoluteEndTagIdx
			break
		}

		searchPos = absoluteEndTagIdx + 7
	}

	if toolEndIdx == -1 {
		return nil
	}

	toolBlockXML := text[toolStartIdx : toolEndIdx+7]

	getField := func(xml, tag string) string {
		tagRegex := regexp.MustCompile(fmt.Sprintf(`(?i)<%s[^>]*>([\s\S]*?)</%s>`, tag, tag))
		m := tagRegex.FindStringSubmatch(xml)
		if len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}

	sl, _ := strconv.Atoi(getField(toolBlockXML, "start_line"))
	el, _ := strconv.Atoi(getField(toolBlockXML, "end_line"))

	cmd := getField(toolBlockXML, "command")
	if cmd == "" {
		cmd = getField(toolBlockXML, "query")
	}

	return &ToolCallPayload{
		Name:        toolName,
		Path:        getField(toolBlockXML, "path"),
		Content:     getField(toolBlockXML, "content"),
		Command:     cmd,
		Target:      getField(toolBlockXML, "target"),
		Replacement: getField(toolBlockXML, "replacement"),
		StartLine:   sl,
		EndLine:     el,
	}
}

func getInt(val interface{}) (int, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case float64:
		return int(v), true
	case string:
		i, err := strconv.Atoi(v)
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

type OpenCodeZenClient struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

func NewOpenCodeZenClient(apiKey, model, endpoint string) *OpenCodeZenClient {
	return &OpenCodeZenClient{
		apiKey:   apiKey,
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *OpenCodeZenClient) GenerateText(systemPrompt, userPrompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model":  c.model,
		"stream": false,
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": systemPrompt},
			map[string]interface{}{"role": "user", "content": userPrompt},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openCodeZen API error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response text generated")
}

func (c *OpenCodeZenClient) GenerateResponse(history []ChatMessage, systemPrompt string) (string, error) {
	var messages []interface{}
	messages = append(messages, map[string]interface{}{"role": "system", "content": systemPrompt})

	for _, msg := range history {
		messages = append(messages, map[string]interface{}{
			"role":    msg.Sender,
			"content": msg.Text,
		})
	}

	reqBody := map[string]interface{}{
		"model":    c.model,
		"stream":   false,
		"messages": messages,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openCodeZen API error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response text generated")
}

