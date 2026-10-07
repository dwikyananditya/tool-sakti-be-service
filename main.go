package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"

	"tool-sakti-be-service/internal/auth"
	"tool-sakti-be-service/internal/dsm"
	"tool-sakti-be-service/internal/github"
	"tool-sakti-be-service/internal/kpi"
)

func main() {
	_ = godotenv.Load()

	r := gin.Default()

	store := cookie.NewStore(sessionKey())
	store.Options(sessions.Options{Path: "/", HttpOnly: true, MaxAge: 7 * 24 * 3600})
	r.Use(sessions.Sessions("tool-sakti-be-service", store))
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Header("Cache-Control", "no-store")
		}
	})
	r.LoadHTMLGlob("web/templates/*.html")
	r.Static("/static", "web/static")

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{})
	})
	r.POST("/login", handleLogin)
	r.GET("/logout", func(c *gin.Context) {
		auth.Clear(c)
		c.Redirect(http.StatusFound, "/login")
	})

	authed := r.Group("/", auth.RequireToken)
	authed.GET("/", index)
	authed.POST("/scan", handleScan)
	authed.GET("/r/:id", func(c *gin.Context) {
		c.HTML(http.StatusOK, "result.html", gin.H{"login": auth.CurrentLogin(c)})
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("tool-sakti-be-service listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func handleLogin(c *gin.Context) {
	token := c.PostForm("token")
	if token == "" {
		c.HTML(http.StatusOK, "login.html", gin.H{"error": "Token wajib diisi."})
		return
	}
	login, err := auth.Validate(c.Request.Context(), token)
	if err != nil {
		c.HTML(http.StatusOK, "login.html", gin.H{"error": err.Error()})
		return
	}
	if err := auth.SetSession(c, token, login); err != nil {
		c.HTML(http.StatusOK, "login.html", gin.H{"error": "Gagal menyimpan sesi."})
		return
	}
	c.Redirect(http.StatusFound, "/")
}

func index(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"login":     auth.CurrentLogin(c),
		"assignees": dsm.AllAssignees(),
	})
}

type scanResponse struct {
	FileName   string           `json:"fileName"`
	Total      int              `json:"total"`
	Review     int              `json:"review"`
	ScanErrors int              `json:"scanErrors"`
	Rows       []kpi.PreviewRow `json:"rows"`
	Xlsx       string           `json:"xlsx"` // base64-encoded workbook
}

func handleScan(c *gin.Context) {
	dsmHeader, err := c.FormFile("dsm")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File DSM wajib diunggah."})
		return
	}
	dsmText, err := readUpload(dsmHeader)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Gagal membaca DSM."})
		return
	}

	depth, _ := strconv.Atoi(c.PostForm("depth"))
	if depth < 1 {
		depth = 1
	}

	entries := dsm.ParseDsm(string(dsmText), 2026, c.PostFormArray("assignee"))
	if len(entries) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Tidak menemukan tiket DSM lengkap dengan tanggal, sesi, dan assignee."})
		return
	}

	client := github.New(auth.Token(c))
	scans, scanErrs := client.Collect(c.Request.Context(), scanInputs(entries), depth)
	for _, e := range scanErrs {
		log.Printf("scan error %s: %s", e.URL, e.Err)
	}

	rows, _ := kpi.PreviewRows(entries, scans)

	file, stats, outName, buildErr := buildWorkbook(c, entries, scans)
	if buildErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membangun workbook."})
		return
	}
	b, err := file.WriteToBuffer()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menulis workbook."})
		return
	}

	log.Printf("tool-sakti-be-service: %d row, %d review, %d scan error", len(rows), stats.Review, len(scanErrs))
	c.JSON(http.StatusOK, scanResponse{
		FileName:   outName,
		Total:      len(rows),
		Review:     stats.Review,
		ScanErrors: len(scanErrs),
		Rows:       rows,
		Xlsx:       base64.StdEncoding.EncodeToString(b.Bytes()),
	})
}

func buildWorkbook(c *gin.Context, entries []dsm.Entry, scans map[string]*github.Scan) (*excelize.File, kpi.Stats, string, error) {
	if kpiHeader, err := c.FormFile("kpi"); err == nil {
		kpiBytes, rerr := readUpload(kpiHeader)
		if rerr != nil {
			return nil, kpi.Stats{}, "", rerr
		}
		f, stats, berr := kpi.FillWorkbook(kpiBytes, entries, scans)
		return f, stats, replaceExt(kpiHeader.Filename), berr
	}
	f, stats, berr := kpi.BuildFromDSM(entries, scans)
	return f, stats, kpi.GeneratedFileName(firstDate(entries)), berr
}

func scanInputs(entries []dsm.Entry) []github.ScanInput {
	type agg struct {
		dates   map[string]bool
		targets map[string][]string
	}
	grouped := map[string]*agg{}
	var order []string
	for _, e := range dsm.CollapseDailyEntries(entries) {
		g := grouped[e.TicketURL]
		if g == nil {
			g = &agg{dates: map[string]bool{}, targets: map[string][]string{}}
			grouped[e.TicketURL] = g
			order = append(order, e.TicketURL)
		}
		g.dates[e.Date] = true
		if e.Status != "" {
			g.targets[e.Date] = appendUnique(g.targets[e.Date], e.Status)
		}
	}
	items := make([]github.ScanInput, 0, len(order))
	for _, url := range order {
		g := grouped[url]
		dates := make([]string, 0, len(g.dates))
		for d := range g.dates {
			dates = append(dates, d)
		}
		items = append(items, github.ScanInput{URL: url, Dates: dates, Targets: g.targets})
	}
	return items
}

func readUpload(h *multipart.FileHeader) ([]byte, error) {
	f, err := h.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func appendUnique(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

var xlsxExtRe = regexp.MustCompile(`(?i)\.xlsx?$`)

func replaceExt(name string) string {
	if xlsxExtRe.MatchString(name) {
		return xlsxExtRe.ReplaceAllString(name, "-JEJAK.xlsx")
	}
	return name + "-JEJAK.xlsx"
}

func firstDate(entries []dsm.Entry) string {
	collapsed := dsm.CollapseDailyEntries(entries)
	if len(collapsed) == 0 {
		return ""
	}
	return collapsed[0].Date
}

func sessionKey() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}
