from typing import List, Dict, Optional

from pydantic import BaseModel, Field


class API(BaseModel):
    model_list: List[str] = Field(description="List of supported models")
    api_key: str = Field(description="OpenAI API key")
    base_url: str = Field(description="OpenAI API base url")

    is_azure: Optional[bool] = Field(default=False, description="Is Azure API")
    azure_deployment: Optional[str] = Field(default=None, description="Azure deployment name")
    api_version: Optional[str] = Field(default=None, description="Azure API version")


class Config(BaseModel):
    config: Dict[str, List[API]] = Field(description="namespace to API list")
