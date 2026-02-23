package vk

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apiURL     = "https://api.vk.com/method/"
	apiVersion = "5.131"
)

type Client struct {
	AccessToken string
	httpClient  *http.Client
}

type Group struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Employee struct {
	ID     int
	Name   string
	URL    string
	Domain string
}

type Post struct {
	ID       int    `json:"id"`
	Date     int    `json:"date"`
	Text     string `json:"text"`
	Views    Views  `json:"views"`
	Likes    Count  `json:"likes"`
	Reposts  Count  `json:"reposts"`
	Comments Count  `json:"comments"`
}

type Views struct {
	Count int `json:"count"`
}

type Count struct {
	Count int `json:"count"`
}

type ActivityStats struct {
	Likes   int
	Reposts int
	Total   int
}

func NewClient(token string) *Client {
	tr := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        100,              // ← УСКОРЕНИЕ
		MaxIdleConnsPerHost: 100,              // ← УСКОРЕНИЕ
		IdleConnTimeout:     90 * time.Second, // ← УСКОРЕНИЕ
	}
	return &Client{
		AccessToken: token,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second, // ← УСКОРЕНИЕ (было 20)
		},
	}
}

func (c *Client) makeRequest(method string, params url.Values) ([]byte, error) {
	params.Set("access_token", c.AccessToken)
	params.Set("v", apiVersion)

	resp, err := c.httpClient.Get(apiURL + method + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) GetGroupByDomain(domain string) (*Group, error) {
	params := url.Values{}
	params.Set("group_id", domain)

	body, err := c.makeRequest("groups.getById", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Response []Group `json:"response"`
	}
	json.Unmarshal(body, &result)

	if len(result.Response) == 0 {
		return nil, fmt.Errorf("группа не найдена")
	}
	return &result.Response[0], nil
}

func (c *Client) GetEmployees(screenNames []string) (map[int]Employee, error) {
	params := url.Values{}
	params.Set("user_ids", strings.Join(screenNames, ","))
	params.Set("fields", "domain")

	body, err := c.makeRequest("users.get", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Response []struct {
			ID        int    `json:"id"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Domain    string `json:"domain"`
		} `json:"response"`
	}
	json.Unmarshal(body, &result)

	employees := make(map[int]Employee)
	for _, u := range result.Response {
		domain := u.Domain
		if domain == "" {
			domain = fmt.Sprintf("id%d", u.ID)
		}
		employees[u.ID] = Employee{
			ID:     u.ID,
			Name:   fmt.Sprintf("%s %s", u.FirstName, u.LastName),
			URL:    fmt.Sprintf("https://vk.com/%s", domain),
			Domain: domain,
		}
	}
	return employees, nil
}

func (c *Client) GetWallPosts(ownerID, count int) ([]Post, error) {
	return c.GetWallPostsWithOffset(ownerID, count, 0)
}

func (c *Client) GetWallPostsWithOffset(ownerID, count, offset int) ([]Post, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.Itoa(ownerID))
	params.Set("count", strconv.Itoa(count))
	params.Set("offset", strconv.Itoa(offset))

	body, err := c.makeRequest("wall.get", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Response struct {
			Items []Post `json:"items"`
		} `json:"response"`
	}
	json.Unmarshal(body, &result)
	return result.Response.Items, nil
}

func (c *Client) GetLikes(ownerID, itemID int) ([]int, error) {
	params := url.Values{}
	params.Set("type", "post")
	params.Set("owner_id", strconv.Itoa(ownerID))
	params.Set("item_id", strconv.Itoa(itemID))
	params.Set("count", "1000")

	body, _ := c.makeRequest("likes.getList", params)

	var result struct {
		Response struct {
			Items []int `json:"items"`
		} `json:"response"`
	}
	json.Unmarshal(body, &result)
	return result.Response.Items, nil
}

func (c *Client) GetReposts(ownerID, postID int) ([]int, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.Itoa(ownerID))
	params.Set("post_id", strconv.Itoa(postID))
	params.Set("count", "1000")

	body, _ := c.makeRequest("wall.getReposts", params)

	var result struct {
		Response struct {
			Profiles []struct {
				ID int `json:"id"`
			} `json:"profiles"`
		} `json:"response"`
	}
	json.Unmarshal(body, &result)

	ids := []int{}
	for _, p := range result.Response.Profiles {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

// ← ГЛАВНОЕ УСКОРЕНИЕ: с 3 до 8 параллельных запросов + батчинг
func (c *Client) GetLikesAndRepostsParallel(ownerID int, postIDs []int) (map[int][]int, map[int][]int) {
	likesMap := make(map[int][]int)
	repostsMap := make(map[int][]int)

	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 8) // ← БЫЛО 3, СТАЛО 8

	for _, postID := range postIDs {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Параллельно запрашиваем лайки и репосты одновременно
			var likesResult []int
			var repostsResult []int
			var wgInner sync.WaitGroup
			wgInner.Add(2)

			go func() {
				defer wgInner.Done()
				likesResult, _ = c.GetLikes(ownerID, id)
			}()

			go func() {
				defer wgInner.Done()
				repostsResult, _ = c.GetReposts(ownerID, id)
			}()

			wgInner.Wait()

			mu.Lock()
			likesMap[id] = likesResult
			repostsMap[id] = repostsResult
			mu.Unlock()
		}(postID)
	}

	wg.Wait()
	return likesMap, repostsMap
}
