import json
import re
from packaging.specifiers import SpecifierSet
from packaging.version import Version
from Orchestration.types import JSBuildType, JS_BUILD_FILES 
SUPPORTED_NODE_VERSIONS = ["16", "18", "20", "21"]
DEFAULT_NODE_VERSION = "20"


def choose_highest_node(specifier: str) -> str:
    try:
        spec = SpecifierSet(specifier)
        compatible = [
            v for v in SUPPORTED_NODE_VERSIONS
            if Version(v) in spec
        ]
        if compatible:
            return compatible[-1]
    except Exception:
        pass

    return DEFAULT_NODE_VERSION


def resolve_node_version(files: dict) -> str:
    """
    files = {
        ".nvmrc": "...",
        ".node-version": "...",
        "package.json": "...",
    }
    """

    # 1️⃣ .nvmrc
    if ".nvmrc" in files:
        version = files[".nvmrc"].strip().lstrip("v")
        return version

    # 2️⃣ .node-version
    if ".node-version" in files:
        return files[".node-version"].strip()

    # 3️⃣ package.json
    if "package.json" in files:
        try:
            data = json.loads(files["package.json"])
            engines = data.get("engines", {})
            node_spec = engines.get("node")
            if node_spec:
                return choose_highest_node(node_spec)
        except Exception:
            pass

    # fallback
    return DEFAULT_NODE_VERSION






def detect_js_build_type(files: dict) -> JSBuildType:
    for build_type, indicators in JS_BUILD_FILES.items():
        if any(f in files for f in indicators):
            return build_type
    return JSBuildType.UNKNOWN



from Orchestration.types import JSBuildType


def js_install_commands(build_type: JSBuildType) -> list[str]:
    
    if build_type == JSBuildType.PNPM:
        return [
            "RUN npm install -g pnpm",
            "COPY package.json pnpm-lock.yaml ./",
            "RUN pnpm install --frozen-lockfile"
        ]

    elif build_type == JSBuildType.YARN:
        return [
            "COPY package.json yarn.lock ./",
            "RUN yarn install --frozen-lockfile"
        ]

    elif build_type == JSBuildType.NPM:
        return [
            "COPY package.json package-lock.json* ./",
            "RUN npm ci"
        ]

    # fallback if only package.json
    return [
        "COPY package.json ./",
        "RUN npm install"
    ]


DEFAULT_NODE_VERSION = "20"


def node_version_to_image(version: str | None = None) -> str:
    version = version or DEFAULT_NODE_VERSION
    return f"node:{version}-slim"
