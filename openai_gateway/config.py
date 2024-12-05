from typing import List, Dict, Literal, Union

from openai import AsyncOpenAI
from openai.lib.azure import AsyncAzureOpenAI
from pydantic import BaseModel, Field, TypeAdapter
from typing_extensions import Annotated


class BaseClientConfig(BaseModel):
    type: Literal["openai", "azure", "alias"]

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        raise NotImplementedError


class BaseOpenAIConfig(BaseClientConfig):
    models: List[str] = Field(description="List of supported models")
    api_key: str = Field(description="OpenAI API key")


class OpenAIConfig(BaseOpenAIConfig):
    type: Literal["openai"]

    base_url: str = Field(description="OpenAI API base url")

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        return AsyncOpenAI(api_key=self.api_key, base_url=self.base_url)


class AzureConfig(BaseOpenAIConfig):
    type: Literal["azure"]

    azure_endpoint: str = Field(description="Azure endpoint", examples=["https://***.openai.azure.com/"])
    api_version: str = Field(description="Azure API version", examples=["2024-02-15-preview"])

    def to_client(self, *args, **kwargs) -> AsyncOpenAI:
        return AsyncAzureOpenAI(azure_endpoint=self.azure_endpoint, api_key=self.api_key, api_version=self.api_version)


class AliasConfig(BaseClientConfig):
    type: Literal["alias"]

    alias: Dict[str, str] = Field(description="Alias for model")

    @property
    def models(self) -> List[str]:
        return list(self.alias.keys())


ClientConfig = Annotated[Union[OpenAIConfig, AzureConfig, AliasConfig], Field(discriminator="type")]

type_adapter: TypeAdapter[Dict[str, List[ClientConfig]]] = TypeAdapter(Dict[str, List[ClientConfig]])
