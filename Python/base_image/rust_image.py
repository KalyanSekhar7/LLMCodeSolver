import re
import tomllib
from packaging.version import Version
from Orchestration.types import RustBuildType, RUST_BUILD_FILES
SUPPORTED_RUST_VERSIONS = [
    "1.70",
    "1.71",
    "1.72",
    "1.73",
    "1.74",
    "1.75",
]

DEFAULT_RUST_VERSION = "stable"


def choose_highest_rust(min_version: str) -> str:
    compatible = [
        v for v in SUPPORTED_RUST_VERSIONS
        if Version(v) >= Version(min_version)
    ]
    if compatible:
        return compatible[-1]
    return DEFAULT_RUST_VERSION


def resolve_rust_version(files: dict) -> str:
    """
    files = {
        "Cargo.toml": "...",
        "rust-toolchain": "...",
        "rust-toolchain.toml": "...",
    }
    """

    # 1️⃣ rust-toolchain.toml
    if "rust-toolchain.toml" in files:
        data = tomllib.loads(files["rust-toolchain.toml"].encode())
        channel = data.get("toolchain", {}).get("channel")
        if channel:
            return channel

    # 2️⃣ rust-toolchain
    if "rust-toolchain" in files:
        return files["rust-toolchain"].strip()

    # 3️⃣ Cargo.toml
    if "Cargo.toml" in files:
        data = tomllib.loads(files["Cargo.toml"].encode())
        rust_version = data.get("package", {}).get("rust-version")
        if rust_version:
            return choose_highest_rust(rust_version)

    # fallback
    return DEFAULT_RUST_VERSION


def rust_version_to_image(version: str) -> str:
    return f"rust:{version}"

def detect_rust_build_type(files: dict) -> RustBuildType:
    for build_type, indicators in RUST_BUILD_FILES.items():
        if any(f in files for f in indicators):
            return build_type
    return RustBuildType.UNKNOWN

def get_rust_binary_name(files: dict) -> str:
    import re

    if "Cargo.toml" in files:
        match = re.search(r'^name\s*=\s*"([^"]+)"', files["Cargo.toml"], re.MULTILINE)
        if match:
            return match.group(1)

    return "app"


def rust_install_commands(build_type: RustBuildType, files: dict) -> list[str]:
    binary_name = get_rust_binary_name(files)

    return [
        # Copy manifests first (for Docker cache efficiency)
        "COPY Cargo.toml Cargo.lock* ./",

        # Dummy build to cache dependencies
        "RUN mkdir -p src && echo 'fn main() {}' > src/main.rs",
        "RUN cargo build --release",
        "RUN rm -rf src",

        # Copy actual source
        "COPY . .",

        # Final optimized build
        "RUN cargo build --release",

        # Run correct binary
        f'CMD ["./target/release/{binary_name}"]'
    ]

