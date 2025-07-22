import os
import time
import tomllib
from contextlib import asynccontextmanager
from typing import AsyncIterator, Callable, Annotated

import uvicorn
from fastapi import FastAPI, HTTPException, Request, status, Depends, Header
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, Response
from fastapi.security import APIKeyHeader
from openai.types.chat.chat_completion import ChatCompletion
from openai.types.chat.chat_completion_chunk import ChatCompletionChunk
from openai.types.completion import Completion
from openai.types.embedding import Embedding
from sse_starlette.sse import EventSourceResponse

from openai_gateway.client_router import ClientRouter
from openai_gateway.entity import ModelList
from openai_gateway.logger import get_logger
from openai_gateway.project_root import get_project_root

GenRes = ChatCompletion | Completion | Embedding

logger = get_logger(__name__)
router: ClientRouter = ...
with open(os.path.join(get_project_root(), "pyproject.toml"), "rb") as f:
    version = tomllib.load(f)["project"]["version"]
uptime: float = time.time()


@asynccontextmanager
async def lifespan(_: FastAPI):
    global router
    router = ClientRouter(os.environ["CONFIG"], os.environ["API_KEYS"])
    yield


app = FastAPI(lifespan=lifespan, version=version)

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


def exclude_none(d: dict) -> dict:
    return {k: v for k, v in d.items() if v is not None}


async def stream(func: Callable, request: dict, model: str, api: str, *, request_id: str | None) -> AsyncIterator[str]:
    response: str = ""
    reasoning_content: str | None = None
    chunk: Completion | ChatCompletionChunk = ...
    start_time = time.time()
    async for chunk in await func(**(request | {"model": model})):
        yield chunk.model_dump_json()
        try:
            if isinstance(chunk, Completion):
                response += chunk.choices[0].text
            elif isinstance(chunk, ChatCompletionChunk):
                delta = chunk.choices[0].delta
                for key in ['content', 'reasoning_content']:
                    if hasattr(delta, key) and (v := getattr(delta, key)):
                        if key == 'reasoning_content':
                            reasoning_content = (reasoning_content or "") + v
                        if key == 'content':
                            response += v
            else:
                raise Exception("Unknown chunk type")
        except Exception as e:
            logger.exception(exclude_none({
                **request,
                "exception_class": e.__class__.__name__,
                "exception_message": str(e),
                "request_id": request_id,
            }))
    logger.info(exclude_none({
        "api": api,
        "request": request,
        "response": response,
        "chunk": None if chunk is ... else chunk.model_dump(),
        "time": round(time.time() - start_time, 3),
        "reasoning_content": reasoning_content,
        "request_id": request_id,
    }))
    yield "[DONE]"


async def generate(func: Callable, request: dict, model: str, api: str, *, request_id: str | None) -> GenRes:
    start_time = time.time()
    response: GenRes = await func(**(request | {"model": model}))
    logger.info(exclude_none({
        "api": api,
        "request": request,
        "response": response.model_dump(),
        "time": round(time.time() - start_time, 3),
        "request_id": request_id,
    }))
    return response


api_key_header = APIKeyHeader(name="Authorization")


def get_token(authorization: Annotated[str, Depends(api_key_header)]) -> str:
    if router.token_list:
        prefix = 'Bearer '
        if not authorization.startswith(prefix):
            raise HTTPException(status_code=401, detail="Invalid authorization header")
        token = authorization.replace(prefix, '')
        if token in router.token_list:
            return token
        raise HTTPException(status_code=401, detail="Invalid API key")


def get_request_id(x_request_id: Annotated[str | None, Header()] = None) -> str | None:
    return x_request_id


def process_enable_thinking(body: dict) -> dict:
    x = body.pop("enable_thinking", None)
    chat_template_kwargs = body.pop("chat_template_kwargs", {})
    y = chat_template_kwargs.get("enable_thinking", None)

    if chat_template_kwargs:
        body.setdefault("extra_body", {})["chat_template_kwargs"] = chat_template_kwargs

    if x is None and y is None:
        return body

    if x is not None or y is not None:
        if x is None:
            x = y
        elif y is None:
            y = x
        if x != y:
            raise HTTPException(status_code=400, detail="enable_thinking must be the same")
        extra_body = body.setdefault("extra_body", {})
        extra_body["enable_thinking"] = x
        extra_body.setdefault("chat_template_kwargs", {})["enable_thinking"] = x
    return body


@app.post("/v1/completions")
@app.post("/v1/chat/completions")
@app.post("/v1/embeddings")
@app.post("/v1/responses")
async def chat_completions(
        request: Request,
        _token: Annotated[str, Depends(get_token)],
        request_id: Annotated[str | None, Depends(get_request_id)],
):
    body: dict = await request.json()
    model, client = router[body["model"]]
    api = request.url.path
    method = client
    for each in api.split("/"):
        if each and each != "v1":
            method = getattr(method, each)

    body = process_enable_thinking(body)

    args = (method.create, body, model, api)
    if body.get("stream", False):
        return EventSourceResponse(stream(*args, request_id=request_id), media_type="text/event-stream")
    return await generate(*args, request_id=request_id)


@app.get("/v1/models")
async def get_models(_token: str = Depends(get_token)) -> ModelList:
    return router.model_list


@app.get("/health")
async def health():
    return {
        "uptime": round(time.time() - uptime, 3),
        "version": version,
    }


def main():
    uvicorn.run(
        'main:app',
        host=os.getenv('HOST', '0.0.0.0'),
        port=int(os.getenv('PORT', '8000')),
        workers=int(os.getenv('WORKERS', '1'))
    )


if __name__ == '__main__':
    main()
