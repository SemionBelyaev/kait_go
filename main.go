package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"smm-helper/cache"
	"smm-helper/vk"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	VK_ACCESS_TOKEN = "vk1.a.G3mh1mFkWfQl50xF98rovwQ9njEx0T-GDgVwyV_IaejuVRvAOI-ROxtBV19IASrIyTmvA3aaRfJo9N8rVo6aecB9p61w3-8CdF3wqRrVSDaQXdvaQ8Kd_xnSBUtjxXyMT274YZvDunCJqUWdG_WpW8qm3jOpF4cwAOKQ6DdNlNWaOQmpdxpoT7LBEh46PgdXtQ6FT4BW2AISZlH5vkVwVQ"
	GROUP_DOMAIN    = "kait_20_official"
)

var (
	vkClient  *vk.Client
	dataCache *cache.Cache
	groupID   int
	groupName string
	employees = []string{
		"kozhan_vi", "id50311017", "idlinkinpark", "id138790792",
		"starostaandrey", "id206710878", "id313673888",
		"fishka074", "iamkatekey", "yara.timofeeva",
	}
	employeeData map[int]vk.Employee
)

func init() {
	vkClient = vk.NewClient(VK_ACCESS_TOKEN)
	dataCache = cache.NewCache()

	group, err := vkClient.GetGroupByDomain(GROUP_DOMAIN)
	if err != nil {
		log.Fatal("Ошибка получения группы: ", err)
	}
	groupID = -group.ID
	groupName = group.Name

	employeeData, err = vkClient.GetEmployees(employees)
	if err != nil {
		log.Fatal("Ошибка получения сотрудников: ", err)
	}

	fmt.Printf("✅ Группа: %s (ID: %d)\n", groupName, group.ID)
	fmt.Printf("✅ Сотрудников: %d\n", len(employeeData))
	fmt.Println("✅ Кэширование включено (5 минут)")
}

func main() {
	r := mux.NewRouter()

	// VK роуты
	r.HandleFunc("/", indexHandler).Methods("GET")
	r.HandleFunc("/employee_activity", employeeActivityHandler).Methods("GET", "POST")
	r.HandleFunc("/posts_analysis", postsAnalysisHandler).Methods("GET", "POST")
	r.HandleFunc("/date_range", dateRangeHandler).Methods("GET", "POST")
	r.HandleFunc("/clear_cache", clearCacheHandler).Methods("GET")

	// TELEGRAM роуты ← ДОБАВЬ ЭТО
	r.HandleFunc("/tg", tgIndexHandler).Methods("GET")
	r.HandleFunc("/tg/posts_analysis", tgPostsAnalysisHandler).Methods("GET")

	compressed := handlers.CompressHandler(r)
	logged := handlers.LoggingHandler(os.Stdout, compressed)

	fmt.Println("🚀 Сервер запущен на http://localhost:8080")
	fmt.Println("📱 VK: http://localhost:8080")
	fmt.Println("✈️  Telegram: http://localhost:8080/tg")
	log.Fatal(http.ListenAndServe(":8080", logged))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, map[string]string{
		"GroupName": groupName,
		"GroupURL":  "https://vk.com/" + GROUP_DOMAIN,
	})
}

