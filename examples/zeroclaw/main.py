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
import time
from datetime import timedelta

import requests
from opensandbox import SandboxSync
from opensandbox.config import ConnectionConfigSync


def check_zeroclaw(sbx: SandboxSync) -> bool:
    """
    Health check: poll zeroclaw gateway until it returns 200.

    Returns:
        True  when ready
        False on timeout or any exception
    """
    try:
        endpoint = sbx.get_endpoint(42617)
        start = time.perf_counter()
        url = f"http://{endpoint.endpoint}/health"
        for _ in range(150):  # max for ~30s
            try:
                resp = requests.get(url, timeout=1)
                if resp.status_code == 200:
                    elapsed = time.perf_counter() - start
                    print(f"[check] sandbox ready after {elapsed:.1f}s")
                    return True
            except Exception:
                pass
            time.sleep(0.2)
        return False
    except Exception as exc:
        print(f"[check] failed: {exc}")
        return False


def main() -> None:
    server = "http://localhost:8080"
    image = os.getenv("ZEROCLAW_SANDBOX_IMAGE", "opensandbox/zeroclaw:latest")
    timeout_seconds = 3600  # 1 hour

    print(f"Creating zeroclaw sandbox with image={image} on OpenSandbox server {server}...")
    sandbox = SandboxSync.create(
        image=image,
        timeout=timedelta(seconds=timeout_seconds),
        metadata={"example": "zeroclaw"},
        entrypoint=["zeroclaw", "gateway", "--host", "0.0.0.0", "--port", "42617"],
        connection_config=ConnectionConfigSync(domain=server),
        health_check=check_zeroclaw,
        env={"ZEROCLAW_ALLOW_PUBLIC_BIND": "true"},
    )

    endpoint = sandbox.get_endpoint(42617)
    print(f"Zeroclaw gateway started. Please refer to {endpoint.endpoint}")


if __name__ == "__main__":
    main()
