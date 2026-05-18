package main

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type MLPayload struct {
	CPU         float64 `json:"cpu"`
	RAM         float64 `json:"ram"`
	Disk        float64 `json:"disk"`
	Temperature float64 `json:"temperature"`
	CoreDiff    float64 `json:"core_diff"`
	PPS         float64 `json:"pps"`
	BPS         float64 `json:"bps"`
}

type MLResponse struct {
	IsAnomaly bool    `json:"is_anomaly"`
	Score     float64 `json:"score"`
}

func checkMLAnomaly(mlURL string, payload MLPayload) (bool, float64, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return false, 0, err
	}

	resp, err := http.Post(mlURL+"/api/predict", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()

	var mlResp MLResponse
	if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
		return false, 0, err
	}

	return mlResp.IsAnomaly, mlResp.Score, nil
}
