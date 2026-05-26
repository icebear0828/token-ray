package agyscanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	servicePath     = "/exa.language_server_pb.LanguageServerService"
	trajectoryPath  = servicePath + "/GetCascadeTrajectory"
	trajectoriesPath = servicePath + "/GetAllCascadeTrajectories"
	httpTimeout     = 3 * time.Second
)

var httpClient = &http.Client{Timeout: httpTimeout}

type TrajectorySummary struct {
	StepCount int    `json:"stepCount"`
	Status    string `json:"status"`
}

type TrajectoriesResponse struct {
	Summaries map[string]TrajectorySummary `json:"trajectorySummaries"`
}

type TrajectoryResponse struct {
	Trajectory Trajectory `json:"trajectory"`
	Code       string     `json:"code,omitempty"`
	Message    string     `json:"message,omitempty"`
}

type Trajectory struct {
	CascadeID         string              `json:"cascadeId"`
	GeneratorMetadata []GeneratorMetadata  `json:"generatorMetadata"`
}

type GeneratorMetadata struct {
	ChatModel ChatModel `json:"chatModel"`
}

type ChatModel struct {
	Usage             ChatModelUsage     `json:"usage"`
	ResponseModel     string             `json:"responseModel"`
	ModelDisplayName  string             `json:"modelDisplayName"`
	ChatStartMetadata ChatStartMetadata  `json:"chatStartMetadata"`
}

type ChatModelUsage struct {
	InputTokens     string `json:"inputTokens"`
	OutputTokens    string `json:"outputTokens"`
	CacheReadTokens string `json:"cacheReadTokens"`
}

type ChatStartMetadata struct {
	CreatedAt string `json:"createdAt"`
}

func (u ChatModelUsage) InputInt() int {
	n, _ := strconv.Atoi(u.InputTokens)
	return n
}

func (u ChatModelUsage) OutputInt() int {
	n, _ := strconv.Atoi(u.OutputTokens)
	return n
}

func (u ChatModelUsage) CacheReadInt() int {
	n, _ := strconv.Atoi(u.CacheReadTokens)
	return n
}

func getAllTrajectories(port int) (map[string]TrajectorySummary, error) {
	body, err := postJSON(port, trajectoriesPath, []byte("{}"))
	if err != nil {
		return nil, err
	}
	var resp TrajectoriesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode trajectories: %w", err)
	}
	return resp.Summaries, nil
}

func getTrajectory(port int, cascadeID string) (*TrajectoryResponse, error) {
	payload, _ := json.Marshal(map[string]string{"cascade_id": cascadeID})
	body, err := postJSON(port, trajectoryPath, payload)
	if err != nil {
		return nil, err
	}
	var resp TrajectoryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode trajectory: %w", err)
	}
	if resp.Code != "" {
		return nil, fmt.Errorf("trajectory %s: %s", resp.Code, resp.Message)
	}
	return &resp, nil
}

func postJSON(port int, path string, payload []byte) ([]byte, error) {
	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	return body, nil
}
