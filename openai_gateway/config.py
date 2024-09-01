from typing import List, Dict, Literal, Union
from openai_gateway.client import KeyPoolClient

from openai import AsyncOpenAI
from openai.lib.azure import AsyncAzureOpenAI
from pydantic import BaseModel, Field
from typing_extensions import Annotated


class ClientConfigBase(BaseModel):
    models: List[str] = Field(description="List of supported models")

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        raise NotImplementedError


class OpenAIConfig(ClientConfigBase):
    type: Literal["openai"]

    api_key: str = Field(description="OpenAI API key")
    base_url: str = Field(description="OpenAI API base url")

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        return AsyncOpenAI(api_key=self.api_key, base_url=self.base_url)


class AzureConfig(ClientConfigBase):
    type: Literal["azure"]

    api_key: str = Field(description="Azure API key")
    azure_endpoint: str = Field(description="Azure endpoint", examples=["https://***.openai.azure.com/"])
    api_version: str = Field(description="Azure API version", examples=["2024-02-15-preview"])

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        return AsyncAzureOpenAI(azure_endpoint=self.azure_endpoint, api_key=self.api_key, api_version=self.api_version)


class PoolConfig(ClientConfigBase):
    type: Literal["pool"]

    model_pool: Union[List[str], Dict[str, int]]

    def to_client(self, route: Dict[str, Dict[str, AsyncOpenAI]], *args, **kwargs) -> AsyncOpenAI:
        pass


class KeyPoolConfig(ClientConfigBase):
    type: Literal["key_pool"]

    api_keys: Union[List[str], Dict[str, int]]
    base_url: str = Field(description="OpenAI API base url")

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        api_keys: Dict[str, int] = {k: 1 for k in self.api_keys} if isinstance(self.api_keys, list) else self.api_keys
        return KeyPoolClient(api_keys=api_keys, base_url=self.base_url)


ClientConfig = Annotated[Union[OpenAIConfig, AzureConfig], Field(discriminator="type")]


class Config(BaseModel):
    namespace: Dict[str, List[ClientConfig]] = Field(description="namespace to client config list")
