# OpenAI Gateway

OpenAI Compatible LLM Gateway

## Deploy

Copy [compose.yml](./compose.yml) then `docker compose up -d`

## Env Variable

### CONFIG

```json
{
  "config": {
    "default": [
      {
        "model_list": ["gpt-4o", "gpt-4o-mini"],
        "api_key": "***",
        "base_url": "https://api.openai.com/v1"
      }
    ],
    "azure": [
      {
        "model_list": ["gpt-4o", "gpt-4o-mini"],
        "api_key": "***",
        "base_url": "",
        "api_version": "",
        "is_azure": true
      }
    ]
  }
}
```

Access the model:

```shell
curl localhost:8000/v1/chat/completions -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}'

curl localhost:8000/v1/chat/completions -d '{"model":"azure/gpt-4o","messages":[{"role":"user","content":"Hello"}]}'
```

