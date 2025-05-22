import httpx
import pytest
from dotenv import load_dotenv
from fastapi.testclient import TestClient
from openai import OpenAI
from openai.types.chat import ChatCompletion

from main import app

load_dotenv()
api_key: str = "key"


@pytest.fixture()
def client():
    with TestClient(app) as client:
        yield client


def test_openai_client(client: httpx.Client):
    o = OpenAI(base_url="/v1", api_key=api_key, http_client=client)
    m = "model"
    openai_response: ChatCompletion = o.chat.completions.create(
        model=m, messages=[{"role": "user", "content": "Hello!"}], extra_body={"enable_thinking": False})
    response: str = openai_response.choices[0].message.content
    print(response)


@pytest.mark.parametrize("extra_body", [
    {"chat_template_kwargs": {"enable_thinking": False}},
    {"enable_thinking": False}
])
def test_httpx_client(client: httpx.Client, extra_body: dict):
    httpx_response: httpx.Response = client.post("/v1/chat/completions", json={
        "model": "model",
        "messages": [{"role": "user", "content": "Hello"}],
        **extra_body
    }, headers={"Authorization": f"Bearer {api_key}"})
    json_response: dict = httpx_response.json()
    response: str = json_response["choices"][0]["message"]["content"]
    print(response)


def test_openai_client_stream(client: httpx.Client):
    o = OpenAI(base_url="/v1", api_key=api_key, http_client=client)
    m = "model"
    for chunk in o.chat.completions.create(
            model=m, messages=[{"role": "user", "content": "Hello!"}], extra_body={"enable_thinking": True}, stream=True
    ):
        delta = chunk.choices[0].delta
        for key in ['content', 'reasoning_content']:
            if hasattr(delta, key) and (v := getattr(delta, key)):
                print(v, end="", flush=True)


def test_error(client: httpx.Client):
    httpx_response: httpx.Response = client.post("/v1/chat/completions", json={
        "model": "model",
        "messages": [{"role": "user", "content": "Hello"}],
        "enable_thinking": False,
        "chat_template_kwargs": {"enable_thinking": True}
    }, headers={"Authorization": f"Bearer {api_key}"})
    assert httpx_response.status_code == 400
