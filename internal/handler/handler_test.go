package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LucienShui/openai-gateway/internal/config"
	"go.uber.org/zap"
)

func TestProxySwitchesAliasUpstream(t *testing.T) {
	var aliyunCalls, tokenPlanCalls atomic.Int32
	upstream := func(calls *atomic.Int32, response string) *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var request struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "flash" {
				t.Errorf("upstream got model %q, decode error %v", request.Model, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fmt.Fprintf(w, `{"upstream":%q}`, response)
		}))
		t.Cleanup(server.Close)
		return server
	}
	aliyun := upstream(&aliyunCalls, "aliyun")
	tokenPlan := upstream(&tokenPlanCalls, "aliyun_tokenplan")

	for _, target := range []string{"aliyun", "aliyun_tokenplan"} {
		t.Run(target, func(t *testing.T) {
			aliyunCalls.Store(0)
			tokenPlanCalls.Store(0)
			cfg, err := config.NewConfig(fmt.Sprintf(`{
				"default":[{"type":"alias","alias":{"stable":%q}}],
				"aliyun":[{"type":"openai","models":["flash"],"base_url":%q}],
				"aliyun_tokenplan":[{"type":"openai","models":["flash"],"base_url":%q}]
			}`, target+"/flash", aliyun.URL, tokenPlan.URL), "")
			if err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"stable"}`))
			res := httptest.NewRecorder()
			New(cfg, zap.NewNop()).Proxy(res, req)
			if want := fmt.Sprintf(`{"upstream":%q}`, target); res.Code != http.StatusOK || res.Body.String() != want {
				t.Fatalf("response = %d %s, want 200 %s", res.Code, res.Body, want)
			}
			if aliyunCalls.Load()+tokenPlanCalls.Load() != 1 {
				t.Fatal("expected exactly one upstream request")
			}
		})
	}
}
