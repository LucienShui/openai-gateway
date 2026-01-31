# OpenAI Gateway

OpenAI Compatible LLM Gateway

## Deploy

> For kubernetes example, refer to [openai-gateway-k8s.yaml](./openai-gateway-k8s.yaml).

Copy [compose.yml](./compose.yml), set ENV variables, then `docker compose up -d`

### Environment variable

| Name                       | Default   | Comment                                      |
|----------------------------|-----------|----------------------------------------------|
| API_KEYS                   |           | Format `key1,key2`                           |
| CONFIG                     |           | See [CONFIG](#CONFIG)                        |
| LOG_LEVEL                  | `INFO`    | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR`  |
| PORT                       | `8000`    | Server port                                  |
| HOST                       | `0.0.0.0` | Server host                                  |
| OTEL_EXPORTER_OTLP_ENDPOINT|           | OpenTelemetry endpoint for tracing           |
| VERSION                    |           | Service version for telemetry                |
| ENV                        |           | Deployment environment name for telemetry    |

### CONFIG

You can access a specific model with `namespace/model`.

+ `namespace` is a user defined name to identify the same model but different provider like Azure and OpenAI.
+ You can use `default/model` or `model` in brief when `namespace` is `default`.
+ `model` will be treated as a passed-by parameter to the provider.

```json
{
  "default": [
    {
      "type": "openai",
      "models": [
        "gpt-4",
        "gpt-4-32k",
        "gpt-4-turbo",
        "gpt-4o",
        "gpt-4o-mini",
        "gpt-3.5-turbo",
        "gpt-3.5-turbo-16k"
      ],
      "api_key": "***",
      "base_url": "https://api.openai.com/v1"
    },
    {
      "type": "openai",
      "models": [
        "deepseek-chat",
        "deepseek-coder"
      ],
      "api_key": "***",
      "base_url": "https://api.deepseek.com/v1"
    }
  ],
  "azure": [
    {
      "type": "azure",
      "models": [
        "gpt-4o",
        "gpt-4o-mini"
      ],
      "api_key": "***",
      "azure_endpoint": "https://***.openai.azure.com/",
      "api_version": "2024-02-15-preview"
    }
  ],
  "foo": [
    {
      "type": "alias",
      "alias": {
        "azure-4o": "azure/gpt-4o",
        "deepseek": "deepseek-chat"
      }
    }
  ]
}
```

## Usage

### Chat generation

Default namespace:

```shell
curl localhost:8000/v1/chat/completions -X POST \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer key1'
```

User defined namespace:

```shell
curl -X POST localhost:8000/v1/chat/completions \
  -d '{"model":"azure/gpt-4o","messages":[{"role":"user","content":"Hello"}]}' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer key2'
```

Alias model:

```shell
curl -X POST localhost:8000/v1/chat/completions \
  -d '{"model":"foo/deepseek","messages":[{"role":"user","content":"Hello"}]}' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer key2'
```

### Generation

```shell
curl localhost:8000/v1/completions -X POST \
  -d '{"model":"gpt-4o","prompt":"Hi"}' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer key1'
```

### Model list

```shell
curl localhost:8000/v1/models \
  -H 'Authorization: Bearer key2'
```

### Healthcheck

```shell
curl localhost:8000/health
```

Response:

```json
{
  "status": "ok",
  "uptime": "1h30m45s"
}
```
