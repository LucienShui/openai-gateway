# OpenAI Gateway

OpenAI Compatible LLM Gateway

## Deploy

Copy [compose.yml](./compose.yml), set ENV variables, then `docker compose up -d`

### Environment variable

| Name         | Default       | Comment                |
|--------------|---------------|------------------------|
| API_KEY_LIST |               | Format `key1,key2`     |
| CONFIG       |               | See [CONFIG](#CONFIG)  |
| WORKERS      | 1             | `uvicorn` worker count |
| PORT         | 8000          | Set it in `.env`       |
| LOG_DIR      | `${PWD}/logs` | Set it in `.env`       |
| HOST         | 0.0.0.0       | Never mind             |

### CONFIG

```json
 {
  "config": {
    "default": [
      {
        "model_list": [
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
        "model_list": [
          "deepseek-chat",
          "deepseek-coder"
        ],
        "api_key": "***",
        "base_url": "https://api.deepseek.com/v1"
      }
    ]
  },
  "azure": [
    {
      "model_list": [
        "gpt-4o",
        "gpt-4o-mini"
      ],
      "api_key": "***",
      "base_url": "https://***.openai.azure.com/",
      "api_version": "2024-02-15-preview",
      "is_azure": true
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

Non-default namespace:

```shell
curl -X POST localhost:8000/v1/chat/completions \
  -d '{"model":"azure/gpt-4o","messages":[{"role":"user","content":"Hello"}]}' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer key2'
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
