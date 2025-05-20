import os

import pytest


@pytest.fixture(scope="function")
def project_root() -> str:
    target: str = 'pyproject.toml'
    path = os.path.dirname(os.path.abspath(__file__))
    while target not in os.listdir(path):
        path = os.path.dirname(path)
    return path
