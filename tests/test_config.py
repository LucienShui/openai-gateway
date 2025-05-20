import os

import pytest

from main import Config
from tests.fixture import project_root


@pytest.mark.parametrize("config_file, ops", [
    ("config.json", [
        {"op": "eq", "key": "alias/deepseek", "value": "deepseek-chat"},
        {"op": "eq", "key": "multi/level/4o", "value": "gpt-4o"},
        {"op": "eq", "key": "gpt-4o", "value": "gpt-4o"},
    ]),
    ("duplicate.json", ValueError)
])
def test_config(project_root: str, config_file: str, ops: list[dict] | type):
    with open(os.path.join(project_root, "tests/resources", config_file)) as f:
        config_string: str = f.read()

    if isinstance(ops, type):
        assert issubclass(ops, Exception)
        with pytest.raises(ops):
            Config(config_string, "")
    else:
        config = Config(config_string, "")
        for op_dict in ops:
            op: str = op_dict["op"]
            key: str = op_dict["key"]
            value: str = op_dict["value"]

            if op == "eq":
                model, client = config.get_client(key)
                assert model == value
