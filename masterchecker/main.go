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
	url := "http://localhost:8080/v1/stats"
	req, err := http.NewRequest("GET", url+"/pairs", nil)
	if err != nil {
		log.Fatalln("failure making pairs req:", err)
	}
	end := time.Now().UTC().Format(time.RFC3339)
	frame := 24 * time.Hour * 7
	start := time.Now().UTC().Add(-frame).Format(time.RFC3339)
	_, _, _ = start, end, frame
	q := req.URL.Query()
	q.Set("start_time", start)
	q.Set("end_time", end)
	q.Set("frame", frame.String())
	req.URL.RawQuery = q.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Fatalln("failure getting pairs:", resp, err)
	}
	defer resp.Body.Close()

	// v0 API:
	//var pairs struct {
	//Stats map[string]models.PairBucket `json:"stats"`
	//}
	// v1 API:
	var pairs struct {
		Stats []models.PairBucket `json:"stats"`
	}
	err = json.NewDecoder(resp.Body).Decode(&pairs)
	if err != nil {
		log.Fatalln("error decoding pairs", err)
	}

	var pairVol decimal.Decimal
	for _, s := range pairs.Stats {
		pairVol = pairVol.Add(s.VolumeUSD)
	}

	req, err = http.NewRequest("GET", url+"/tokens", nil)
	if err != nil {
		log.Fatalln("failure making tokens req:", err)
	}
	q = req.URL.Query()
	q.Set("start_time", start)
	q.Set("end_time", end)
	q.Set("frame", frame.String())
	req.URL.RawQuery = q.Encode()
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Fatalln("failure getting totals:", resp, err)
	}
	defer resp.Body.Close()

	// v0 API:
	//var tokens struct {
	//Stats map[string]models.TokenBucket `json:"stats"`
	//}
	// v1 API:
	var tokens struct {
		Stats []models.TokenBucket `json:"stats"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tokens)
	if err != nil {
		log.Fatalln("error decoding totals", err)
	}

	var tokenVol decimal.Decimal
	for _, t := range tokens.Stats {
		// hokey, but intentional to check something
		fmt.Println(t.Symbol, t.VolumeUSD)
		tokenVol = tokenVol.Add(t.VolumeUSD)
	}

	req, err = http.NewRequest("GET", url+"/", nil)
	if err != nil {
		log.Fatalln("failure making total sreq:", err)
	}
	q = req.URL.Query()
	q.Set("start_time", start)
	q.Set("end_time", end)
	q.Set("frame", frame.String())
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

	fmt.Println("pairVol:", pairVol, "tokenVol:", tokenVol, "api:", totalVol)
}
