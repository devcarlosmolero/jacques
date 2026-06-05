package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed version.json
var versionJSON []byte

func botVersion() string {
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(versionJSON, &v); err != nil {
		return "unknown"
	}
	return v.Version
}
