package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	aiEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	aiModel    = "meta-llama/llama-3.3-70b-instruct"
)

type AIClient struct {
	apiKey string
	client *http.Client
	mu     sync.Mutex
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

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiRequest struct {
	Model    string      `json:"model"`
	Messages []aiMessage `json:"messages"`
}

type aiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *AIClient) Ask(prompt string, notes []Note) (string, error) {
	system := "Ты полезный ассистент. Отвечай кратко и по делу."
	if len(notes) > 0 {
		system += "\n\nЗаметки пользователя:\n"
		for _, n := range notes {
			system += fmt.Sprintf("- #%d: %s\n", n.ID, n.Text)
		}
		system += "\nЕсли вопрос про заметки — используй эту информацию."
	}

	req := aiRequest{
		Model: aiModel,
		Messages: []aiMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
	}

	return c.doRequest(req)
}

func (c *AIClient) doRequest(req aiRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", aiEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ai request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 429 {
		log.Printf("AI rate limit, waiting 10s...")
		time.Sleep(10 * time.Second)
		return c.retryRequest(req)
	}

	if resp.StatusCode != 200 {
		var errResp aiResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "AI: " + errResp.Error.Message, nil
		}
		if resp.StatusCode == 401 {
			return "AI не подключён: неверный API-ключ.", nil
		}
		return "", fmt.Errorf("ai error %d: %s", resp.StatusCode, string(respBody))
	}

	var ar aiResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return "", fmt.Errorf("ai parse: %w", err)
	}

	if ar.Error != nil {
		return "AI: " + ar.Error.Message, nil
	}

	if len(ar.Choices) == 0 {
		return "AI не дал ответа.", nil
	}

	return ar.Choices[0].Message.Content, nil
}

func (c *AIClient) retryRequest(req aiRequest) (string, error) {
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", aiEndpoint, bytes.NewReader(body))
	if err != nil {
		return "AI временно недоступен. Попробуй позже.", nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "AI временно недоступен. Попробуй позже.", nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "AI временно недоступен. Попробуй позже.", nil
	}

	var ar aiResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return "AI временно недоступен. Попробуй позже.", nil
	}

	if len(ar.Choices) == 0 {
		return "AI не дал ответа.", nil
	}

	return ar.Choices[0].Message.Content, nil
}
