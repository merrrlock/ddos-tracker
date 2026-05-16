package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type PromResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func QueryPrometheus(promURL string, query string) (float64, error) {
	reqURL := fmt.Sprintf("%s/api/v1/query?query=%s", promURL, url.QueryEscape(query))

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	log.Printf("\n[RAW PROMETHEUS BODY] Запрос: %s\nОтвет: %s\n", query, string(bodyBytes))

	var promResp PromResponse
	if err := json.Unmarshal(bodyBytes, &promResp); err != nil {
		return 0, err
	}

	if len(promResp.Data.Result) == 0 || len(promResp.Data.Result[0].Value) < 2 {
		return 0, fmt.Errorf("нет данных для запроса")
	}

	valStr, ok := promResp.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("неожиданный формат значения")
	}

	return strconv.ParseFloat(valStr, 64)
}
