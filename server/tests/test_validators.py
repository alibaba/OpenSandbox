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

from src.services.validators import ensure_metadata_labels


def test_ensure_metadata_labels_accepts_common_k8s_forms():
    # Various valid label shapes: with/without prefix, mixed chars, empty value allowed.
    valid_metadata = {
        "app": "web",
        "opensandbox.dev/hello": "world",
        "k8s.io/name": "app-1",
        "example.com/label": "a.b_c-1",
        "team": "A1_b-2.c",
        "empty": "",
    }

    # Should not raise
    ensure_metadata_labels(valid_metadata)


def test_ensure_metadata_labels_allows_none_or_empty():
    ensure_metadata_labels(None)
    ensure_metadata_labels({})
