package hnpopular

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/chyiyaqing/newsbot/internal/store"
)

const cdnBase = "https://hn-popularity.cdn.refactoringenglish.com"

// FetchTopBlogs downloads CSV data from the HN Popularity CDN and returns the top blogs by score.
func FetchTopBlogs(limit int) ([]store.Blog, error) {
	// 1. Download and aggregate scores from hn-data.csv
	log.Printf("Fetching HN data from %s/hn-data.csv", cdnBase)
	scores, err := fetchScores()
	if err != nil {
		return nil, fmt.Errorf("fetch scores: %w", err)
	}

	// 2. Download author metadata from domains-meta.csv
	log.Printf("Fetching domain metadata from %s/domains-meta.csv", cdnBase)
	meta, err := fetchMeta()
	if err != nil {
		return nil, fmt.Errorf("fetch meta: %w", err)
	}

	// 3. Build blog list sorted by total score descending
	type entry struct {
		domain string
		score  int
	}
	entries := make([]entry, 0, len(scores))
	for domain, score := range scores {
		entries = append(entries, entry{domain, score})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	if limit > len(entries) {
		limit = len(entries)
	}

	blogs := make([]store.Blog, 0, limit)
	for i := 0; i < limit; i++ {
		e := entries[i]
		blogs = append(blogs, store.Blog{
			Domain: e.domain,
			Score:  e.score,
			Author: meta[e.domain],
			Rank:   i + 1,
		})
	}

	log.Printf("Found %d blogs", len(blogs))
	return blogs, nil
}

// fetchScores downloads hn-data.csv and returns total score per domain.
// CSV columns: domain, score, date
func fetchScores() (map[string]int, error) {
	records, err := fetchCSV(cdnBase + "/hn-data.csv")
	if err != nil {
		return nil, err
	}

	scores := make(map[string]int)
	for i, row := range records {
		if i == 0 {
			continue // skip header
		}
		if len(row) < 2 {
			continue
		}
		domain := strings.TrimSpace(row[0])
		score, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil {
			continue
		}
		scores[domain] += score
	}
	return scores, nil
}

// fetchMeta downloads domains-meta.csv and returns author name per domain.
// CSV columns: domain, author, bio, topics
func fetchMeta() (map[string]string, error) {
	records, err := fetchCSV(cdnBase + "/domains-meta.csv")
	if err != nil {
		return nil, err
	}

	meta := make(map[string]string)
	for i, row := range records {
		if i == 0 {
			continue // skip header
		}
		if len(row) < 2 {
			continue
		}
		domain := strings.TrimSpace(row[0])
		author := strings.TrimSpace(row[1])
		meta[domain] = author
	}
	return meta, nil
}

// fetchCSV downloads a CSV file and returns all records.
func fetchCSV(url string) ([][]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	r := csv.NewReader(resp.Body)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // variable number of fields
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV from %s: %w", url, err)
	}
	return records, nil
}