func employeeActivityHandler(w http.ResponseWriter, r *http.Request) {
	count := 30
	if r.Method == "POST" {
		c, _ := strconv.Atoi(r.FormValue("n"))
		if c > 0 && c <= 100 {
			count = c
		}
	}

	cacheKey := fmt.Sprintf("employee_activity_%d", count)

	if cached, found := dataCache.Get(cacheKey); found {
		fmt.Printf("📦 Из кэша (%d постов)\n", count)
		tmpl := template.Must(template.ParseFiles("templates/employee_activity.html"))
		tmpl.Execute(w, cached)
		return
	}

	fmt.Printf("🔄 Загрузка с VK (%d постов)...\n", count)
	startTime := time.Now()

	posts, err := vkClient.GetWallPosts(groupID, count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	postIDs := []int{}
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}

	likesMap, repostsMap := vkClient.GetLikesAndRepostsParallel(groupID, postIDs)

	activity := make(map[int][]string)
	postDates := []string{}
	postLinks := []string{}
	totals := make(map[int]vk.ActivityStats)

	for _, post := range posts {
		postDate := time.Unix(int64(post.Date), 0).Format("02.01")
		postLink := fmt.Sprintf("https://vk.com/wall%d_%d", groupID, post.ID)
		postDates = append(postDates, postDate)
		postLinks = append(postLinks, postLink)

		likes := likesMap[post.ID]
		reposts := repostsMap[post.ID]

		likeSet := make(map[int]bool)
		for _, id := range likes {
			likeSet[id] = true
		}
		repostSet := make(map[int]bool)
		for _, id := range reposts {
			repostSet[id] = true
		}

		for empID := range employeeData {
			hasLike := likeSet[empID]
			hasRepost := repostSet[empID]

			var symbol string
			if hasLike && hasRepost {
				symbol = "❤️🔁"
			} else if hasLike {
				symbol = "❤️"
			} else if hasRepost {
				symbol = "🔁"
			} else {
				symbol = "➖"
			}

			activity[empID] = append(activity[empID], symbol)

			stats := totals[empID]
			if hasLike {
				stats.Likes++
			}
			if hasRepost {
				stats.Reposts++
			}
			stats.Total = stats.Likes + stats.Reposts
			totals[empID] = stats
		}
	}

	type empData struct {
		Employee vk.Employee
		Activity []string
		Stats    vk.ActivityStats
	}
	var data []empData
	for empID, emp := range employeeData {
		data = append(data, empData{
			Employee: emp,
			Activity: activity[empID],
			Stats:    totals[empID],
		})
	}

	for i := 0; i < len(data)-1; i++ {
		for j := 0; j < len(data)-i-1; j++ {
			if data[j].Stats.Total < data[j+1].Stats.Total {
				data[j], data[j+1] = data[j+1], data[j]
			}
		}
	}

	result := map[string]interface{}{
		"Data":      data,
		"PostDates": postDates,
		"PostLinks": postLinks,
		"N":         count,
	}

	dataCache.Set(cacheKey, result, 5*time.Minute)
	fmt.Printf("✅ Загружено за %v\n", time.Since(startTime))

	tmpl := template.Must(template.ParseFiles("templates/employee_activity.html"))
	tmpl.Execute(w, result)
}

