
from enum import Enum, StrEnum

class PythonBuildType(Enum):
    PYPROJECT = "pyproject"
    SETUP = "setup"
    REQUIREMENTS = "requirements"
    PIPFILE = "pipfile"
    CONDA = "conda"
    UNKNOWN = "unknown"

PYTHON_BUILD_FILES = {
    PythonBuildType.PYPROJECT: ["pyproject.toml"],
    PythonBuildType.SETUP: ["setup.py"],
    PythonBuildType.REQUIREMENTS: ["requirements.txt"],
    PythonBuildType.PIPFILE: ["Pipfile"],
    PythonBuildType.CONDA: ["environment.yml", "environment.yaml"],
}

from enum import StrEnum

class JSBuildType(StrEnum):
    NPM = "npm"
    YARN = "yarn"
    PNPM = "pnpm"
    UNKNOWN = "unknown"

JS_BUILD_FILES = {
    JSBuildType.PNPM: ["pnpm-lock.yaml"],
    JSBuildType.YARN: ["yarn.lock"],
    JSBuildType.NPM: ["package-lock.json", "package.json"],
}


class GoBuildType(Enum):
    MODULES = "modules"
    DEP = "dep"
    GLIDE = "glide"
    VENDOR = "vendor"
    BASIC = "basic"
    UNKNOWN = "unknown"
    
    
GO_BUILD_FILES = {
    GoBuildType.MODULES: ["go.mod"],
    GoBuildType.DEP: ["Gopkg.toml"],
    GoBuildType.GLIDE: ["glide.yaml"],
    GoBuildType.VENDOR: ["vendor"],  # directory
}


class RustBuildType(Enum):
    CARGO = "cargo"
    BASIC = "basic"
    UNKNOWN = "unknown"

RUST_BUILD_FILES = {
    RustBuildType.CARGO: ["Cargo.toml"],
}



from dataclasses import dataclass
from typing import Dict, Optional

@dataclass
class RepoContext:
    repo_url: str
    files: Dict[str, str]
    language: Optional[str] = None
    build_type: Optional[str] = None
    runtime_version: Optional[str] = None
    docker_image: Optional[str] = None


LANGUAGE_BUILD_MAP = {
    "python": PYTHON_BUILD_FILES,
    "javascript": JS_BUILD_FILES,
    "js": JS_BUILD_FILES,
    "go": GO_BUILD_FILES,
    "rust": RUST_BUILD_FILES,
}
