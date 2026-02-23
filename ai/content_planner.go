package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ContentPlanner struct {
	apiKey string
	token  string
}

func NewContentPlanner(apiKey string) *ContentPlanner {
	return &ContentPlanner{
		apiKey: apiKey,
	}
}

type ContentPlanRequest struct {
	GroupName      string
	GroupTheme     string
	RecentPosts    []string
	AdditionalInfo string
	DaysCount      int
}

type DayPlan struct {
	Day      string
	Time     string
	Theme    string
	Text     string
	Hashtags string
	MediaTip string
}

// Получение токена GigaChat
func (cp *ContentPlanner) getToken() error {
	if cp.token != "" {
		return nil
	}

	req, _ := http.NewRequest("POST", "https://ngw.devices.sberbank.ru:9443/api/v2/oauth",
		bytes.NewBuffer([]byte("scope=GIGACHAT_API_PERS")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", fmt.Sprintf("%d", time.Now().Unix()))
	req.SetBasicAuth(cp.apiKey, "")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	cp.token = result.AccessToken
	return nil
}

func (cp *ContentPlanner) GenerateContentPlan(req ContentPlanRequest) ([]DayPlan, error) {
	if err := cp.getToken(); err != nil {
		return nil, fmt.Errorf("ошибка авторизации: %v", err)
	}

	recentPostsText := ""
	if len(req.RecentPosts) > 0 {
		recentPostsText = "Последние посты:\n" + strings.Join(req.RecentPosts, "\n---\n")
	}

	days := []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}
	if req.DaysCount < len(days) {
		days = days[:req.DaysCount]
	}

	prompt := fmt.Sprintf(`Создай контент-план для VK группы "%s" (тематика: %s).

%s

Дополнительно: %s

Создай план на %d дней. Для КАЖДОГО дня строго в формате:

ДЕНЬ: [название]
ВРЕМЯ: [время публикации, например 10:00]
ТЕМА: [тема поста]
ТЕКСТ: [готовый текст 100-250 символов с эмодзи]
ХЕШТЕГИ: #хештег1 #хештег2 #хештег3
ВИЗУАЛ: [рекомендация по фото]
---

Дни: %s`,
		req.GroupName, req.GroupTheme, recentPostsText, req.AdditionalInfo,
		req.DaysCount, strings.Join(days, ", "))

	body := map[string]interface{}{
		"model": "GigaChat",
		"messages": []map[string]string{
			{"role": "system", "content": "Ты - SMM-специалист с опытом ведения групп ВКонтакте."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.8,
	}

	jsonBody, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", "https://gigachat.devices.sberbank.ru/api/v1/chat/completions",
		bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cp.token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ошибка API GigaChat: %s", string(bodyBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("пустой ответ от GigaChat")
	}

	content := result.Choices[0].Message.Content
	return parseContentPlan(content, days), nil
}

func parseContentPlan(content string, days []string) []DayPlan {
	plans := []DayPlan{}
	blocks := strings.Split(content, "---")

	for i, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		plan := DayPlan{}
		if i < len(days) {
			plan.Day = days[i]
		}

		lines := strings.Split(block, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ДЕНЬ:") {
				plan.Day = strings.TrimSpace(strings.TrimPrefix(line, "ДЕНЬ:"))
			} else if strings.HasPrefix(line, "ВРЕМЯ:") {
				plan.Time = strings.TrimSpace(strings.TrimPrefix(line, "ВРЕМЯ:"))
			} else if strings.HasPrefix(line, "ТЕМА:") {
				plan.Theme = strings.TrimSpace(strings.TrimPrefix(line, "ТЕМА:"))
			} else if strings.HasPrefix(line, "ТЕКСТ:") {
				plan.Text = strings.TrimSpace(strings.TrimPrefix(line, "ТЕКСТ:"))
			} else if strings.HasPrefix(line, "ХЕШТЕГИ:") {
				plan.Hashtags = strings.TrimSpace(strings.TrimPrefix(line, "ХЕШТЕГИ:"))
			} else if strings.HasPrefix(line, "ВИЗУАЛ:") {
				plan.MediaTip = strings.TrimSpace(strings.TrimPrefix(line, "ВИЗУАЛ:"))
			}
		}

		if plan.Day != "" || plan.Text != "" {
			plans = append(plans, plan)
		}
	}

	return plans
}
