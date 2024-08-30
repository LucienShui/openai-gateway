import os
import time
from contextlib import asynccontextmanager
from typing import Dict, Annotated, AsyncIterable, List, Tuple, Optional, Callable

import uvicorn
from fastapi import FastAPI, HTTPException, Request, status, Header
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, Response
from openai import AsyncOpenAI
from openai.lib.azure import AsyncAzureOpenAI
from openai.types.chat.chat_completion import ChatCompletion
from openai.types.chat.chat_completion_chunk import ChatCompletionChunk
from openai.types.completion import Completion
from openai.types.model import Model
from sse_starlette.sse import EventSourceResponse

from openai_gateway.config import Config, API
from openai_gateway.entity import ModelList
from openai_gateway.logger import get_logger

ns_dict: Dict[str, Dict[str, AsyncOpenAI]] = {}
token_list: List[str] = []
model_list: ModelList = ModelList()  # Response of /v1/models
logger = get_logger(__name__)


@asynccontextmanager
async def lifespan(_: FastAPI):
    config: Dict[str, List[API]] = Config.model_validate_json(os.environ["CONFIG"]).config
    for namespace, api_list in config.items():
        for api in api_list:
            for model in api.models:
                if api.is_azure:
                    ns_dict.setdefault(namespace, {})[model] = AsyncAzureOpenAI(
                        azure_endpoint=api.base_url, api_version=api.api_version, api_key=api.api_key)
                else:
                    ns_dict.setdefault(namespace, {})[model] = AsyncOpenAI(api_key=api.api_key, base_url=api.base_url)
                model_list.data.append(Model(
                    id="/".join(([namespace] if namespace != "default" else []) + [model]),
                    created=int(time.time()),
                    owned_by=namespace,
                    object="model"
                ))

    token_list.extend(os.environ["API_KEYS"].split(","))
    yield


app = FastAPI(lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"]
)


@app.exception_handler(Exception)
async def exception_handler(_: Request, e: Exception) -> Response:
    logger.exception({
        "exception_class": e.__class__.__name__,
        "exception_message": str(e),
        "status": 500
    })
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={
            "exception_class": e.__class__.__name__,
            "exception_message": str(e),
            "status": 500
        }
    )


async def get_namespace_and_model(key: str) -> Tuple[str, str]:
    if "/" in key:
        namespace, model = key.split("/")
        return namespace, model
    return "default", key


async def stream(func: Callable, request: dict, model: str, api: str) -> AsyncIterable[str]:
    response = ""
    chunk: Completion | ChatCompletionChunk = ...
    start_time = time.time()
    async for chunk in await func(**(request | {"model": model})):
        yield chunk.model_dump_json()
        try:
            if isinstance(chunk, Completion):
                response += chunk.choices[0].text
            elif isinstance(chunk, ChatCompletionChunk):
                response += chunk.choices[0].delta.content or ""
            else:
                raise Exception("Unknown chunk type")
        except Exception as e:
            logger.exception({
                **request,
                "exception_class": e.__class__.__name__,
                "exception_message": str(e)
            })
    logger.info({
        "api": api,
        "request": request,
        "response": response,
        "chunk": None if chunk is ... else chunk.model_dump(),
        "time": round(time.time() - start_time, 3)
    })


async def generate(func: Callable, request: dict, model: str, api: str) -> ChatCompletion | Completion:
    start_time = time.time()
    response: ChatCompletion | Completion = await func(**(request | {"model": model}))
    logger.info({
        "api": api,
        "request": request,
        "response": response.model_dump(),
        "time": round(time.time() - start_time, 3)
    })
    return response


def get_token(authorization: str) -> Optional[str]:
    prefix = 'Bearer '
    if authorization.startswith(prefix):
        return authorization.replace(prefix, '')
    return None


async def get_client(request: dict, authorization: str) -> Tuple[str, AsyncOpenAI]:
    if not (get_token(authorization) in token_list):
        raise HTTPException(status_code=401, detail="Invalid API key")
    namespace, model = await get_namespace_and_model(request["model"])
    if client_dict := ns_dict.get(namespace, {}):
        if client := client_dict.get(model, None):
            return model, client
        raise HTTPException(status_code=404, detail="Model not found")
    raise HTTPException(status_code=404, detail="Namespace not found")


@app.post("/v1/completions")
@app.post("/v1/chat/completions")
async def chat_completions(request: dict, authorization: Annotated[str | None, Header()], raw_request: Request):
    model, client = await get_client(request, authorization)
    api = raw_request.url.path
    create_func = (client.chat.completions if "chat" in api else client.completions).create
    args = (create_func, request, model, api)
    if request.get("stream", False):
        return EventSourceResponse(stream(*args), media_type="text/event-stream")
    return await generate(*args)


@app.get("/v1/models")
async def get_models(authorization: Annotated[str | None, Header()]) -> ModelList:
    if not (get_token(authorization) in token_list):
        raise HTTPException(status_code=401, detail="Invalid API key")
    return model_list


@app.get("/health")
async def health() -> Response:
    return Response(status_code=200)


def main():
    uvicorn.run(
        'main:app',
        host=os.getenv('HOST', '0.0.0.0'),
        port=int(os.getenv('PORT', '8000')),
        workers=int(os.getenv('WORKERS', '1'))
    )


if __name__ == '__main__':
    main()
