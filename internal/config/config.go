package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type ClientConfig struct {
	Type          string            `json:"type"`
	Models        []string          `json:"models,omitempty"`
	APIKey        string            `json:"api_key,omitempty"`
	BaseURL       string            `json:"base_url,omitempty"`
	AzureEndpoint string            `json:"azure_endpoint,omitempty"`
	APIVersion    string            `json:"api_version,omitempty"`
	Alias         map[string]string `json:"alias,omitempty"`
}

type UpstreamClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	IsAzure    bool
	APIVersion string
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Config struct {
	Map       map[string]*RouteEntry
	TokenList []string
	ModelList ModelList
}

type RouteEntry struct {
	Model  string
	Client *UpstreamClient
}

func NewConfig(configJSON, apiKeys string) (*Config, error) {
	var rawConfig map[string][]ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg := &Config{
		Map: make(map[string]*RouteEntry),
		ModelList: ModelList{
			Object: "list",
			Data:   []Model{},
		},
	}

	if apiKeys != "" {
		cfg.TokenList = strings.Split(apiKeys, ",")
	}

	configFlatten := make(map[string]*RouteEntry)

	for namespace, clientConfigs := range rawConfig {
		for _, cc := range clientConfigs {
			client := createClient(&cc)

			if cc.Type == "alias" {
				for aliasModel, targetKey := range cc.Alias {
					key := getKey(namespace, aliasModel)
					target, ok := configFlatten[targetKey]
					if !ok {
						return nil, fmt.Errorf("alias target not found: %s", targetKey)
					}
					entry := &RouteEntry{
						Model:  target.Model,
						Client: target.Client,
					}
					if _, exists := cfg.Map[key]; exists {
						return nil, fmt.Errorf("duplicate model name detected: %s", key)
					}
					configFlatten[key] = entry
					cfg.Map[key] = entry
					cfg.ModelList.Data = append(cfg.ModelList.Data, Model{
						ID:      key,
						Object:  "model",
						Created: time.Now().Unix(),
						OwnedBy: namespace,
					})
				}
			} else {
				for _, model := range cc.Models {
					key := getKey(namespace, model)
					entry := &RouteEntry{
						Model:  model,
						Client: client,
					}
					if _, exists := cfg.Map[key]; exists {
						return nil, fmt.Errorf("duplicate model name detected: %s", key)
					}
					configFlatten[key] = entry
					cfg.Map[key] = entry
					cfg.ModelList.Data = append(cfg.ModelList.Data, Model{
						ID:      key,
						Object:  "model",
						Created: time.Now().Unix(),
						OwnedBy: namespace,
					})
				}
			}
		}
	}

	return cfg, nil
}

func createClient(cc *ClientConfig) *UpstreamClient {
	client := &UpstreamClient{
		APIKey: cc.APIKey,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	switch cc.Type {
	case "openai":
		client.BaseURL = strings.TrimSuffix(cc.BaseURL, "/")
	case "azure":
		client.IsAzure = true
		client.BaseURL = strings.TrimSuffix(cc.AzureEndpoint, "/")
		client.APIVersion = cc.APIVersion
	}

	return client
}

func getKey(namespace, model string) string {
	if namespace == "default" {
		return model
	}
	return namespace + "/" + model
}

func (c *Config) GetRoute(model string) (*RouteEntry, bool) {
	entry, ok := c.Map[model]
	return entry, ok
}

func (c *Config) ValidateToken(token string) bool {
	if len(c.TokenList) == 0 {
		return true
	}
	for _, t := range c.TokenList {
		if t == token {
			return true
		}
	}
	return false
}

func (u *UpstreamClient) BuildURL(path string) string {
	if u.IsAzure {
		return u.buildAzureURL(path)
	}
	return u.BaseURL + strings.TrimPrefix(path, "/v1")
}

func (u *UpstreamClient) buildAzureURL(path string) string {
	baseURL := u.BaseURL + "/openai" + path
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	q := parsed.Query()
	q.Set("api-version", u.APIVersion)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func GetEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
