# Copyright 2026 Alibaba Group Holding Ltd.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import urllib.request
from pathlib import Path


def _is_arm64() -> bool:
    """Check if running on ARM64 architecture."""
    machine = platform.machine().lower()
    return machine in ("arm64", "aarch64")


def _download(url: str) -> str:
    with urllib.request.urlopen(url) as resp:
        return resp.read().decode("utf-8")


def _clone_dify_docker_dir(target_dir: Path, dify_ref: str) -> None:
    """Clone only the docker directory from Dify repo using sparse checkout."""
    if target_dir.exists():
        shutil.rmtree(target_dir)
    target_dir.mkdir(parents=True, exist_ok=True)
    
    # Use git sparse checkout to clone only docker directory
    subprocess.run(
        ["git", "clone", "--depth=1", "--filter=blob:none", "--sparse",
         "-b", dify_ref, "https://github.com/langgenius/dify.git", str(target_dir)],
        check=True, capture_output=True
    )
    subprocess.run(
        ["git", "sparse-checkout", "set", "docker"],
        cwd=str(target_dir), check=True, capture_output=True
    )
    
    # Move docker contents to target_dir root
    docker_dir = target_dir / "docker"
    if docker_dir.exists():
        for item in docker_dir.iterdir():
            dest = target_dir / item.name
            if dest.exists():
                if dest.is_dir():
                    shutil.rmtree(dest)
                else:
                    dest.unlink()
            shutil.move(str(item), str(target_dir))
        docker_dir.rmdir()
    
    # Remove .git directory
    git_dir = target_dir / ".git"
    if git_dir.exists():
        shutil.rmtree(git_dir)


def _write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def _update_env(env_path: Path, updates: dict[str, str]) -> None:
    existing = env_path.read_text(encoding="utf-8").splitlines()
    seen = set()
    updated_lines = []
    for line in existing:
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in line:
            updated_lines.append(line)
            continue
        key, _ = line.split("=", 1)
        if key in updates:
            updated_lines.append(f"{key}={updates[key]}")
            seen.add(key)
        else:
            updated_lines.append(line)
    for key, value in updates.items():
        if key not in seen:
            updated_lines.append(f"{key}={value}")
    env_path.write_text("\n".join(updated_lines) + "\n", encoding="utf-8")


def main() -> None:
    base_dir = Path(__file__).resolve().parent
    target_dir = Path(os.environ.get("DIFY_COMPOSE_DIR", base_dir / ".dify"))
    # Use latest stable release tag, not main branch (which may reference unreleased images)
    dify_ref = os.environ.get("DIFY_REF", "1.11.4")
    dify_port = os.environ.get("DIFY_PORT", "5001")
    use_mirror = os.environ.get("USE_DOCKER_MIRROR", "").lower() in ("1", "true", "yes")

    print(f"Cloning Dify docker directory (ref: {dify_ref})...")
    _clone_dify_docker_dir(target_dir, dify_ref)
    print(f"Dify docker directory cloned to: {target_dir}")

    # Optionally replace images with mirror
    compose_path = target_dir / "docker-compose.yaml"
    if use_mirror and not _is_arm64():
        compose_content = compose_path.read_text(encoding="utf-8")
        compose_content = _replace_images_with_mirror(compose_content)
        compose_path.write_text(compose_content, encoding="utf-8")
        print("Using Docker mirror for amd64 images")
    elif use_mirror and _is_arm64():
        print("Skipping Docker mirror on ARM64 (mirror images are amd64 only)")

    # Copy .env.example to .env and update
    env_example = target_dir / ".env.example"
    env_path = target_dir / ".env"
    if env_example.exists():
        shutil.copy(env_example, env_path)

    base_url = f"http://localhost:{dify_port}"
    _update_env(
        env_path,
        {
            "DIFY_PORT": dify_port,
            # Expose nginx on the configured port
            "NGINX_PORT": dify_port,
            "EXPOSE_NGINX_PORT": dify_port,
            "CONSOLE_API_URL": base_url,
            "CONSOLE_WEB_URL": base_url,
            "SERVICE_API_URL": base_url,
            "APP_API_URL": base_url,
            "APP_WEB_URL": base_url,
            # Ensure plugin debugging is exposed to host
            "EXPOSE_PLUGIN_DEBUGGING_HOST": "localhost",
            "EXPOSE_PLUGIN_DEBUGGING_PORT": "5003",
        },
    )
    print("Dify compose files ready")


def _replace_images_with_mirror(content: str) -> str:
    """Replace dockerhub images with china mirror."""
    mirror_prefix = "swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io"
    lines = content.splitlines()
    new_lines = []
    
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("image:"):
            key, value = line.split(":", 1)
            image_name = value.strip().strip("'").strip('"')
            
            # Skip if already has a domain (contains dot or localhost)
            parts = image_name.split("/")
            if "." in parts[0] or "localhost" in parts[0]:
                new_lines.append(line)
                continue
                
            # Docker Hub image
            if "/" not in image_name:
                # Official library image
                new_image = f"{mirror_prefix}/library/{image_name}"
            else:
                # Namespaced image
                new_image = f"{mirror_prefix}/{image_name}"
            
            # Preserve original indentation and key
            prefix = line.split("image:")[0]
            new_lines.append(f"{prefix}image: {new_image}")
        else:
            new_lines.append(line)
            
    return "\n".join(new_lines) + "\n"


if __name__ == "__main__":
    main()