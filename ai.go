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

type deepseekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepseekRequest struct {
	Model       string             `json:"model"`
	Messages    []deepseekMessage  `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
}

type deepseekResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
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

	req := deepseekRequest{
		Model: "deepseek-chat",
		Messages: []deepseekMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
	}

	return c.doRequest(req)
}

func (c *AIClient) doRequest(req deepseekRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST",
		"https://api.deepseek.com/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("deepseek request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("deepseek request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 429 {
		log.Printf("DeepSeek rate limit, waiting 10s...")
		time.Sleep(10 * time.Second)
		return c.retryRequest(req)
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 {
			return "AI не подключён: неверный API-ключ DeepSeek.", nil
		}
		if resp.StatusCode == 402 {
			return "AI временно недоступен: закончились средства на балансе DeepSeek.", nil
		}
		return "", fmt.Errorf("deepseek error %d: %s", resp.StatusCode, string(respBody))
	}

	var dr deepseekResponse
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return "", fmt.Errorf("deepseek parse: %w", err)
	}

	if len(dr.Choices) == 0 {
		return "AI не дал ответа.", nil
	}

	return dr.Choices[0].Message.Content, nil
}

func (c *AIClient) retryRequest(req deepseekRequest) (string, error) {
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST",
		"https://api.deepseek.com/v1/chat/completions",
		bytes.NewReader(body),
	)
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

	var dr deepseekResponse
	if err := json.Unmarshal(respBody, &dr); err != nil {
		return "AI временно недоступен. Попробуй позже.", nil
	}

	if len(dr.Choices) == 0 {
		return "AI не дал ответа.", nil
	}

	return dr.Choices[0].Message.Content, nil
}
