from typing import List

from openai.types.model import Model
from pydantic import BaseModel


class ModelList(BaseModel):
    object: str = "list"
    data: List[Model] = []
