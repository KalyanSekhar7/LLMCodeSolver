import re
from Orchestration.types import GoBuildType, GO_BUILD_FILES
DEFAULT_GO_VERSION = "1.22"


def resolve_go_version(files: dict) -> str:
    """
    files = {
        "go.mod": "...",
        "go.work": "...",
    }
    """

    # 1️⃣ go.mod
    if "go.mod" in files:
        match = re.search(r'^go\s+([\d.]+)', files["go.mod"], re.MULTILINE)
        if match:
            return match.group(1)

    # 2️⃣ go.work (rare)
    if "go.work" in files:
        match = re.search(r'^go\s+([\d.]+)', files["go.work"], re.MULTILINE)
        if match:
            return match.group(1)

    # fallback
    return DEFAULT_GO_VERSION


def go_version_to_image(version: str) -> str:
    return f"golang:{version}"


def detect_go_build_type(files: dict) -> GoBuildType:
    for build_type, indicators in GO_BUILD_FILES.items():
        if any(f in files for f in indicators):
            return build_type
    return GoBuildType.UNKNOWN

def go_install_commands(build_type: GoBuildType) -> list[str]:
    return [
        # Download dependencies first (better caching)
        "COPY go.mod go.sum* ./",
        "RUN go mod download",

        # Copy rest of source
        "COPY . .",

        # Build binary
        "RUN go build -o app",

        # Default run command
        'CMD ["./app"]'
    ]
