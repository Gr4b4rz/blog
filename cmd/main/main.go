package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

const REPO_REDIS_KEY string = "REPO:"

type GithubInfo struct {
	Url         string
	Nickname    string
	Name        string
	City        string
	Description string
	Company     string
}

type Repo struct {
	Name        string
	Url         string
	Description string
	UpdateDate  time.Time
}

type ArticleBrief struct {
	Href     string
	Date     string
	Title    string
	Subtitle string
	Brief    string
	Img      string
	Tags     []string
	Keywords []string
}

// TemplateRenderer is a custom html/template renderer for Echo framework
type TemplateRenderer struct {
	templates *template.Template
}

// Render renders a template document
func (t *TemplateRenderer) Render(w io.Writer, name string,
	data interface{}, c echo.Context) error {
	// Add global methods if data is a map
	if viewContext, isMap := data.(map[string]interface{}); isMap {
		viewContext["reverse"] = c.Echo().Reverse
	}

	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	ctx := context.Background()
	// TODO: docker-compose instance
	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	e := echo.New()
	e.Static("dist", "dist")
	e.Static("img", "img")
	renderer := &TemplateRenderer{
		templates: template.Must(template.ParseGlob("public/*.html")),
	}
	e.Renderer = renderer

	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})

	e.GET("/articles", func(c echo.Context) error {
		if c.Request().Header.Get("Hx-Request") == "" {
			return c.Render(http.StatusOK, "index.html", nil)
		}
		c.Response().Header().Set("Cache-Control", "no-store, max-age=0")
		articles := get_articles()
		return c.Render(http.StatusOK, "articles", articles)
	})

	e.GET("/repositories", func(c echo.Context) error {
		if c.Request().Header.Get("Hx-Request") == "" {
			return c.Render(http.StatusOK, "index.html", nil)
		}
		repos := get_repositories(ctx, rdb)
		c.Response().Header().Set("Cache-Control", "no-store, max-age=0")
		return c.Render(http.StatusOK, "repos", repos)
	})

	e.GET("/about-me", func(c echo.Context) error {
		if c.Request().Header.Get("Hx-Request") == "" {
			return c.Render(http.StatusOK, "index.html", nil)
		}
		c.Response().Header().Set("Cache-Control", "no-store, max-age=0")
		gh_info := get_github_info()
		return c.Render(http.StatusOK, "about_me", gh_info)
	})

	e.GET("/about-page", func(c echo.Context) error {
		if c.Request().Header.Get("Hx-Request") == "" {
			return c.Render(http.StatusOK, "index.html", nil)
		}
		c.Response().Header().Set("Cache-Control", "no-store, max-age=0")
		return c.Render(http.StatusOK, "about_this_page.html", nil)
	})

	e.Logger.Fatal(e.Start(":8000"))
}

func get_articles() []ArticleBrief {
	var briefs []ArticleBrief
	briefs = append(briefs, ArticleBrief{Img: "img/articles/netflow2020.jpg",
		Title:    "NetfFlow v9 & DDoS",
		Subtitle: "Jak wykorzystywać NetFlow w detekcji ataków DDoS",
		Brief:    "Skuteczna obrona przed cyberatakami jest obecnie jednym z większych wyzwań współczesnego świata IT. Sieci prywatne, jak i korporacyjne są często celami przeróżnych ataków hakerskich, w tym ataków DDoS, z powodu których straty sięgają nierzadko setek tysięcy a nawet milionów dolarów. Dlatego w EXATEL rozwijamy rozwiązania antyDDoS odpowiadające za ich detekcję i mitygację.",
		Date:     "26.10.2020",
		Href:     "https://geek.justjoin.it/jak-wykorzystywac-netflow-w-detekcji-atakow-ddos/",
		Keywords: []string{"NetFlow", "DDoS", "antyDDoS", "Exatel"},
		Tags:     []string{"Polish", "Cybersecurity", "Networking"}})
	return briefs
}

func get_github_info() GithubInfo {
	res, err := http.Get("https://api.github.com/users/gr4b4rz")
	if err != nil {
		fmt.Printf("error making http get request: %s\n", err)
		// handle error
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("client: could not read response body: %s\n", err)
		// handle error
	}
	var body_json map[string]string
	json.Unmarshal(body, &body_json)
	return GithubInfo{
		Url:         body_json["html_url"],
		Nickname:    body_json["login"],
		Name:        body_json["name"],
		City:        body_json["location"],
		Description: body_json["bio"],
		Company:     body_json["company"],
	}
}

func get_repositories(ctx context.Context, rdb *redis.Client) []Repo {
	cached_repos := repos_from_cache(ctx, rdb)
	if cached_repos != nil {
		return cached_repos
	}
	res, err := http.Get("https://api.github.com/users/gr4b4rz/repos")
	if err != nil {
		fmt.Printf("error making http get request: %s\n", err)
		// handle error
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("client: could not read response body: %s\n", err)
		// handle error
	}
	var body_json []map[string]string
	json.Unmarshal(body, &body_json)

	var repos []Repo

	for _, elem := range body_json {
		res, err := http.Get(elem["url"])
		if err != nil {
			fmt.Printf("error making http get request: %s\n", err)
			// handle error
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			fmt.Printf("client: could not read response body: %s\n", err)
			// handle error
		}
		var repo_json map[string]string
		json.Unmarshal(body, &repo_json)
		time_date, err := time.Parse(time.RFC3339, repo_json["pushed_at"])
		if err != nil {
			fmt.Printf("Could not parse last commit date: %s\n", err)
			// handle error
		}
		repos = append(repos, Repo{Name: elem["name"], Url: elem["html_url"],
			Description: repo_json["description"], UpdateDate: time_date})

	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].UpdateDate.After(repos[j].UpdateDate)
	})

	cache_repos(ctx, rdb, repos)
	return repos
}

// Transforms given list of Repos to the list of maps and writes them in the Redis DB.
// Keys should match REPO_REDIS_KEY* pattern, e.g. REPO:1, REPO:2 ...
// Keys have TTL set up to 24 hours.
func cache_repos(ctx context.Context, rdb *redis.Client, repos []Repo) {
	pipeline := rdb.Pipeline()
	for idx, repo := range repos {
		key := REPO_REDIS_KEY + strconv.Itoa(idx)
		var kv_map map[string]interface{}
		inrec, _ := json.Marshal(repo)
		json.Unmarshal(inrec, &kv_map)
		pipeline.Process(ctx, rdb.HSet(ctx, key, kv_map))
		pipeline.Process(ctx, rdb.Expire(ctx, key, time.Duration(24*time.Hour)))
	}
	_, err := pipeline.Exec(ctx)
	if err != nil {
		fmt.Printf("Error when writing repositories data in the redis cache")
	}
}

// Retrieves list of Repos from the Redis DB.
// Returns nil if an error occurs or if there are no repos in the DB.
func repos_from_cache(ctx context.Context, rdb *redis.Client) []Repo {
	repo_keys, err := rdb.Keys(ctx, REPO_REDIS_KEY+"*").Result()
	if err != nil || len(repo_keys) == 0 {
		return nil
	}
	var repos []Repo
	for _, repo_key := range repo_keys {
		val, err := rdb.HGetAll(ctx, repo_key).Result()
		if err != nil {
			return nil
		}
		time_date, err := time.Parse(time.RFC3339, val["UpdateDate"])
		repos = append(repos, Repo{Name: val["Name"], Url: val["Url"],
			Description: val["Description"], UpdateDate: time_date})
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].UpdateDate.After(repos[j].UpdateDate)
	})

	return repos
}
