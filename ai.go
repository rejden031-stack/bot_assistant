package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AIClient struct {
	apiKey string
	client *http.Client
}

func NewAIClient(apiKey string) *AIClient {
	if apiKey == "" {
		return nil
	}
	return &AIClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (c *AIClient) ask(prompt string, notes []Note, systemText string) (string, error) {
	system := systemText
	if system == "" {
		system = "Ты полезный ассистент. Отвечай кратко и по делу."
	}
	if len(notes) > 0 {
		system += "\n\nЗаметки пользователя:\n"
		for _, n := range notes {
			system += fmt.Sprintf("- #%d: %s\n", n.ID, n.Text)
		}
		system += "\nЕсли вопрос про заметки — используй эту информацию."
	}

	req := geminiRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: system + "\n\n" + prompt}}},
		},
	}

	return c.doRequest(req)
}

func (c *AIClient) transcribe(audioData []byte) (string, error) {
	encoded := base64.StdEncoding.EncodeToString(audioData)

	req := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{InlineData: &geminiInlineData{MimeType: "audio/ogg", Data: encoded}},
					{Text: "Распознай и верни только текст голосового сообщения. Без лишних слов."},
				},
			},
		},
	}

	return c.doRequest(req)
}

func (c *AIClient) doRequest(req geminiRequest) (string, error) {
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST",
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key="+c.apiKey,
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("gemini error %d: %s", resp.StatusCode, string(respBody))
	}

	var gr geminiResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return "", fmt.Errorf("gemini parse: %w", err)
	}

	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return "AI не дал ответа.", nil
	}

	return gr.Candidates[0].Content.Parts[0].Text, nil
}
