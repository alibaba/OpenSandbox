#!/usr/bin/env python3

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

import os
import re
import sys
import argparse
from pathlib import Path

# Configuration
PROJECT_ROOT = Path(__file__).resolve().parent.parent

# Component Configuration
COMPONENTS = {
    "code-interpreter": {
        "image": "opensandbox/code-interpreter",
        "version_file": PROJECT_ROOT / "sandboxes" / "code-interpreter" / "VERSION_TAG"
    },
    "execd": {
        "image": "opensandbox/execd",
        "version_file": PROJECT_ROOT / "components" / "execd" / "VERSION_TAG"
    },
    "ingress": {
        "image": "opensandbox/ingress",
        "version_file": PROJECT_ROOT / "components" / "ingress" / "VERSION_TAG"
    },
    "egress": {
        "image": "opensandbox/egress",
        "version_file": PROJECT_ROOT / "components" / "egress" / "VERSION_TAG"
    }
}

# Files/Directories to ignore during scan
IGNORE_DIRS = { ".git", ".idea", ".vscode", "__pycache__", "node_modules", "dist", "build", ".gemini"}
# Extensions to scan
INCLUDE_EXTS = { ".md", ".py", ".java", ".ts", ".js", ".kt", ".sh", ".yaml", ".yml", ".toml", ".properties"}

def get_pattern(image_name):
    # Regex to match image usage: [registry/][user/]repo:tag
    # Group 1: Optional registry/user prefix
    # Group 2: The specific version tag
    # We ignore ${TAG}, $TAG, :latest, :local, :dev, :test to avoid breaking build scripts and tests
    return re.compile(r'([a-zA-Z0-9.-]+(?::\d+)?/)?' + re.escape(image_name) + r":(?![\$\{]|latest\b|local\b|dev\b|test\b)([a-zA-Z0-9._-]+)")

def get_current_version(component_name):
    version_file = COMPONENTS[component_name]["version_file"]
    if not version_file.exists():
        print(f"Error: Version file not found at {version_file} for component {component_name}")
        sys.exit(1)
    return version_file.read_text().strip()

def should_process_file(path: Path):
    if path.name.startswith("."):
        return False
    if path.suffix not in INCLUDE_EXTS:
        return False
    # Specific exclusions can be added here
    if path.resolve() == Path(__file__).resolve():
        return False
    return True

def scan_files(root: Path):
    for root_dir, dirs, files in os.walk(root):
        # Modify dirs in-place to skip ignored directories
        dirs[:] = [d for d in dirs if d not in IGNORE_DIRS]
        
        for file in files:
            file_path = Path(root_dir) / file
            if should_process_file(file_path):
                yield file_path

def verify_component(component_name):
    target_version = get_current_version(component_name)
    image_name = COMPONENTS[component_name]["image"]
    pattern = get_pattern(image_name)
    
    print(f"Verifying {component_name} ({image_name}:{target_version})...")
    errors = []
    
    for file_path in scan_files(PROJECT_ROOT):
        try:
            content = file_path.read_text(encoding="utf-8", errors="ignore")
            for match in pattern.finditer(content):
                version = match.group(2)
                if version != target_version:
                    errors.append(f"{file_path.relative_to(PROJECT_ROOT)}: Found {version}, expected {target_version}")
        except Exception as e:
            print(f"Warning: Could not read {file_path}: {e}")
            
    return errors

def update_component(component_name):
    target_version = get_current_version(component_name)
    image_name = COMPONENTS[component_name]["image"]
    pattern = get_pattern(image_name)
    
    print(f"Updating {component_name} to {target_version}...")
    count = 0
    
    for file_path in scan_files(PROJECT_ROOT):
        try:
            content = file_path.read_text(encoding="utf-8", errors="ignore")
            
            def replace_func(match):
                prefix = match.group(1) or ""
                return f"{prefix}{image_name}:{target_version}"
            
            new_content, n = pattern.subn(replace_func, content)
            
            if n > 0:
                if new_content != content:
                    file_path.write_text(new_content, encoding="utf-8")
                    print(f"Updated {n} occurrence(s) in {file_path.relative_to(PROJECT_ROOT)}")
                    count += 1
        except Exception as e:
            print(f"Warning: Could not update {file_path}: {e}")
            
    print(f"✨ Updated {count} files for {component_name}.")

def main():
    parser = argparse.ArgumentParser(description="Manage OpenSandbox component versions in documentation.")
    parser.add_argument("action", choices=["verify", "update"], help="Action to perform")
    parser.add_argument("--component", choices=COMPONENTS.keys(), help="Specific component to process. If omitted, all are processed.")
    
    args = parser.parse_args()
    
    components_to_process = [args.component] if args.component else COMPONENTS.keys()
    
    if args.action == "verify":
        all_errors = []
        for comp in components_to_process:
            errors = verify_component(comp)
            all_errors.extend(errors)
            
        if all_errors:
            print("\n❌ Version mismatches found:")
            for error in all_errors:
                print(f"  - {error}")
            print("\nPlease run: python3 scripts/manage_sandbox_version.py update")
            sys.exit(1)
        else:
            print("\n✅ All versions match.")
            
    elif args.action == "update":
        for comp in components_to_process:
            update_component(comp)

if __name__ == "__main__":
    main()
