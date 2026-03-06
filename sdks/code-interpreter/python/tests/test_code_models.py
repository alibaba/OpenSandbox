#
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
#
"""
Tests for code interpreter models.

This module provides test coverage for code execution models including
language support, context creation, and validation scenarios.
"""
import pytest

from code_interpreter.models.code import (
    CodeContext,
    SupportedLanguage,
)


# ============================================================================
# SupportedLanguage Tests
# ============================================================================


def test_supported_language_constants() -> None:
    """Test that all supported languages have correct string values."""
    assert SupportedLanguage.PYTHON == "python"
    assert SupportedLanguage.JAVA == "java"
    assert SupportedLanguage.GO == "go"
    assert SupportedLanguage.TYPESCRIPT == "typescript"
    assert SupportedLanguage.BASH == "bash"
    assert SupportedLanguage.JAVASCRIPT == "javascript"


def test_supported_language_values_are_lowercase() -> None:
    """Test that language values are lowercase (consistent convention)."""
    languages = [
        SupportedLanguage.PYTHON,
        SupportedLanguage.JAVA,
        SupportedLanguage.GO,
        SupportedLanguage.TYPESCRIPT,
        SupportedLanguage.BASH,
        SupportedLanguage.JAVASCRIPT,
    ]
    for lang in languages:
        assert lang == lang.lower(), f"Language '{lang}' should be lowercase"


def test_supported_language_no_duplicates() -> None:
    """Test that all language values are unique."""
    languages = [
        SupportedLanguage.PYTHON,
        SupportedLanguage.JAVA,
        SupportedLanguage.GO,
        SupportedLanguage.TYPESCRIPT,
        SupportedLanguage.BASH,
        SupportedLanguage.JAVASCRIPT,
    ]
    assert len(languages) == len(set(languages)), "Language values should be unique"


# ============================================================================
# CodeContext Tests
# ============================================================================


def test_code_context_creation_with_id() -> None:
    """Test creating a context with explicit ID."""
    context = CodeContext(id="ctx-123", language=SupportedLanguage.PYTHON)
    assert context.id == "ctx-123"
    assert context.language == "python"


def test_code_context_creation_without_id() -> None:
    """Test creating a context without ID (auto-generated)."""
    context = CodeContext(language=SupportedLanguage.JAVA)
    assert context.id is None
    assert context.language == "java"


def test_code_context_all_supported_languages() -> None:
    """Test context creation with all supported languages."""
    languages = [
        SupportedLanguage.PYTHON,
        SupportedLanguage.JAVA,
        SupportedLanguage.GO,
        SupportedLanguage.TYPESCRIPT,
        SupportedLanguage.BASH,
        SupportedLanguage.JAVASCRIPT,
    ]
    for lang in languages:
        context = CodeContext(language=lang)
        assert context.language == lang


def test_code_context_empty_language_rejected() -> None:
    """Test that empty string language is rejected."""
    with pytest.raises(ValueError, match="blank"):
        CodeContext(language="")


def test_code_context_whitespace_language_rejected() -> None:
    """Test that whitespace-only language is rejected."""
    with pytest.raises(ValueError, match="blank"):
        CodeContext(language="   ")


def test_code_context_whitespace_variants_rejected() -> None:
    """Test various whitespace-only language values are rejected."""
    invalid_languages = [
        "   ",
        "\t",
        "\n",
        "\t\n  ",
        "  \t  ",
    ]
    for invalid_lang in invalid_languages:
        with pytest.raises(ValueError, match="blank"):
            CodeContext(language=invalid_lang)


def test_code_context_case_sensitive() -> None:
    """Test that language validation is case-sensitive."""
    # These should be valid (just strings, not checking against supported list)
    context_upper = CodeContext(language="PYTHON")
    assert context_upper.language == "PYTHON"
    
    context_mixed = CodeContext(language="Python")
    assert context_mixed.language == "Python"


def test_code_context_arbitrary_language() -> None:
    """Test that arbitrary language strings are accepted (for extensibility)."""
    # Model allows any non-empty string for forward compatibility
    context = CodeContext(language="rust")
    assert context.language == "rust"


def test_code_context_serialization() -> None:
    """Test that context serializes correctly."""
    context = CodeContext(id="test-id", language="python")
    dumped = context.model_dump()
    assert dumped["id"] == "test-id"
    assert dumped["language"] == "python"


def test_code_context_with_none_id_serialization() -> None:
    """Test serialization with None ID."""
    context = CodeContext(language="go")
    dumped = context.model_dump()
    assert dumped["id"] is None
    assert dumped["language"] == "go"


def test_code_context_unicode_language() -> None:
    """Test that unicode strings are handled correctly."""
    # While not practical, the model should handle unicode
    context = CodeContext(language="日本語")
    assert context.language == "日本語"


# ============================================================================
# Integration Tests
# ============================================================================


def test_code_context_multiple_contexts_different_languages() -> None:
    """Test creating multiple contexts with different languages."""
    python_ctx = CodeContext(id="py-1", language=SupportedLanguage.PYTHON)
    java_ctx = CodeContext(id="java-1", language=SupportedLanguage.JAVA)
    go_ctx = CodeContext(id="go-1", language=SupportedLanguage.GO)
    
    assert python_ctx.language != java_ctx.language
    assert java_ctx.language != go_ctx.language
    assert python_ctx.id != java_ctx.id != go_ctx.id


def test_code_context_model_config() -> None:
    """Test that model config allows arbitrary types."""
    from pydantic import ConfigDict
    assert CodeContext.model_config.get("arbitrary_types_allowed") is True
