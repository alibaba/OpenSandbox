# Copyright 2025 Alibaba Group Holding Ltd.
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

"""
FastAPI application entry point for OpenSandbox Lifecycle API.

This module initializes the FastAPI application with middleware, routes,
and configuration for the sandbox lifecycle management service.
"""

import copy
import logging.config
from contextlib import asynccontextmanager
from typing import Any

import httpx
from fastapi import FastAPI, Request
from fastapi.exceptions import HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from src.config import load_config
from uvicorn.config import LOGGING_CONFIG as UVICORN_LOGGING_CONFIG

# Load configuration before initializing routers/middleware
app_config = load_config()

# Unify logging format (including uvicorn access/error logs) with timestamp prefix.
_log_config = copy.deepcopy(UVICORN_LOGGING_CONFIG)
_fmt = "%(levelprefix)s %(asctime)s %(name)s: %(message)s"
_datefmt = "%Y-%m-%d %H:%M:%S%z"

# Enable colors and set format for both default and access loggers
_log_config["formatters"]["default"]["fmt"] = _fmt
_log_config["formatters"]["default"]["datefmt"] = _datefmt
_log_config["formatters"]["default"]["use_colors"] = True

_log_config["formatters"]["access"]["fmt"] = _fmt
_log_config["formatters"]["access"]["datefmt"] = _datefmt
_log_config["formatters"]["access"]["use_colors"] = True

# Ensure project loggers (src.*) emit at configured level using the default handler.
_log_config["loggers"]["src"] = {
    "handlers": ["default"],
    "level": app_config.server.log_level.upper(),
    "propagate": False,
}

logging.config.dictConfig(_log_config)
logging.getLogger().setLevel(
    getattr(logging, app_config.server.log_level.upper(), logging.INFO)
)

from src.api.lifecycle import router  # noqa: E402
from src.middleware.auth import AuthMiddleware  # noqa: E402


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.http_client = httpx.AsyncClient(timeout=180.0)
    yield
    await app.state.http_client.aclose()


# Initialize FastAPI application
app = FastAPI(
    title="OpenSandbox Lifecycle API",
    version="0.1.0",
    description="The Sandbox Lifecycle API coordinates how untrusted workloads are created, "
                "executed, paused, resumed, and finally disposed.",
    docs_url="/docs",
    redoc_url="/redoc",
    lifespan=lifespan,
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # 当前环境默认放开所有来源，生产部署需按配置收敛
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Attach global config for runtime access
app.state.config = app_config

# Add authentication middleware
app.add_middleware(AuthMiddleware, config=app_config)

# Include API routes at root and versioned prefix
app.include_router(router)
app.include_router(router, prefix="/v1")

DEFAULT_ERROR_CODE = "GENERAL::UNKNOWN_ERROR"
DEFAULT_ERROR_MESSAGE = "An unexpected error occurred."


def _normalize_error_detail(detail: Any) -> dict[str, str]:
    """
    Ensure HTTP errors always conform to {"code": "...", "message": "..."}.
    """
    if isinstance(detail, dict):
        code = detail.get("code") or DEFAULT_ERROR_CODE
        message = detail.get("message") or DEFAULT_ERROR_MESSAGE
        return {"code": code, "message": message}
    message = str(detail) if detail else DEFAULT_ERROR_MESSAGE
    return {"code": DEFAULT_ERROR_CODE, "message": message}


@app.exception_handler(HTTPException)
async def sandbox_http_exception_handler(request: Request, exc: HTTPException):
    """
    Flatten FastAPI HTTPException payload to the standard error schema.
    """
    content = _normalize_error_detail(exc.detail)
    return JSONResponse(
        status_code=exc.status_code,
        content=content,
        headers=exc.headers,
    )


@app.get("/health")
async def health_check():
    """
    Health check endpoint.

    Returns:
        dict: Health status
    """
    return {"status": "healthy"}


if __name__ == "__main__":
    import uvicorn

    # Run the application
    uvicorn.run(
        "src.main:app",
        host=app_config.server.host,
        port=app_config.server.port,
        reload=True,
        log_config=_log_config,
    )
