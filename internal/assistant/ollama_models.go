package assistant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelInfo is one entry from Ollama's /api/tags, trimmed to what the
// Settings UI shows: the model select and its badge row.
type ModelInfo struct {
	Name      string
	Size      int64
	Family    string
	ParamSize string
	Quant     string
}

// ListModels queries an Ollama server's /api/tags for the models it has
// pulled locally, so Settings can offer a picker instead of a freeform
// text field the user has to get exactly right.
func ListModels(baseURL string) ([]ModelInfo, error) {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("assistant: no Ollama base URL configured")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(base + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: %s", resp.Status)
	}
	var payload struct {
		Models []struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Details struct {
				Family            string `json:"family"`
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, len(payload.Models))
	for i, m := range payload.Models {
		out[i] = ModelInfo{
			Name:      m.Name,
			Size:      m.Size,
			Family:    m.Details.Family,
			ParamSize: m.Details.ParameterSize,
			Quant:     m.Details.QuantizationLevel,
		}
	}
	return out, nil
}

// GetCapabilities queries Ollama's /api/show for what a model actually
// declares support for — e.g. ["completion", "tools", "vision", "thinking"].
// Different models genuinely differ here (a qwen3 model may lack vision that
// a gemma model has), so the system prompt and the vision tool path are
// built from whatever this returns rather than assuming every model is the
// same. A failure here (older Ollama without this field, unreachable
// server) just means capabilities come back empty — callers treat that as
// "unknown, assume the conservative defaults" rather than a hard error.
func GetCapabilities(baseURL, model string) ([]string, error) {
	base := strings.TrimRight(baseURL, "/")
	if base == "" || model == "" {
		return nil, fmt.Errorf("assistant: missing Ollama base URL or model")
	}
	// Ollama has used both "model" and "name" as the request key across
	// versions; sending both is harmless and avoids version-sniffing.
	body, err := json.Marshal(map[string]string{"model": model, "name": model})
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(base+"/api/show", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var payload struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Capabilities, nil
}
