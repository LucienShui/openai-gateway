import os


def get_project_root(target: str = 'pyproject.toml') -> str:
    path = os.path.dirname(os.path.abspath(__file__))
    while target not in os.listdir(path):
        path = os.path.dirname(path)
    return path


project_root: str = get_project_root()
