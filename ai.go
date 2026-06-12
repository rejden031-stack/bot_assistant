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
	aiEndpoint   = "https://openrouter.ai/api/v1/chat/completions"
	aiModel      = "google/gemma-4-31b-it:free"
	maxRetries   = 3
	retryDelay   = 5 * time.Second
	requestLimit = time.Minute
)

type AIClient struct {
	apiKey     string
	client     *http.Client
	mu         sync.Mutex
	lastReq    time.Time
}

func NewAIClient(apiKey string) *AIClient {
	if apiKey == "" {
		return nil
	}
	return &AIClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
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

	elapsed := time.Since(c.lastReq)
	if elapsed < requestLimit {
		time.Sleep(requestLimit - elapsed)
	}

	body, _ := json.Marshal(req)

	for attempt := 0; attempt <= maxRetries; attempt++ {
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

		if resp.StatusCode == 200 {
			c.lastReq = time.Now()

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

		log.Printf("AI attempt %d/%d: status %d", attempt+1, maxRetries+1, resp.StatusCode)

		if resp.StatusCode == 401 {
			return "AI не подключён: неверный API-ключ.", nil
		}

		if resp.StatusCode == 429 && attempt < maxRetries {
			wait := retryDelay * (1 << attempt)
			log.Printf("AI rate limit, waiting %v...", wait)
			time.Sleep(wait)
			continue
		}

		var errResp aiResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "AI: " + errResp.Error.Message, nil
		}
		return "", fmt.Errorf("ai error %d: %s", resp.StatusCode, string(respBody))
	}

	return "AI временно недоступен. Попробуй позже.", nil
}
