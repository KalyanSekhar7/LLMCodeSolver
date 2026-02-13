import yaml
from Orchestration.create_dockerfile import create_dockerfile
from pathlib import Path
if __name__ == "__main__":

    BASE_DIR = Path(__file__).resolve().parent
    config_path = BASE_DIR / "orchestration_config.yaml"
    with open(config_path, "r") as f:
        config = yaml.safe_load(f)

    repo_url = config["repository"]["url"]
    repo_name = config["repository"]["name"]
    
    
    repo_laanguage = config["type"]
    
    if repo_laanguage == "python":
        from Orchestration.base_image_extractor import get_repo_file_index, resolve_python_version
        from Orchestration.base_image.python_image import python_version_to_image,detect_python_build_type, PythonBuildType,python_install_commands

        repo_files = get_repo_file_index(repo_url)
        python_version = resolve_python_version(repo_files)
        base_image = python_version_to_image(python_version)
        build_types = detect_python_build_type(repo_files)
        install_steps = python_install_commands(build_types)

        create_dockerfile(base_image, repo_name, repo_url,install_steps)
    elif repo_laanguage == "go":
        from Orchestration.base_image_extractor import get_repo_file_index
        from Orchestration.base_image.go_image import (
            resolve_go_version,
            go_version_to_image,
            detect_go_build_type,
            go_install_commands
        )

        repo_files = get_repo_file_index(repo_url)

        go_version = resolve_go_version(repo_files)
        base_image = go_version_to_image(go_version)

        build_type = detect_go_build_type(repo_files)
        install_steps = go_install_commands(build_type)

        create_dockerfile(
            base_image=base_image,
            repo_name=repo_name,
            repo_url=repo_url,
            install_steps=install_steps
        )

    elif repo_laanguage == "rust":
        from Orchestration.base_image_extractor import get_repo_file_index
        from Orchestration.base_image.rust_image import (
            resolve_rust_version,
            rust_version_to_image,
            detect_rust_build_type,
            rust_install_commands,
        )

        repo_files = get_repo_file_index(repo_url)

        # Resolve Rust version
        rust_version = resolve_rust_version(repo_files)
        base_image = rust_version_to_image(rust_version)

        # Detect build type (Cargo etc.)
        build_type = detect_rust_build_type(repo_files)

        # Generate install/build steps (includes dynamic binary name)
        install_steps = rust_install_commands(build_type, repo_files)

        create_dockerfile(
            base_image=base_image,
            repo_name=repo_name,
            repo_url=repo_url,
            install_steps=install_steps,
        )

    elif repo_laanguage == "javascript":
        from Orchestration.base_image_extractor import get_repo_file_index
        from Orchestration.base_image.node_version import resolve_node_version, node_version_to_image,js_install_commands,detect_js_build_type, JSBuildType

        repo_files = get_repo_file_index(repo_url)
        js_version = resolve_node_version(repo_files)
        base_image = node_version_to_image(js_version)
        build_types = detect_js_build_type(repo_files)
        install_steps = js_install_commands(build_types)

        create_dockerfile(base_image, repo_name, repo_url,install_steps)
    