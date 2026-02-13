import os


def create_dockerfile(
    base_image: str,
    repo_name: str,
    repo_url: str,
    install_steps: list[str] | None = None,
    output_path: str = "Dockerfile",
):
    install_steps = install_steps or []

    base_lines = [
        f"FROM {base_image}",
        "",
        "ENV DEBIAN_FRONTEND=noninteractive",
        "",
        "RUN apt-get update && apt-get install -y \\",
        "    git \\",
        "    build-essential \\",
        "    bash \\",
        "    sudo \\",
        "    curl \\",
        "    ca-certificates \\",
        "    --no-install-recommends \\",
        "    && rm -rf /var/lib/apt/lists/*",
        "",
        "RUN mkdir -p /working_directory/testbed",
        "",
        "WORKDIR /working_directory/testbed",
        "",
        f"RUN git clone {repo_url} {repo_name}",
        "",
        f"WORKDIR /working_directory/testbed/{repo_name}",
        "",
    ]

    # Inject language-specific install/build steps
    base_lines.extend(install_steps)

    # If no CMD was defined in install_steps, fallback to bash
    if not any(step.strip().startswith("CMD") for step in install_steps):
        base_lines.append('CMD ["/bin/bash"]')

    dockerfile_content = "\n".join(base_lines)

    with open(output_path, "w") as f:
        f.write(dockerfile_content)

    print(f"Dockerfile created at {os.path.abspath(output_path)}")
