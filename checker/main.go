package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/goswap/stats-api/models"
	"github.com/shopspring/decimal"
)

func main() {
	req, err := http.NewRequest("GET", "http://localhost:8080/v1/stats/pairs", nil)
	end := time.Now().UTC().Format(time.RFC3339)
	interval := 24 * time.Hour
	start := time.Now().UTC().Add(-interval).Format(time.RFC3339)
	_, _, _ = start, end, interval
	q := req.URL.Query()
	q.Set("time_start", start)
	q.Set("time_end", end)
	q.Set("time_frame", interval.String())
	req.URL.RawQuery = q.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Fatalln("failure getting pairs:", resp, err)
	}
	defer resp.Body.Close()

	// v0 API:
	//var stats struct {
	//Stats map[string]models.PairBucket `json:"stats"`
	//}
	// v1 API:
	var stats struct {
		Stats []models.PairBucket `json:"stats"`
	}
	err = json.NewDecoder(resp.Body).Decode(&stats)
	if err != nil {
		log.Fatalln("error decoding pairs", err)
	}

	var vol decimal.Decimal
	for _, s := range stats.Stats {
		vol = vol.Add(s.VolumeUSD)
	}

	req, err = http.NewRequest("GET", "http://localhost:8080/v1/stats", nil)
	q = req.URL.Query()
	q.Set("time_start", start)
	q.Set("time_end", end)
	q.Set("time_frame", interval.String())
	req.URL.RawQuery = q.Encode()
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Fatalln("failure getting totals:", resp, err)
	}
	defer resp.Body.Close()

	// v0 API:
	//var totals struct {
	//Stats []models.TotalBucket `json:"overTime"`
	//}
	// v1 API:
	var totals struct {
		Stats []models.TotalBucket `json:"stats"`
	}
	err = json.NewDecoder(resp.Body).Decode(&totals)
	if err != nil {
		log.Fatalln("error decoding totals", err)
	}

	var totalVol decimal.Decimal
	for _, t := range totals.Stats {
		// hokey, but intentional to check something
		totalVol = t.VolumeUSD
	}

	fmt.Println("computed:", vol, "api:", totalVol)
}
