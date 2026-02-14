from urllib.parse import urlparse
from Orchestration.types import PythonBuildType,PYTHON_BUILD_FILES,LANGUAGE_BUILD_MAP
from Orchestration.base_image.python_image import resolve_python_version
import requests
def parse_github_url(repo_url: str):
    parsed = urlparse(repo_url)
    parts = parsed.path.strip("/").split("/")
    return parts[0], parts[1]


import requests

def get_default_branch(owner: str, repo: str) -> str:
    url = f"https://api.github.com/repos/{owner}/{repo}"
    r = requests.get(url)

    if r.status_code != 200:
        raise Exception("Repository not found")

    data = r.json()
    return data["default_branch"]

def get_repo_file_index(repo_url: str,repo_language: str = "python") -> dict:
    owner, repo = parse_github_url(repo_url)

    branch = get_default_branch(owner, repo)

    files = list_repo_files(owner, repo, branch)

    return files


import requests


def list_repo_files(owner: str, repo: str, branch: str, repo_language: str = "python") -> dict:
    
    url = f"https://api.github.com/repos/{owner}/{repo}/contents"
    params = {"ref": branch}

    r = requests.get(url, params=params)

    if r.status_code != 200:
        raise Exception("Could not fetch repository contents")

    items = r.json()

    # Get only root-level file/directory names
    repo_items = {item["name"]: item["type"] for item in items}

    # Normalize language key
    repo_language = repo_language.lower()

    if repo_language not in LANGUAGE_BUILD_MAP:
        return {}

    build_config = LANGUAGE_BUILD_MAP[repo_language]

    # Flatten expected build filenames
    expected_files = set()
    for file_list in build_config.values():
        expected_files.update(file_list)

    # Filter only matching build files
    filtered = {
        name: repo_items[name]
        for name in repo_items
        if name in expected_files
    }

    return filtered





    
repo_files = get_repo_file_index("https://github.com/scikit-learn/scikit-learn")
python_versoin = resolve_python_version(repo_files)
print(python_versoin)
# build_type = detect_python_build_type(repo_files)
# print(build_type)