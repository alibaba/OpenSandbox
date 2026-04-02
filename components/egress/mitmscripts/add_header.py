# Example mitmproxy addon: add a static header to every request.
# Use: OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT=/opt/opensandbox/mitmscripts/add_header.py
from mitmproxy import http

HEADER_NAME = "X-OpenSandbox-Egress"
HEADER_VALUE = "1"


def request(flow: http.HTTPFlow) -> None:
    if flow.request:
        flow.request.headers[HEADER_NAME] = HEADER_VALUE
