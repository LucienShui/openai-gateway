import pytest

from openai_gateway.project_root import get_project_root


@pytest.fixture(scope="function")
def project_root() -> str:
    return get_project_root()
