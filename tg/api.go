package tg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const apiURL = "https://api.telegram.org/bot"

type Client struct {
	BotToken  string
	ChannelID string
}

type Channel struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type Employee struct {
	ID       int64
	Username string
	Name     string
}

type Post struct {
	MessageID int       `json:"message_id"`
	Date      int       `json:"date"`
	Text      string    `json:"text"`
	Views     int       `json:"views"`
	Forwards  int       `json:"forwards"`
	Reactions Reactions `json:"reactions"`
}

type Reactions struct {
	TotalCount int `json:"total_count"`
}

type ActivityStats struct {
	Reactions int
	Forwards  int
	Total     int
}

func NewClient(botToken, channelID string) *Client {
	return &Client{
		BotToken:  botToken,
		ChannelID: channelID,
	}
}

func (c *Client) makeRequest(method string, params url.Values) ([]byte, error) {
	fullURL := apiURL + c.BotToken + "/" + method + "?" + params.Encode()

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *Client) GetChannel() (*Channel, error) {
	params := url.Values{}
	params.Set("chat_id", c.ChannelID)

	body, err := c.makeRequest("getChat", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OK     bool    `json:"ok"`
		Result Channel `json:"result"`
	}
	json.Unmarshal(body, &result)

	if !result.OK {
		return nil, fmt.Errorf("канал не найден")
	}

	return &result.Result, nil
}

func (c *Client) GetEmployees(usernames []string) (map[int64]Employee, error) {
	employees := make(map[int64]Employee)

	// Для Telegram нужно вручную задать ID пользователей
	// или получить их через упоминания в постах
	for i, username := range usernames {
		employees[int64(i+1)] = Employee{
			ID:       int64(i + 1),
			Username: username,
			Name:     username,
		}
	}

	return employees, nil
}

func (c *Client) GetChannelPosts(limit int) ([]Post, error) {
	// Telegram API не предоставляет прямого метода для получения истории постов канала
	// Используем костыль: получаем последние сообщения через updates
	// В реальном приложении нужно использовать MTProto или сохранять посты в БД

	posts := []Post{}

	// Получаем последние обновления (последние 100 сообщений)
	params := url.Values{}
	params.Set("chat_id", c.ChannelID)
	params.Set("limit", strconv.Itoa(limit))

	// ВАЖНО: Telegram Bot API не дает историю канала напрямую
	// Нужно использовать либо:
	// 1. Telethon/Pyrogram (Python)
	// 2. TDLib
	// 3. Сохранять посты в БД при получении через webhook

	// Для демонстрации вернем пустой массив
	// В production используй одно из решений выше

	return posts, nil
}

// Альтернативный метод: получение конкретного поста
func (c *Client) GetPost(messageID int) (*Post, error) {
	params := url.Values{}
	params.Set("chat_id", c.ChannelID)
	params.Set("message_id", strconv.Itoa(messageID))

	body, err := c.makeRequest("getUpdates", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OK     bool `json:"ok"`
		Result Post `json:"result"`
	}
	json.Unmarshal(body, &result)

	if !result.OK {
		return nil, fmt.Errorf("пост не найден")
	}

	return &result.Result, nil
}

// Получение реакций (доступно только для супергрупп/каналов с включенными реакциями)
func (c *Client) GetReactions(messageID int) ([]int64, error) {
	// Telegram Bot API не предоставляет информацию о том, кто поставил реакцию
	// Можно получить только общее количество через getUpdates
	return []int64{}, nil
}

// Параллельное получение данных
func (c *Client) GetPostsDataParallel(messageIDs []int) (map[int]*Post, error) {
	postsMap := make(map[int]*Post)

	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 5)

	for _, msgID := range messageIDs {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			post, _ := c.GetPost(id)

			mu.Lock()
			postsMap[id] = post
			mu.Unlock()
		}(msgID)
	}

	wg.Wait()
	return postsMap, nil
}
