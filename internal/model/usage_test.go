package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"token-ray/internal/model"
)

func TestSyncRecordJSONPreservesDeviceAndRepairMetadata(t *testing.T) {
	updatedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	record := model.SyncRecord{
		TokenUsage: model.TokenUsage{
			Source:       "Codex",
			Model:        "gpt-5.5",
			InputTokens:  100,
			OutputTokens: 20,
			UUID:         "codex:1",
		},
		CostUSD:    1.23,
		FilePath:   "/logs.sqlite",
		DeviceID:   "remote-mac",
		Superseded: true,
		UpdatedAt:  updatedAt,
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.SyncRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.DeviceID != "remote-mac" || decoded.TokenUsage.DeviceID != "" {
		t.Fatalf("device_id did not round trip as sync metadata: json=%s decoded=%+v", data, decoded)
	}
	if !decoded.Superseded || !decoded.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("repair sync metadata did not round trip: json=%s decoded=%+v", data, decoded)
	}
}
