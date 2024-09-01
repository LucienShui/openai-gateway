import random
from typing import Callable, Dict, Tuple

from openai import AsyncOpenAI


class ClientBase(AsyncOpenAI):
    def __int__(self):
        super().__init__(api_key="placeholder", base_url="https://fake.local")


class PoolClient(ClientBase):
    def __init__(self, clients: Dict[str, Tuple[AsyncOpenAI, int]]):
        super().__init__()

        self.clients: Dict[str, Tuple[AsyncOpenAI, int]] = clients
        self.names = [k for k in self.clients.keys()]
        self.weights = [v[1] for v in self.clients.values()]

        self.completions.create = self.create_wrapper(chat_mode=False)
        self.chat.completions.create = self.create_wrapper(chat_mode=True)

    def create_wrapper(self, chat_mode: bool) -> Callable:
        async def create(*args, **kwargs):
            name: str = random.choices(self.names, weights=self.weights, k=1)[0]
            client: AsyncOpenAI = self.clients[name][0]
            return (client.chat.completions if chat_mode else client.completions).create(*args, **kwargs)

        return create


class KeyPoolClient(PoolClient):

    def __init__(self, api_keys: Dict[str, int], base_url: str):
        clients = {f"client_{i}": (AsyncOpenAI(api_key=k, base_url=base_url), w)
                   for i, (k, w) in enumerate(api_keys.items())}
        super().__init__(clients)