func postsAnalysisHandler(w http.ResponseWriter, r *http.Request) {
	count := 30
	if r.Method == "POST" {
		c, _ := strconv.Atoi(r.FormValue("n"))
		if c > 0 && c <= 100 {
			count = c
		}
	}

	cacheKey := fmt.Sprintf("posts_analysis_%d", count)

	if cached, found := dataCache.Get(cacheKey); found {
		tmpl := template.Must(template.ParseFiles("templates/posts_analysis.html"))
		tmpl.Execute(w, cached)
		return
	}

	posts, err := vkClient.GetWallPosts(groupID, count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type postStat struct {
		Date     string
		Link     string
		Text     string
		Views    int
		Likes    int
		Reposts  int
		Comments int
	}

	stats := []postStat{}
	totalViews, totalLikes, totalReposts, totalComments := 0, 0, 0, 0

	for _, p := range posts {
		date := time.Unix(int64(p.Date), 0).Format("02.01.2006 15:04")
		link := fmt.Sprintf("https://vk.com/wall%d_%d", groupID, p.ID)
		text := p.Text
		if len(text) > 150 {
			text = text[:150] + "..."
		}

		stats = append(stats, postStat{
			Date:     date,
			Link:     link,
			Text:     text,
			Views:    p.Views.Count,
			Likes:    p.Likes.Count,
			Reposts:  p.Reposts.Count,
			Comments: p.Comments.Count,
		})

		totalViews += p.Views.Count
		totalLikes += p.Likes.Count
		totalReposts += p.Reposts.Count
		totalComments += p.Comments.Count
	}

	result := map[string]interface{}{
		"Stats": stats,
		"N":     count,
		"Totals": map[string]int{
			"Views":    totalViews,
			"Likes":    totalLikes,
			"Reposts":  totalReposts,
			"Comments": totalComments,
		},
	}

	dataCache.Set(cacheKey, result, 30*time.Minute)

	tmpl := template.Must(template.ParseFiles("templates/posts_analysis.html"))
	tmpl.Execute(w, result)
}

func dateRangeHandler(w http.ResponseWriter, r *http.Request) {
	var report map[string]interface{}

	if r.Method == "POST" {
		dateFrom := r.FormValue("date_from")
		dateTo := r.FormValue("date_to")

		startDate, err1 := time.Parse("02.01.2006", dateFrom)
		endDate, err2 := time.Parse("02.01.2006", dateTo)

		if err1 != nil || err2 != nil {
			report = map[string]interface{}{"Error": "Неверный формат даты (ДД.ММ.ГГГГ)"}
		} else {
			endDate = endDate.Add(23*time.Hour + 59*time.Minute)

			allPosts := []vk.Post{}
			offset := 0
			for {
				posts, err := vkClient.GetWallPostsWithOffset(groupID, 100, offset)
				if err != nil || len(posts) == 0 {
					break
				}

				shouldBreak := false
				for _, post := range posts {
					postTime := time.Unix(int64(post.Date), 0)
					if postTime.After(endDate) {
						continue
					}
					if postTime.Before(startDate) {
						shouldBreak = true
						break
					}
					allPosts = append(allPosts, post)
				}

				if shouldBreak {
					break
				}
				offset += 100
			}

			type postStat struct {
				Date, Link, Text                string
				Views, Likes, Reposts, Comments int
			}

			stats := []postStat{}
			totalViews, totalLikes, totalReposts, totalComments := 0, 0, 0, 0

			for _, p := range allPosts {
				text := p.Text
				if len(text) > 150 {
					text = text[:150] + "..."
				}

				stats = append(stats, postStat{
					Date:     time.Unix(int64(p.Date), 0).Format("02.01.2006 15:04"),
					Link:     fmt.Sprintf("https://vk.com/wall%d_%d", groupID, p.ID),
					Text:     text,
					Views:    p.Views.Count,
					Likes:    p.Likes.Count,
					Reposts:  p.Reposts.Count,
					Comments: p.Comments.Count,
				})

				totalViews += p.Views.Count
				totalLikes += p.Likes.Count
				totalReposts += p.Reposts.Count
				totalComments += p.Comments.Count
			}

			avg := func(total, count int) int {
				if count == 0 {
					return 0
				}
				return total / count
			}

			report = map[string]interface{}{
				"Period": fmt.Sprintf("%s – %s", dateFrom, dateTo),
				"Count":  len(stats),
				"Stats":  stats,
				"Totals": map[string]int{"Views": totalViews, "Likes": totalLikes, "Reposts": totalReposts, "Comments": totalComments},
				"Avg":    map[string]int{"Views": avg(totalViews, len(stats)), "Likes": avg(totalLikes, len(stats)), "Reposts": avg(totalReposts, len(stats)), "Comments": avg(totalComments, len(stats))},
			}
		}
	}

	tmpl := template.Must(template.ParseFiles("templates/date_range.html"))
	tmpl.Execute(w, map[string]interface{}{"Report": report})
}

func clearCacheHandler(w http.ResponseWriter, r *http.Request) {
	dataCache.Clear()
	fmt.Println("🗑️ Кэш очищен")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ========== TELEGRAM ФУНКЦИОНАЛ ==========

func tgIndexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/tg_index.html"))
	tmpl.Execute(w, map[string]string{
		"ChannelName": "Kait.20 Telegram",
		"ChannelURL":  "https://t.me/kait_20_official",
	})
}

func tgPostsAnalysisHandler(w http.ResponseWriter, r *http.Request) {
	// Демо-данные для Telegram
	demoStats := []map[string]interface{}{
		{"Date": "23.02.2026 14:00", "Text": "Привет студенты! 🎓", "Views": 1250, "Reactions": 45, "Forwards": 12},
		{"Date": "22.02.2026 10:30", "Text": "День защитника Отечества! 🇷🇺", "Views": 2100, "Reactions": 89, "Forwards": 34},
		{"Date": "21.02.2026 16:20", "Text": "Расписание на следующую неделю", "Views": 980, "Reactions": 23, "Forwards": 8},
	}

	totalViews := 0
	totalReactions := 0
	totalForwards := 0

	for _, stat := range demoStats {
		totalViews += stat["Views"].(int)
		totalReactions += stat["Reactions"].(int)
		totalForwards += stat["Forwards"].(int)
	}

	tmpl := template.Must(template.ParseFiles("templates/tg_posts_analysis.html"))
	tmpl.Execute(w, map[string]interface{}{
		"Stats": demoStats,
		"N":     len(demoStats),
		"Totals": map[string]int{
			"Views":     totalViews,
			"Reactions": totalReactions,
			"Forwards":  totalForwards,
		},
	})
}
