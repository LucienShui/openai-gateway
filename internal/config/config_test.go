package config

import (
	"strings"
	"testing"
)

func TestNewConfigResolvesAliasChain(t *testing.T) {
	cfg, err := NewConfig(`{
		"default": [
			{"type":"alias","alias":{"stable":"next","next":"aliyun/flash"}},
			{"type":"openai","models":["direct"],"base_url":"https://example.com/v1"}
		],
		"aliyun": [
			{"type":"alias","alias":{"flash":"aliyun/upstream-model"}},
			{"type":"openai","models":["upstream-model"],"base_url":"https://aliyun.example/v1"}
		]
	}`, "")
	if err != nil {
		t.Fatal(err)
	}

	upstream, ok := cfg.GetRoute("aliyun/upstream-model")
	if !ok || upstream.Model != "upstream-model" {
		t.Fatalf("direct upstream route missing or incorrect: %v", upstream)
	}
	for _, id := range []string{"stable", "next", "aliyun/flash"} {
		route, ok := cfg.GetRoute(id)
		if !ok || route.Model != upstream.Model || route.Client != upstream.Client {
			t.Errorf("%s did not resolve to the same model and client as the upstream route", id)
		}
	}

	owners := map[string]string{}
	for _, model := range cfg.ModelList.Data {
		owners[model.ID] = model.OwnedBy
	}
	if len(cfg.ModelList.Data) != 5 || owners["stable"] != "default" || owners["aliyun/flash"] != "aliyun" {
		t.Errorf("unexpected model list: %+v", cfg.ModelList.Data)
	}
}

func TestNewConfigRejectsInvalidAliases(t *testing.T) {
	tests := []struct {
		name, config, wantError string
	}{
		{
			name: "missing target through alias chain",
			config: `{"default":[{"type":"alias","alias":{"stable":"other/alias"}}],
				"other":[{"type":"alias","alias":{"alias":"upstream/missing"}}]}`,
			wantError: "alias target not found: upstream/missing",
		},
		{
			name:      "self reference",
			config:    `{"default":[{"type":"alias","alias":{"stable":"stable"}}]}`,
			wantError: "alias cycle detected: stable",
		},
		{
			name: "cross namespace cycle",
			config: `{"default":[{"type":"alias","alias":{"stable":"other/alias"}}],
				"other":[{"type":"alias","alias":{"alias":"stable"}}]}`,
			wantError: "alias cycle detected:",
		},
		{
			name: "alias duplicates direct route",
			config: `{"default":[{"type":"openai","models":["stable","upstream"]},
				{"type":"alias","alias":{"stable":"upstream"}}]}`,
			wantError: "duplicate model name detected: stable",
		},
		{
			name: "direct route duplicates alias",
			config: `{"default":[{"type":"alias","alias":{"stable":"upstream"}},
				{"type":"openai","models":["stable","upstream"]}]}`,
			wantError: "duplicate model name detected: stable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewConfig(tc.config, "")
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("NewConfig() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
