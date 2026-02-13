import re
from packaging.specifiers import SpecifierSet
from packaging.version import Version
from Orchestration.types import PythonBuildType, PYTHON_BUILD_FILES
SUPPORTED_PYTHON_VERSIONS = [
    "3.8",
    "3.9",
    "3.10",
    "3.11",
    "3.12"
]

DEFAULT_VERSION = "3.11"


def choose_highest_compatible(specifier: str) -> str:
    """
    Given something like '>=3.8,<4'
    return highest compatible version from SUPPORTED list
    """
    try:
        spec = SpecifierSet(specifier)
        compatible = [
            v for v in SUPPORTED_PYTHON_VERSIONS
            if Version(v) in spec
        ]
        if compatible:
            return compatible[-1]  # highest
    except Exception:
        pass

    return DEFAULT_VERSION


def resolve_python_version(files: dict) -> str:
    """
    files = {
        "pyproject.toml": "...",
        ".python-version": "...",
        "runtime.txt": "...",
        ...
    }
    """

    # 1️⃣ .python-version
    if ".python-version" in files:
        return files[".python-version"].strip()

    # 2️⃣ runtime.txt
    if "runtime.txt" in files:
        match = re.search(r"python-?([\d\.]+)", files["runtime.txt"])
        if match:
            return match.group(1)

    # 3️⃣ pyproject.toml
    if "pyproject.toml" in files:
        content = files["pyproject.toml"]
        match = re.search(r'requires-python\s*=\s*"([^"]+)"', content)
        if match:
            return choose_highest_compatible(match.group(1))

        # poetry style
        match = re.search(r'python\s*=\s*"([^"]+)"', content)
        if match:
            return choose_highest_compatible(match.group(1))

    # 4️⃣ Pipfile
    if "Pipfile" in files:
        match = re.search(r'python_version\s*=\s*"([^"]+)"', files["Pipfile"])
        if match:
            return match.group(1)

    # 5️⃣ setup.py
    if "setup.py" in files:
        match = re.search(r'python_requires\s*=\s*"([^"]+)"', files["setup.py"])
        if match:
            return choose_highest_compatible(match.group(1))

    # fallback
    return DEFAULT_VERSION
resolve_python_version

def python_version_to_image(version: str) -> str:
    return f"python:{version}-slim"


def detect_python_build_type(files: dict) -> PythonBuildType:
    for build_type, indicators in PYTHON_BUILD_FILES.items():
        if any(f in files for f in indicators):
            return build_type
    return PythonBuildType.UNKNOWN



from Orchestration.types import PythonBuildType


def python_install_commands(build_type: PythonBuildType) -> list[str]:
    if build_type == PythonBuildType.REQUIREMENTS:
        return [
            "COPY requirements.txt .",
            "RUN pip install --no-cache-dir -r requirements.txt"
        ]

    elif build_type == PythonBuildType.PYPROJECT:
        return [
            "COPY pyproject.toml .",
            "RUN pip install uv",
            "RUN uv sync"
        ]

    elif build_type == PythonBuildType.PIPFILE:
        return [
            "COPY Pipfile Pipfile.lock* .",
            "RUN pip install pipenv",
            "RUN pipenv install --system --deploy"
        ]

    elif build_type == PythonBuildType.SETUP:
        return [
            "COPY . .",
            "RUN pip install ."
        ]

    elif build_type == PythonBuildType.CONDA:
        return [
            "COPY environment.yml .",
            "RUN apt-get update && apt-get install -y curl",
            "RUN curl -sL https://repo.anaconda.com/miniconda.sh -o miniconda.sh",
            "RUN bash miniconda.sh -b -p /opt/conda",
            "ENV PATH=/opt/conda/bin:$PATH",
            "RUN conda env update -f environment.yml"
        ]

    return []
