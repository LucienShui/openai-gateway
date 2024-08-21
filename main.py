import os
import time
from contextlib import asynccontextmanager
from typing import Dict, Annotated, AsyncIterable, List, Tuple, Optional

import uvicorn
from fastapi import FastAPI, HTTPException, Request, status, Header
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, Response
from openai import AsyncOpenAI
from openai.lib.azure import AsyncAzureOpenAI
from openai.types.chat.chat_completion import ChatCompletion
from openai.types.chat.chat_completion_chunk import ChatCompletionChunk
from openai.types.model import Model
from sse_starlette.sse import EventSourceResponse

from openai_gateway.config import Config, API
from openai_gateway.logger import get_logger

client_dict: Dict[str, Dict[str, AsyncOpenAI]] = {}
token_list: List[str] = []
model_list: List[Model] = []
logger = get_logger(__name__)


@asynccontextmanager
async def lifespan(_: FastAPI):
    config: Dict[str, List[API]] = Config.model_validate_json(os.environ["CONFIG"]).config
    for namespace, api_list in config.items():
        for api in api_list:
            for model in api.model_list:
                if api.is_azure:
                    client_dict.setdefault(namespace, {})[model] = AsyncAzureOpenAI(
                        azure_endpoint=api.base_url, api_version=api.api_version, api_key=api.api_key)
                else:
                    client_dict.setdefault(namespace, {})[model] = AsyncOpenAI(api_key=api.api_key,
                                                                               base_url=api.base_url)
                model_list.append(Model(
                    id="/".join(([namespace] if namespace != "default" else []) + [model]),
                    created=int(time.time())
                ))

    token_list.extend(os.environ["API_KEY_LIST"].split(","))
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
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={
            "error": f"{e.__class__.__name__}: {str(e)}",
            "status": 500
        }
    )


async def get_namespace_and_model(key: str) -> Tuple[str, str]:
    if "/" in key:
        namespace, model = key.split("/")
        return namespace, model
    return "default", key


async def stream(request: dict) -> AsyncIterable[str]:
    namespace, model = await get_namespace_and_model(request["model"])
    response = ""
    chunk: ChatCompletionChunk = ...
    start_time = time.time()
    async for chunk in await client_dict[namespace][model].chat.completions.create(**{**request, "model": model}):
        yield chunk.model_dump_json()
        try:
            response += chunk.choices[0].delta.content or ""
        except Exception as e:
            logger.exception({
                **request,
                "exception_class": e.__class__.__name__,
                "exception_message": str(e)
            })
    logger.info({
        "api": "/v1/chat/completions",
        "request": request,
        "response": response,
        "chunk": None if chunk is ... else chunk.model_dump(),
        "time": round(time.time() - start_time, 3)
    })


def get_token(authorization: str) -> Optional[str]:
    prefix = 'Bearer '
    if authorization.startswith(prefix):
        return authorization.replace(prefix, '')
    return None


@app.post("/v1/chat/completions")
async def create_chat_completion(request: dict, authorization: Annotated[str | None, Header()]):
    if not (get_token(authorization) in token_list):
        raise HTTPException(status_code=401, detail="Invalid API key")
    if request.get("stream", False):
        return EventSourceResponse(stream(request), media_type="text/event-stream")
    else:
        start_time = time.time()
        namespace, model = await get_namespace_and_model(request["model"])
        params = {**request, "model": model}
        response: ChatCompletion = await client_dict[namespace][model].chat.completions.create(**params)
        logger.info({
            "api": "/v1/chat/completions",
            "request": request,
            "response": response.model_dump(),
            "time": round(time.time() - start_time, 3)
        })
        return response


@app.get("/v1/models")
async def get_models(authorization: Annotated[str | None, Header()]) -> List[Model]:
    if not (get_token(authorization) in token_list):
        raise HTTPException(status_code=401, detail="Invalid API key")
    return model_list


def main():
    uvicorn.run(
        'main:app',
        host=os.getenv('HOST', '0.0.0.0'),
        port=int(os.getenv('PORT', '8000')),
        workers=int(os.getenv('WORKERS', '1'))
    )


if __name__ == '__main__':
    main()
