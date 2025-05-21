import time
from typing import Dict, List, Tuple, Iterable

from openai import AsyncOpenAI
from openai.types.model import Model

from openai_gateway.config import type_adapter, AliasConfig, ClientConfig
from openai_gateway.entity import ModelList


class ClientRouter:
    def __init__(self, config_string: str, api_keys: str):
        self.config = type_adapter.validate_json(config_string)
        self.map: Dict[str, Tuple[str, AsyncOpenAI]] = {}
        self.token_list: List[str] = []
        self.model_list: ModelList = ModelList()  # Response of /v1/models

        config_flatten_v2: Dict[str, tuple[str, ClientConfig]] = {}

        for namespace, client_config_list in self.config.items():
            for client_config in client_config_list:
                if isinstance(client_config, AliasConfig):
                    models: Iterable[str] = client_config.alias.keys()
                    alias_list: Iterable[str] = client_config.alias.values()
                    config_list: Iterable[tuple[str, ClientConfig]] = map(config_flatten_v2.__getitem__, alias_list)
                else:
                    models: list[str] = client_config.models
                    config_list: Iterable[tuple[str, ClientConfig]] = [(m, client_config) for m in models]
                key_list: Iterable[str] = map(lambda x: self.get_key(namespace, x), models)
                for k, (m, c) in zip(key_list, config_list):
                    if k in self.map:
                        raise ValueError(f"Duplicate model name detected: {k}")
                    config_flatten_v2[k] = (m, c)
                    self.map[k] = (m, c.to_client())
                    self.model_list.data.append(
                        Model(id=k, created=int(time.time()), owned_by=namespace, object="model")
                    )

        self.token_list.extend(api_keys.split(","))

    @classmethod
    def get_key(cls, namespace: str, model: str) -> str:
        return "/".join(([] if namespace == "default" else [namespace]) + [model])

    def __getitem__(self, key: str) -> Tuple[str, AsyncOpenAI]:
        return self.map[key]
