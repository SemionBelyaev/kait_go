package tg

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type SimpleClient struct {
	ChannelUsername string
}

func NewSimpleClient(username string) *SimpleClient {
	return &SimpleClient{ChannelUsername: username}
}

// Парсинг через t.me (публичный просмотр)
func (sc *SimpleClient) GetRecentPosts(limit int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("https://t.me/s/%s", sc.ChannelUsername)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// Парсим HTML для извлечения постов
	posts := []map[string]interface{}{}

	// Находим блоки постов
	re := regexp.MustCompile(`<div class="tgme_widget_message_text[^>]*>(.*?)</div>`)
	matches := re.FindAllStringSubmatch(html, -1)

	viewsRe := regexp.MustCompile(`<span class="tgme_widget_message_views">([^<]+)</span>`)
	viewsMatches := viewsRe.FindAllStringSubmatch(html, -1)

	for i, match := range matches {
		if i >= limit {
			break
		}

		text := cleanHTML(match[1])
		views := 0

		if i < len(viewsMatches) {
			viewsStr := strings.ReplaceAll(viewsMatches[i][1], "K", "000")
			viewsStr = strings.ReplaceAll(viewsStr, "M", "000000")
			viewsStr = strings.TrimSpace(viewsStr)
			views, _ = strconv.Atoi(viewsStr)
		}

		posts = append(posts, map[string]interface{}{
			"id":    i + 1,
			"text":  text,
			"views": views,
			"date":  time.Now().Unix(),
		})
	}

	return posts, nil
}

func cleanHTML(s string) string {
	// Убираем HTML теги
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}
