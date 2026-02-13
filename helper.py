import requests
from types import GoBuildType, JSBuildType, PythonBuildType, RustBuildType
    




def detect_build_type(repo_url: str) -> PythonBuildType:
    parsed = urlparse(repo_url)
    parts = parsed.path.strip("/").split("/")
    owner, repo = parts[0], parts[1]

    api_url = f"https://api.github.com/repos/{owner}/{repo}/contents"
    response = requests.get(api_url)

    if response.status_code != 200:
        return PythonBuildType.UNKNOWN

    files = [item["name"] for item in response.json()]

    if "pyproject.toml" in files:
        return PythonBuildType.PYPROJECT
    elif "setup.py" in files:
        return PythonBuildType.SETUP
    elif "requirements.txt" in files:
        return PythonBuildType.REQUIREMENTS
    elif "Pipfile" in files:
        return PythonBuildType.PIPFILE
    elif "environment.yml" in files or "environment.yaml" in files:
        return PythonBuildType.CONDA
    else:
        return PythonBuildType.UNKNOWN


 

def detect_go_build_type(repo_url: str) -> GoBuildType:
    parsed = urlparse(repo_url)
    parts = parsed.path.strip("/").split("/")
    owner, repo = parts[0], parts[1]

    api_url = f"https://api.github.com/repos/{owner}/{repo}/contents"
    response = requests.get(api_url)

    if response.status_code != 200:
        return GoBuildType.UNKNOWN

    items = response.json()
    names = [item["name"] for item in items]

    if "go.mod" in names:
        return GoBuildType.MODULES
    elif "Gopkg.toml" in names:
        return GoBuildType.DEP
    elif "glide.yaml" in names:
        return GoBuildType.GLIDE
    elif any(item["type"] == "dir" and item["name"] == "vendor" for item in items):
        return GoBuildType.VENDOR
    else:
        return GoBuildType.BASIC



import requests
from urllib.parse import urlparse


def detect_rust_build_type(repo_url: str) -> RustBuildType:
    parsed = urlparse(repo_url)
    parts = parsed.path.strip("/").split("/")
    owner, repo = parts[0], parts[1]

    api_url = f"https://api.github.com/repos/{owner}/{repo}/contents"
    response = requests.get(api_url)

    if response.status_code != 200:
        return RustBuildType.UNKNOWN

    files = [item["name"] for item in response.json()]

    if "Cargo.toml" in files:
        return RustBuildType.CARGO
    else:
        return RustBuildType.UNKNOWN



def detect_js_build_type(repo_url: str) -> JSBuildType:
    parsed = urlparse(repo_url)
    parts = parsed.path.strip("/").split("/")
    owner, repo = parts[0], parts[1]

    api_url = f"https://api.github.com/repos/{owner}/{repo}/contents"
    response = requests.get(api_url)

    if response.status_code != 200:
        return JSBuildType.UNKNOWN

    files = [item["name"] for item in response.json()]

    if "pnpm-lock.yaml" in files:
        return JSBuildType.PNPM
    elif "yarn.lock" in files:
        return JSBuildType.YARN
    elif "package-lock.json" in files:
        return JSBuildType.NPM
    elif "package.json" in files:
        return JSBuildType.NPM
    else:
        return JSBuildType.UNKNOWN
