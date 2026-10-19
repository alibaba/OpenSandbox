# Copyright 2026 The OpenSandbox Authors
# SPDX-License-Identifier: Apache-2.0

"""Opt-in P2 production-addon loopback acceptance, NOT Linux transparent proof.

Import/discovery does no I/O and registers no passing/skipped unittest cases.
Run explicitly in an authorized socket-capable environment:

  python test_credential_bound_h2_runtime.py --run-real-runtime --artifacts NEW_DIR

Without --run-real-runtime this exits 2 with NOT_RUN. --list is static. The
origin MUST really bind 127.0.0.1:443; occupied/denied 443 is BLOCKED, never a
high-port fallback. The client uses actual SOCKS5 CONNECT to that address,
then verified TLS SNI bound.example.com and H2. The real system.py, receiver,
registry and adapter are untouched. Test observation reads metadata only.

Exit 0 means every selected case passed; 1 is assertion/protocol failure;
2 is blocked or not run. Inspect report.json's full denominator: unselected
cases and broader matrix items stay NOT_RUN. This does not exercise Linux
redirects, the restricted production user, the Go owner, or quarantine.
"""

from __future__ import annotations

import argparse
import hashlib
import http.client
import http.server
import importlib.metadata
import json
import os
import platform
import shutil
import socket
import socketserver
import ssl
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[2]
MITMSCRIPTS = HERE.parent / "mitmscripts"
OBSERVER = HERE / "fixtures/h2_production_observer.py"
PINS = {"mitmproxy": "11.0.2", "h2": "4.1.0", "hyperframe": "6.0.1"}
BOUND = "bound.example.com"
SECRET_ONE = "runtime-live-secret-one"
SECRET_TWO = "runtime-rotated-secret-two"
BODY = (b"synthetic-production-h2-body-" * 6000)[:131089]
BODY_SHA256 = hashlib.sha256(BODY).hexdigest()
SMALL_BODY = b"synthetic-production-h2-ok"
CASES = [f"{name}_{protocol}" for name in (
    "attachment", "removal_drain", "rotation", "remove_readd", "reset_isolation", "disconnect"
) for protocol in ("h1", "h2")] + ["expiry_short_h2", "expiry_default_30s_h2", "d3_pending_writer"]
GO_CASES = ("go_retry", "go_retry_bounded_old")
GO_SCOPE = {
    "go_retry": "Old response held until new_result; success requires migration while the old response remains unfinished.",
    "go_retry_bounded_old": "Old response released after the new GET reaches the post-ACK old-connection fence, before its 8-second retry budget ends. Success would not prove migration while old never finishes or when a finite old response outlasts that budget.",
}
MISSING = [
    "Linux transparent redirects and actual original-destination lookup",
    "Restricted mitmproxy user, IPv6 and production CA provisioning",
    "Go generation owner, readiness, recovery and kernel quarantine readback",
    "Fragmented HEADERS, same-batch DATA/RST, HPACK and flow-control variants",
    "Pre-ACK fence timing / commit-before-admission race and path-only/policy-only rotations",
    "Upstream concurrency queue, reordered IDs, early-response upload/reset variants",
    "Capacity exhaustion, invalid authority, stranded trailers and fatal parser variants",
    "Non-idempotent requests and other client libraries or versions",
]


class Blocked(RuntimeError):
    """An unavailable prerequisite; never a successful skip."""


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def wait_for(predicate, timeout=10, message="condition did not become true"):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(0.02)
    raise AssertionError(message)


def read_events(path):
    if not path.exists():
        return []
    result = []
    for line in path.read_text().splitlines():
        try:
            result.append(json.loads(line))
        except json.JSONDecodeError:
            # The child may currently be writing the final line.
            break
    return result


def exact_read(stream, count):
    output = b""
    while len(output) < count:
        chunk = stream.recv(count - len(output))
        if not chunk:
            raise AssertionError("SOCKS5 connection ended during handshake")
        output += chunk
    return output


class H2Client:
    """One verified SOCKS5/TLS connection, no retries and explicit flow credit."""

    def __init__(self, proxy_port, ca_file):
        import h2.config
        import h2.connection
        import h2.settings

        raw = socket.create_connection(("127.0.0.1", proxy_port), timeout=5)
        self.closed = False
        self.eof = False
        self.terminated = False
        self.frames = []
        self.pending_frames = bytearray()
        self.bodies = {}
        self.headers = {}
        self.sent_requests = []
        self.ended = set()
        self.resets = {}
        self.locally_reset = set()
        self.auto_credit = False
        self.uncredited = {}
        try:
            raw.sendall(b"\x05\x01\x00")
            require(exact_read(raw, 2) == b"\x05\x00", "SOCKS5 no-auth negotiation failed")
            # IPv4 CONNECT 127.0.0.1:443. No proxy authority remapping.
            raw.sendall(b"\x05\x01\x00\x01\x7f\x00\x00\x01\x01\xbb")
            head = exact_read(raw, 4)
            require(head[:3] == b"\x05\x00\x00", "SOCKS5 CONNECT failed")
            address_bytes = {1: 4, 4: 16}.get(head[3])
            if head[3] == 3:
                address_bytes = exact_read(raw, 1)[0]
            require(address_bytes is not None, "SOCKS5 returned an invalid address type")
            exact_read(raw, address_bytes + 2)
            # Do not use create_default_context: it implicitly honors ambient
            # SSLKEYLOGFILE. This verified test client must never produce keys.
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
            context.load_verify_locations(cafile=str(ca_file))
            context.set_alpn_protocols(["h2"])
            self.tls = context.wrap_socket(raw, server_hostname=BOUND)
            require(self.tls.selected_alpn_protocol() == "h2", "client did not negotiate H2")
            self.tls.settimeout(0.1)
            self.h2 = h2.connection.H2Connection(h2.config.H2Configuration(client_side=True))
            self.h2.initiate_connection()
            self.h2.update_settings({h2.settings.SettingCodes.INITIAL_WINDOW_SIZE: 1024})
            self.flush()
        except BaseException:
            raw.close()
            raise

    def flush(self):
        data = self.h2.data_to_send()
        if data:
            self.tls.sendall(data)

    def request(self, stream_id, path="/v1/old"):
        self.sent_requests.append({"stream_id": stream_id, "path": path, "logical_id": f"manual-{stream_id}"})
        self.h2.send_headers(stream_id, [
            (":method", "GET"), (":scheme", "https"),
            (":authority", BOUND), (":path", path),
            ("x-h2-logical-id", f"manual-{stream_id}"),
        ], end_stream=True)
        self.flush()

    def reset(self, stream_id):
        self.h2.reset_stream(stream_id)
        self.locally_reset.add(stream_id)
        self.flush()

    def credit(self):
        # Grant ordinary H2 credit through its public API, never mutate buffers.
        self.auto_credit = True
        for sid, amount in tuple(self.uncredited.items()):
            self.h2.acknowledge_received_data(amount, sid)
        self.uncredited.clear()
        for sid in self.h2.streams:
            if sid not in self.ended and sid not in self.resets and sid not in self.locally_reset:
                self.h2.increment_flow_control_window(1 << 20, stream_id=sid)
        self.h2.increment_flow_control_window(1 << 20)
        self.flush()

    def _frames(self, data):
        from hyperframe.frame import Frame, GoAwayFrame, RstStreamFrame
        self.pending_frames.extend(data)
        while len(self.pending_frames) >= 9:
            frame, length = Frame.parse_frame_header(memoryview(bytes(self.pending_frames[:9])))
            if len(self.pending_frames) < length + 9:
                return
            frame.parse_body(memoryview(bytes(self.pending_frames[9:9 + length])))
            del self.pending_frames[:9 + length]
            item = {"sequence": len(self.frames) + 1, "type": type(frame).__name__,
                    "stream_id": frame.stream_id, "length": length,
                    "end_stream": "END_STREAM" in frame.flags,
                    "monotonic_ns": time.monotonic_ns()}
            if isinstance(frame, (GoAwayFrame, RstStreamFrame)):
                item["error_code"] = int(frame.error_code)
            if isinstance(frame, GoAwayFrame):
                item["last_stream_id"] = frame.last_stream_id
            self.frames.append(item)

    def poll(self):
        import h2.events
        if self.eof:
            return
        try:
            data = self.tls.recv(65536)
        except TimeoutError:
            return
        except (ssl.SSLEOFError, ConnectionResetError):
            data = b""
        if not data:
            self.eof = True
            require(not self.pending_frames, "EOF split a received H2 frame")
            return
        self._frames(data)
        events = self.h2.receive_data(data)
        self.terminated = self.terminated or any(isinstance(event, h2.events.ConnectionTerminated) for event in events)
        for event in events:
            if isinstance(event, h2.events.DataReceived):
                self.bodies.setdefault(event.stream_id, bytearray()).extend(event.data)
                if self.auto_credit and not self.terminated:
                    self.h2.acknowledge_received_data(event.flow_controlled_length, event.stream_id)
                else:
                    self.uncredited[event.stream_id] = self.uncredited.get(event.stream_id, 0) + event.flow_controlled_length
            elif isinstance(event, h2.events.ResponseReceived):
                self.headers[event.stream_id] = dict(event.headers)
            elif isinstance(event, h2.events.StreamEnded):
                self.ended.add(event.stream_id)
            elif isinstance(event, h2.events.StreamReset):
                self.resets[event.stream_id] = int(event.error_code)
        if not self.terminated:
            self.flush()

    def until(self, predicate, timeout=10, message="client condition timed out"):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if predicate():
                return
            require(not self.eof, message + " (EOF)")
            self.poll()
        require(predicate(), message)

    def close(self):
        if not self.closed:
            self.closed = True
            self.tls.close()

    def evidence(self):
        return {"eof": self.eof, "frames": self.frames, "requests": self.sent_requests,
                "ended": sorted(self.ended), "received_resets": self.resets,
                "sent_resets": sorted(self.locally_reset),
                "bodies": {str(sid): {"bytes": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                           for sid, body in self.bodies.items()}}


class OriginServer(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = False
    block_on_close = True

    def __init__(self, root, protocol, scenario):
        self.context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        self.context.load_cert_chain(str(root / "origin.pem"))
        self.context.set_alpn_protocols(["h2"] if protocol == "h2" else ["http/1.1"])
        self.protocol = protocol
        self.scenario = scenario
        self.records = []
        self.terminals = []
        self.next_connection_id = 0
        self.timeout_count = 0
        self.sockets = set()
        self.lock = threading.Lock()
        self.release = threading.Event()
        self.stopping = threading.Event()
        try:
            super().__init__(("127.0.0.1", 443), OriginHandler)
        except OSError as error:
            raise Blocked(f"real loopback origin 127.0.0.1:443 unavailable (errno={error.errno}); no fallback") from None
        self.thread = threading.Thread(target=self.serve_forever, name="h2-p2-origin")
        self.thread.start()

    def record(self, headers, protocol, stream_id=None):
        path = headers.get(":path", headers.get("path", ""))
        token = headers.get("private-token", "")
        classification = {SECRET_ONE: "one", SECRET_TWO: "two", "": "missing"}.get(token, "unexpected")
        with self.lock:
            self.records.append({"sequence": len(self.records) + 1, "path": path,
                                 "protocol": protocol, "credential": classification,
                                 "upstream_stream_id": stream_id,
                                 "logical_id": headers.get("x-h2-logical-id", ""),
                                 "monotonic_ns": time.monotonic_ns()})
        return path, token

    def response(self, path, token):
        body = BODY if path in ("/v1/old", "/v1/old-two") else SMALL_BODY
        return body, [("content-type", "application/octet-stream"),
                      ("content-length", str(len(body))), ("x-credential-echo", token)]

    def finish(self):
        self.stopping.set()
        self.release.set()
        self.shutdown()
        with self.lock:
            streams = list(self.sockets)
        for stream in streams:
            try:
                stream.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            stream.close()
        self.server_close()
        self.thread.join(timeout=5)
        require(not self.thread.is_alive(), "origin accept thread did not terminate")
        with self.lock:
            require(not self.sockets, "origin accepted sockets did not terminate")


class H1Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass

    def log_error(self, _format, *args):
        if any(isinstance(value, TimeoutError) for value in args):
            with self.server.lock:
                self.server.timeout_count += 1

    def do_GET(self):
        headers = {name.lower(): value for name, value in self.headers.items()}
        headers["path"] = self.path
        path, token = self.server.record(headers, "http/1.1")
        body, fields = self.server.response(path, token)
        self.send_response(200)
        if self.server.scenario == "go_retry" and path == "/v1/old":
            self.send_header("content-type", "text/event-stream")
            self.send_header("transfer-encoding", "chunked")
            self.end_headers()
            first = b"data: synthetic-old-start\n\n"
            self.wfile.write(f"{len(first):x}\r\n".encode() + first + b"\r\n")
            self.wfile.flush()
            if not self.server.release.wait(30):
                return
            self.wfile.write(b"0\r\n\r\n")
        else:
            for name, value in fields:
                self.send_header(name, value)
            self.end_headers()
            self.wfile.write(body)
        self.wfile.flush()


class OriginHandler(socketserver.BaseRequestHandler):
    def handle(self):
        raw = self.request
        wrapped = None
        with self.server.lock:
            self.server.sockets.add(raw)
            self.server.next_connection_id += 1
            connection_id = self.server.next_connection_id
        try:
            raw.settimeout(10)
            wrapped = self.server.context.wrap_socket(raw, server_side=True, do_handshake_on_connect=False)
            with self.server.lock:
                self.server.sockets.discard(raw)
                self.server.sockets.add(wrapped)
            wrapped.do_handshake()
            if wrapped.selected_alpn_protocol() == "h2":
                self.serve_h2(wrapped)
            else:
                # Longer than the entire normal-close observation window. A
                # fixture idle timeout must never masquerade as proxy closure.
                wrapped.settimeout(120)
                H1Handler(wrapped, self.client_address, self.server)
        except TimeoutError:
            with self.server.lock:
                self.server.timeout_count += 1
        except (OSError, ssl.SSLError):
            pass
        finally:
            if wrapped is not None:
                wrapped.close()
            raw.close()
            with self.server.lock:
                self.server.sockets.discard(raw)
                self.server.sockets.discard(wrapped)
                self.server.terminals.append({"connection_id": connection_id,
                                              "before_forced_teardown": not self.server.stopping.is_set(),
                                              "monotonic_ns": time.monotonic_ns()})

    def serve_h2(self, stream):
        import h2.config
        import h2.connection
        import h2.events
        import h2.exceptions
        conn = h2.connection.H2Connection(h2.config.H2Configuration(client_side=False, header_encoding="utf-8"))
        conn.initiate_connection()
        stream.sendall(conn.data_to_send())
        stream.settimeout(0.1)
        pending = {}
        while not self.server.stopping.is_set():
            try:
                data = stream.recv(65536)
            except TimeoutError:
                data = None
            if data == b"":
                return
            if data:
                for event in conn.receive_data(data):
                    if isinstance(event, h2.events.RequestReceived):
                        path, token = self.server.record(dict(event.headers), "h2", event.stream_id)
                        body, fields = self.server.response(path, token)
                        conn.send_headers(event.stream_id, [(":status", "200"), *fields])
                        pending[event.stream_id] = [body, 0]
                    elif isinstance(event, h2.events.DataReceived):
                        conn.acknowledge_received_data(event.flow_controlled_length, event.stream_id)
                    elif isinstance(event, h2.events.StreamReset):
                        pending.pop(event.stream_id, None)
                    elif isinstance(event, h2.events.ConnectionTerminated):
                        return
            for sid, pair in list(pending.items()):
                body, offset = pair
                try:
                    while offset < len(body):
                        size = min(conn.local_flow_control_window(sid), conn.max_outbound_frame_size, len(body) - offset)
                        if size <= 0:
                            break
                        conn.send_data(sid, body[offset:offset + size], end_stream=offset + size == len(body))
                        offset += size
                    pair[1] = offset
                    if offset == len(body):
                        pending.pop(sid)
                except h2.exceptions.StreamClosedError:
                    pending.pop(sid, None)
            outbound = conn.data_to_send()
            if outbound:
                stream.sendall(outbound)


class RuntimeHarness:
    """Composition reuses IPC/certificate helpers, never the remapping startup."""

    def __init__(self, root, protocol="h2", scenario="normal", drain_seconds=8):
        from test_credential_bound_runtime import (
            LiveRuntimeHarness,
            generate_origin_certificate,
        )
        self.root = root
        self.root.mkdir()
        self.clients = []
        self.process = None
        self.origin = None
        self.log = None
        self.ipc_events = []
        self.cleanup = {}
        self.drain_seconds = drain_seconds
        self.expected_epoch = None
        self.final_ack_ns = 0
        self.ipc = object.__new__(LiveRuntimeHarness)
        self.ipc.root = root
        # AF_UNIX path is kept under /tmp because macOS sockaddr_un is short.
        self.socket_directory = tempfile.TemporaryDirectory(prefix="h2p2-", dir="/tmp")
        os.chmod(self.socket_directory.name, 0o700)
        self.ipc.socket_path = str(Path(self.socket_directory.name) / "receiver.sock")
        self.socket_path = self.ipc.socket_path
        self.events_path = root / "proxy-events.jsonl"
        generate_origin_certificate(root)
        self.protocol = protocol
        self.scenario = scenario

    def start(self):
        self.origin = OriginServer(self.root, self.protocol, self.scenario)
        with socket.socket() as port_socket:
            port_socket.bind(("127.0.0.1", 0))
            self.proxy_port = port_socket.getsockname()[1]
        self.log = (self.root / "proxy.log").open("w")
        environment = self.ipc.env(self.drain_seconds)
        # A caller's unrelated mitmproxy/proxy configuration must not select a
        # different origin, chained proxy, legacy credential pull or TLS keys.
        # ipc.env() supplies the controlled empty static passthrough handoff.
        environment = {key: value for key, value in environment.items()
                       if key not in ("SSLKEYLOGFILE", "MITMPROXY_SSLKEYLOGFILE")
                       and not key.startswith("H2_PRODUCTION_")
                       and (not key.startswith("OPENSANDBOX_")
                            or key.startswith("OPENSANDBOX_EGRESS_REVISION_")
                            or key == "OPENSANDBOX_EGRESS_MITMPROXY_PASSTHROUGH_HOSTS")}
        environment.update(H2_PRODUCTION_OBSERVATIONS=str(self.root),
                           H2_PRODUCTION_SCENARIO=self.scenario)
        executable = str(Path(sys.executable).parent / "mitmdump")
        args = [executable, "--mode", "socks5", "--listen-host", "127.0.0.1",
                "--listen-port", str(self.proxy_port),
                "--set", "confdir=" + str(self.root / "ca"),
                "--set", "connection_strategy=lazy", "--set", "flow_detail=0",
                "--set", "ssl_verify_upstream_trusted_ca=" + str(self.root / "origin-ca.pem"),
                "-s", str(MITMSCRIPTS / "system.py"), "-s", str(OBSERVER)]
        self.process = subprocess.Popen(args, env=environment, stdout=self.log, stderr=subprocess.STDOUT)
        self.child_pid = self.process.pid
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            require(self.process.poll() is None, "mitmdump exited before readiness; inspect synthetic proxy.log")
            if Path(self.socket_path).exists() and (self.root / "ca/mitmproxy-ca-cert.pem").exists():
                try:
                    if self.ipc.active() is None and any(e["kind"] == "observer_ready" for e in self.events()):
                        self.combined_ca = self.root / "combined-ca.pem"
                        self.combined_ca.write_bytes((self.root / "ca/mitmproxy-ca-cert.pem").read_bytes()
                                                    + (self.root / "origin-ca.pem").read_bytes())
                        return
                except (OSError, http.client.HTTPException):
                    pass
            time.sleep(0.05)
        raise AssertionError("mitmdump did not become IPC/observer ready")

    def events(self):
        return read_events(self.events_path)

    def state(self):
        states = [event for event in self.events() if event["kind"] == "state"]
        return states[-1] if states else {}

    def connection(self):
        entries = self.state().get("connections", [])
        require(len(entries) == 1, "expected exactly one observed live H2 adapter")
        return entries[0]

    def wait_connection(self, predicate, timeout=10):
        def check():
            state = self.state()
            entries = state.get("connections", [])
            return entries[0] if (state.get("monotonic_ns", 0) >= self.final_ack_ns
                                  and len(entries) == 1
                                  and entries[0].get("publication_epoch") == self.expected_epoch
                                  and predicate(entries[0])) else None
        return wait_for(check, timeout, "production H2 state did not satisfy the case")

    def install(self, epoch, secret=SECRET_ONE, *, retry=False):
        from test_credential_bound_runtime import binding
        started = time.monotonic_ns()
        envelope = self.ipc.install(epoch, [] if secret is None else [binding(secret)],
                                    [] if secret is None else [secret], epoch)
        acknowledged = time.monotonic_ns()
        self.expected_epoch = epoch
        require(self.ipc.active() == envelope, "authenticated commit/readback envelope mismatch")
        if retry:
            status, value = self.ipc.command("/v1/revisions/commit", {"revision": envelope})
            require(status == 200 and value["revision"] == envelope, "idempotent authenticated commit failed")
        self.final_ack_ns = time.monotonic_ns()
        self.ipc_events.append({"epoch": epoch, "operation": "authenticated_prepare_commit_readback",
                                "started_ns": started, "ack_ns": acknowledged,
                                "final_ack_observed_ns": self.final_ack_ns,
                                "active_matches_ack": True, "commit_retried": retry})
        return envelope

    def verify_authentication(self):
        connection = http.client.HTTPConnection("localhost", timeout=5)
        connection.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        try:
            connection.sock.settimeout(5)
            connection.sock.connect(self.socket_path)
            connection.request("GET", "/v1/revisions/active", headers={"Authorization": "Bearer incorrect-synthetic-token"})
            response = connection.getresponse()
            response.read()
            require(response.status == 401, "unauthenticated revision IPC was not rejected")
        finally:
            connection.close()
        self.ipc_events.append({"operation": "invalid_token_rejected", "status": 401})

    def connect(self):
        client = H2Client(self.proxy_port, self.root / "ca/mitmproxy-ca-cert.pem")
        self.clients.append(client)
        return client

    def backpressured(self, client, sid=1, path="/v1/old"):
        client.request(sid, path)
        client.until(lambda: sid in client.headers and len(client.bodies.get(sid, b"")) > 0,
                     message="old response did not reach a real flow-control stall")
        record = self.wait_connection(lambda row: any(
            active["stream_id"] == sid and active["application_done"] and active["response_done"]
            for active in row["active"]))
        require(sid not in client.ended and len(client.bodies[sid]) < len(BODY),
                "response completed instead of remaining backpressured after response hook")
        return record

    def expect_origin_closed(self):
        def closed():
            with self.origin.lock:
                return not self.origin.sockets
        wait_for(closed, timeout=10, message="actual upstream origin sockets did not close before forced teardown")
        require(self.origin.timeout_count == 0, "origin timeout cannot prove proxy transport termination")
        self.cleanup["origin_naturally_closed_before_teardown"] = True
        self.cleanup["origin_timeout_count"] = self.origin.timeout_count

    def expect_clean_runtime(self):
        wait_for(lambda: (state := self.state()) and
                        state.get("registry_connections") == 0 and state.get("registry_requests") == 0 and
                        state.get("runtime_connections") == 0 and state.get("adapters") == 0 and
                        state.get("upstream_tasks") == 0,
                        timeout=10, message="production registry/adapter/upstream cleanup did not converge")
        self.expect_origin_closed()

    def finish(self):
        errors = []
        for client in self.clients:
            client.close()
        if self.process is not None:
            before_stop = self.process.poll()
            if before_stop is None:
                self.process.terminate()
            try:
                code = self.process.wait(timeout=8)
            except subprocess.TimeoutExpired:
                self.process.kill()
                code = self.process.wait(timeout=5)
                errors.append("mitmdump needed forced cleanup")
            self.cleanup.update(child_pid=self.child_pid, child_reaped=True,
                                parent_survived=True, exit_code=code,
                                exit_before_cleanup=before_stop)
        if self.origin is not None:
            try:
                self.origin.finish()
                self.cleanup["origin_sockets_closed"] = True
                self.cleanup["origin_listener_closed"] = self.origin.socket.fileno() == -1
                require(self.cleanup["origin_listener_closed"], "origin listener survived cleanup")
            except Exception as error:  # noqa: BLE001 - preserve sanitized failure and cleanup evidence
                errors.append(str(error))
        if self.log is not None:
            self.log.close()
        stale = Path(self.socket_path).exists()
        # D3 deliberately bypasses shutdown hooks. Remove only this harness's
        # owned stale socket after exact-child wait, never a shared IPC path.
        if stale:
            Path(self.socket_path).unlink()
        self.socket_directory.cleanup()
        self.cleanup.update(ipc_stale_after_exit=stale, owned_ipc_removed=not Path(self.socket_path).exists())
        if self.process is not None:
            with socket.socket() as probe:
                probe.settimeout(0.2)
                self.cleanup["proxy_listener_closed"] = probe.connect_ex(("127.0.0.1", self.proxy_port)) != 0
            require(self.cleanup["proxy_listener_closed"], "proxy listener survived exact-child reap")
        # Synthetic private keys are transient fixture state, not evidence.
        (self.root / "origin.pem").unlink(missing_ok=True)
        shutil.rmtree(self.root / "ca", ignore_errors=True)
        require(not errors, "; ".join(errors))

    def evidence(self):
        records = [] if self.origin is None else self.origin.records
        counts = {}
        for record in records:
            key = record["logical_id"] or record["path"]
            counts[key] = counts.get(key, 0) + 1
        return {"topology": "SOCKS5 CONNECT 127.0.0.1:443 / TLS bound.example.com / H2",
                "upstream_protocol": self.protocol, "drain_seconds": self.drain_seconds,
                "ipc": self.ipc_events, "origin": records, "origin_attempt_counts": counts,
                "origin_transport_terminals": [] if self.origin is None else self.origin.terminals,
                "clients": [client.evidence() for client in self.clients], "cleanup": self.cleanup}


def assert_attached(row):
    require(row["adapter"] == "BoundH2Server" and row["client_h2"], "actual production H2 adapter missing")
    require(row["destination_port"] == 443, "actual destination was not canonical 443")
    chain = row["layer_chain"]
    require("ClientTLSLayer" in chain and "NextLayer" in chain and chain[-1] == "HttpLayer",
            "actual TLS/NextLayer/HttpLayer dynamic ancestry not observed")


def assert_credential(harness, path, classification, count=1):
    records = [record for record in harness.origin.records if record["path"] == path]
    require(len(records) == count, "unexpected origin handling count for synthetic path")
    require(all(record["credential"] == classification for record in records), "origin injection classification mismatch")
    expected = "h2" if harness.protocol == "h2" else "http/1.1"
    require(all(record["protocol"] == expected for record in records), "upstream protocol matrix cell was not genuinely negotiated")


def assert_redacted(client, sid):
    require(client.headers[sid].get(b"x-credential-echo") == b"[REDACTED]", "response header did not use pinned redaction")


def assert_no_goaway(client):
    require(not any(frame["type"] == "GoAwayFrame" for frame in client.frames), "GOAWAY arrived before old response completion")


def assert_graceful(client, stream_ids, frozen):
    client.credit()
    client.until(lambda: client.eof, timeout=10, message="drained connection did not reach EOF")
    goaways = [frame for frame in client.frames if frame["type"] == "GoAwayFrame"]
    require(len(goaways) == 1, "normal drain did not deliver exactly one GOAWAY")
    goaway = goaways[0]
    require(goaway["error_code"] == 0 and goaway["last_stream_id"] == frozen, "GOAWAY did not retain frozen client stream bound")
    for sid in stream_ids:
        require(bytes(client.bodies[sid]) == BODY and sid in client.ended, "old response body/END_STREAM was incomplete")
        endings = [frame for frame in client.frames if frame["stream_id"] == sid and frame["end_stream"]]
        require(len(endings) == 1 and endings[0]["sequence"] < goaway["sequence"], "old END_STREAM did not precede unique GOAWAY")


def assert_refused(client, sid):
    client.until(lambda: sid in client.resets, message="post-ACK valid new stream was not reset")
    require(client.resets[sid] == 7, "post-ACK new stream was not REFUSED_STREAM")


def case_attachment(harness):
    harness.verify_authentication()
    harness.install(1, retry=True)
    client = harness.connect()
    row = harness.backpressured(client)
    assert_attached(row)
    assert_redacted(client, 1)
    assert_credential(harness, "/v1/old", "one")
    client.credit()
    client.until(lambda: 1 in client.ended)
    require(bytes(client.bodies[1]) == BODY, "attachment body bytes did not match")
    harness.wait_connection(lambda state: not state["active"])
    client.close()
    harness.expect_clean_runtime()


def case_removal_drain(harness):
    harness.install(1)
    client = harness.connect()
    assert_attached(harness.backpressured(client))
    harness.backpressured(client, 3, "/v1/old-two")
    harness.install(2, None, retry=True)
    row = harness.wait_connection(lambda state: state["fenced"])
    require(row["frozen"] == 3 and len(row["active"]) == 2, "ACK fence/high-water or retained old handles missing")
    require(row["deadline"] is not None, "removal failed to retain a drain deadline")
    for sid in (5, 7):
        client.request(sid, "/v1/new")
        assert_refused(client, sid)
    assert_no_goaway(client)
    require(not any(record["path"] == "/v1/new" for record in harness.origin.records), "refused request reached origin")
    assert_graceful(client, [1, 3], 3)
    for path in ("/v1/old", "/v1/old-two"):
        assert_credential(harness, path, "one")
    harness.expect_clean_runtime()


def case_rotation(harness):
    harness.install(1)
    client = harness.connect()
    harness.backpressured(client)
    harness.install(2, SECRET_TWO, retry=True)
    row = harness.wait_connection(lambda state: state["deadline"] is not None)
    deadline = row["deadline"]
    require(not row["fenced"] and row["frozen"] is None, "covered rotation incorrectly fenced the connection")
    client.request(3, "/v1/new")
    client.until(lambda: 3 in client.ended)
    assert_redacted(client, 1)
    assert_redacted(client, 3)
    assert_credential(harness, "/v1/old", "one")
    assert_credential(harness, "/v1/new", "two")
    require(any(event["kind"] == "requestheaders" and event["stream_id"] == 3 and event["epoch"] == 2
                for event in harness.events()), "new stream did not pin the new snapshot")
    harness.install(3, SECRET_TWO)
    row = harness.wait_connection(lambda state: state["deadline"] == deadline)
    old = next(record for record in row["active"] if record["stream_id"] == 1)
    require(old["epoch"] == 1 and old["request_deadline"] == deadline, "old snapshot/first deadline drifted")
    assert_no_goaway(client)
    client.credit()
    client.until(lambda: 1 in client.ended)
    require(bytes(client.bodies[1]) == BODY, "rotated old response body was incomplete")
    harness.wait_connection(lambda state: not state["active"] and state["deadline"] is None)
    client.close()
    harness.expect_clean_runtime()


def case_remove_readd(harness):
    harness.install(1)
    client = harness.connect()
    harness.backpressured(client)
    harness.install(2, None)
    first = harness.wait_connection(lambda state: state["fenced"])
    harness.install(3, SECRET_TWO)
    client.request(3, "/v1/new")
    assert_refused(client, 3)
    current = harness.wait_connection(lambda state: state["fenced"])
    require(current["fenced"] and current["frozen"] == first["frozen"] == 1
            and current["deadline"] == first["deadline"], "remove/readd reopened or extended the old connection")
    require(not any(record["path"] == "/v1/new" for record in harness.origin.records), "readded old connection reached origin")
    assert_no_goaway(client)
    assert_graceful(client, [1], 1)
    harness.expect_clean_runtime()


def case_reset_isolation(harness):
    harness.install(1)
    client = harness.connect()
    harness.backpressured(client)
    harness.backpressured(client, 3, "/v1/old-two")
    client.reset(1)
    harness.wait_connection(lambda state: [record["stream_id"] for record in state["active"]] == [3])
    require(harness.state()["registry_requests"] == 1, "reset released the other active stream")
    harness.install(2, None)
    harness.wait_connection(lambda state: state["fenced"] and state["frozen"] == 3)
    assert_no_goaway(client)
    assert_graceful(client, [3], 3)
    harness.expect_clean_runtime()


def case_disconnect(harness):
    harness.install(1)
    client = harness.connect()
    harness.backpressured(client)
    client.close()
    harness.expect_clean_runtime()
    require(harness.process.poll() is None, "normal disconnect incorrectly exited the child")


def case_expiry(harness):
    harness.install(1)
    client = harness.connect()
    harness.backpressured(client)
    started = time.monotonic()
    harness.install(2, SECRET_TWO)
    first = harness.wait_connection(lambda state: state["deadline"] is not None)
    require(abs(first["deadline"] - started - harness.drain_seconds) < 1,
            "configured short/default30 first deadline differs from actual monotonic retirement")
    harness.backpressured(client, 3, "/v1/old-two")
    harness.install(3, SECRET_TWO, retry=True)
    row = harness.wait_connection(lambda state: state["deadline"] == first["deadline"] and len(state["active"]) == 2)
    require(not row["fenced"], "covered expiry case unexpectedly used a host-removal fence")
    client.until(lambda: client.eof, timeout=harness.drain_seconds + 5,
                 message="actual configured drain deadline failed to close the transport")
    elapsed = time.monotonic() - started
    require(harness.drain_seconds - 0.5 <= elapsed <= harness.drain_seconds + 4,
            "actual expiry occurred outside its configured first deadline window")
    require(1 not in client.ended and 3 not in client.ended, "expiry case did not interrupt old and newer backpressured streams")
    assert_no_goaway(client)
    harness.expiry_evidence = {"elapsed_seconds": elapsed, "graceful": False,
                               "newer_stream_collaterally_interrupted": True,
                               "deadline_seconds": harness.drain_seconds,
                               "default_value_explicitly_configured": harness.drain_seconds == 30}
    harness.expect_clean_runtime()
    require(harness.process.poll() is None, "ordinary deadline expiry incorrectly triggered process exit")


def case_d3(harness):
    harness.install(1)
    client = harness.connect()
    client.request(1)
    gate = wait_for(lambda: next((event for event in harness.events() if event["kind"] == "d3_hook_gated"), None),
                    message="real async ServerConnectedHook fault did not reach gate")
    require(gate["actual_tcp_writer"] and gate["task_pending"] and gate["attached"], "D3 fault did not hold an owned real TCP writer")
    client.close()
    code = harness.process.wait(timeout=10)
    require(code == 86, "D3 stranded-writer child did not actually exit with fixed code 86")
    require(not harness.origin.records, "D3 pending-connect case unexpectedly processed a request at origin")
    harness.expect_origin_closed()
    harness.d3_evidence = {"child_exit_code": code, "parent_survived": True,
                           "exact_child_wait_completed": True, "go_owner_or_quarantine_claim": False}


def go_fence_event(events, connection_serial, after_ns):
    """Match only the bounded synthetic new GET on the known post-ACK fence."""
    return next((item for item in events
                 if item.get("kind") == "requestheaders"
                 and item.get("logical_id") == "new"
                 and item.get("attached") is True and item.get("admitted") is False
                 and item.get("fenced") is True
                 and item.get("connection_serial") == connection_serial
                 and item.get("monotonic_ns", 0) >= after_ns), None)


def case_go_retry(harness, binary, required_version, *, bounded_old=False):
    harness.install(1)
    events = []
    release_evidence = {"mode": "post_ack_new_fence" if bounded_old else "after_new_result"}
    normal_cleanup_checked = False
    cleanup_observation = {"attempted": False, "completed": False,
                           "scope": "after Go client process exit, before harness teardown",
                           "client_driven": True, "proxy_first_goaway_proven": False}
    stderr = (harness.root / "go-stderr.log").open("w")
    process = subprocess.Popen(
        [str(binary), "-socks-address", f"127.0.0.1:{harness.proxy_port}",
         "-ca-file", str(harness.combined_ca), "-timeout", "25s", "-new-timeout", "8s",
         "-require-go-version", required_version],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr, text=True,
        env={key: value for key, value in os.environ.items() if key != "SSLKEYLOGFILE"})

    def read_output():
        for line in process.stdout:
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                events.append({"event": "invalid_json"})

    reader = threading.Thread(target=read_output, name="h2-p2-go-output")
    reader.start()

    def event(name):
        return next((item for item in events if item.get("event") == name), None)

    try:
        ready = wait_for(lambda: event("ready") or event("fatal"), timeout=5,
                         message="Go fixture did not announce exact version/config")
        require(ready.get("event") == "ready" and ready.get("go_version") == required_version
                and ready.get("required_go_version") == required_version,
                "Go fixture runtime differs from independently expected version")
        old_headers = wait_for(lambda: event("old_headers"), timeout=10,
                               message="Go old GET did not receive streaming H2 headers")
        require(old_headers["proto"] == "HTTP/2.0" and old_headers["tls_verified"], "Go old request was not verified H2")
        row = harness.wait_connection(lambda state: bool(state["active"]))
        assert_attached(row)
        old_stream = row["active"][0]["stream_id"]
        old_connection_serial = row["connection_serial"]
        harness.install(2, None)
        harness.wait_connection(lambda state: state["fenced"] and state["frozen"] == old_stream)
        new_command_ns = time.monotonic_ns()
        process.stdin.write('{"command":"continue_new"}\n')
        process.stdin.flush()
        if bounded_old:
            # This release is driven solely by the verified production fence,
            # never by the outcome of the new GET or an application retry.
            fence = wait_for(lambda: go_fence_event(harness.events(), old_connection_serial,
                                                    max(harness.final_ack_ns, new_command_ns)), timeout=5,
                             message="bounded-old release did not observe the post-ACK new GET at its old fence")
            before_result_observed = event("new_result") is None
            harness.origin.release.set()
            release_ns = time.monotonic_ns()
            release_evidence.update(fence_event=fence, ack_ns=harness.final_ack_ns,
                                    command_ns=new_command_ns, release_signal_ns=release_ns,
                                    released_before_new_result_observed=before_result_observed,
                                    elapsed_after_command_seconds=(release_ns - new_command_ns) / 1e9)
        new = wait_for(lambda: event("new_result"), timeout=12,
                       message="Go new GET did not reach its independent bounded result")
        if not bounded_old:
            # Preserve the original held-old capability case and any negative
            # result: release only after its independently bounded new result.
            harness.origin.release.set()
            release_evidence.update(command_ns=new_command_ns, release_signal_ns=time.monotonic_ns(),
                                    released_before_new_result_observed=False)
        old = wait_for(lambda: event("old_result"), timeout=8,
                       message="Go old body did not complete after origin release")
        wait_for(lambda: event("done"), timeout=5, message="Go fixture did not finish")
        code = process.wait(timeout=5)
        reader.join(timeout=5)
        require(not reader.is_alive(), "Go stdout reader was not reaped")
        # The Go helper closes its own connections before normal exit. Verify
        # runtime and real-origin cleanup BEFORE judging positive retry success,
        # so the preserved held-old failure still contains cleanup evidence.
        cleanup_observation.update(attempted=True, go_exit_code=code, normal_exit=code == 0,
                                   scope=("after normal Go client exit, before harness teardown" if code == 0
                                          else "after unsuccessful Go client exit, before harness teardown"),
                                   started_ns=time.monotonic_ns())
        harness.cleanup["go_cleanup_origin"] = "client_driven_after_Go_process_exit_before_harness_teardown"
        harness.cleanup["proxy_first_goaway_proven"] = False
        harness.expect_clean_runtime()
        normal_cleanup_checked = True
        cleanup_observation.update(completed=True, finished_ns=time.monotonic_ns())
        require(harness.process.poll() is None, "Go normal cleanup unexpectedly exited mitmdump")
        require(old["do_calls"] == new["do_calls"] == 1, "Go fixture made application-level replay calls")
        require(old["body_complete"] and old["status"] == 200 and not old["error_class"], "library retry interrupted the old body")
        require(old["body_sha256"] == hashlib.sha256(b"data: synthetic-old-start\n\n").hexdigest(), "Go old streaming body hash differs")
        counts = harness.evidence()["origin_attempt_counts"]
        require(counts.get("old") == 1 and counts.get("new", 0) <= 1, "origin handled a logical GET more than once")
        require(code == 0, "Go fixture did not finish with complete observations")
        if bounded_old:
            require(release_evidence["released_before_new_result_observed"]
                    and release_evidence["elapsed_after_command_seconds"] < 8,
                    "bounded-old release was not signaled before observed new_result within the 8-second budget")
        require(new["body_complete"] and new["status"] == 200 and not new["error_class"], "selected Go library did not complete a bounded automatic retry")
        require(new["body_sha256"] == hashlib.sha256(SMALL_BODY).hexdigest(), "Go new body hash differs")
        ids = new["connection_ids"]
        require(ids and ids[0] in old["connection_ids"] and any(value not in old["connection_ids"] for value in ids),
                "Go did not first reuse the fenced connection and then move to a new connection")
        require(go_fence_event(harness.events(), old_connection_serial,
                               max(harness.final_ack_ns, new_command_ns)) is not None,
                "Go new logical request did not reach its original connection's post-ACK production fence")
        assert_credential(harness, "/v1/old", "one")
        assert_credential(harness, "/v1/new", "missing")
    finally:
        harness.origin.release.set()
        forced_go_cleanup = process.poll() is None
        if forced_go_cleanup:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
        process.stdin.close()
        process.stdout.close()
        reader.join(timeout=5)
        stderr.close()
        (harness.root / "go-events.jsonl").write_text("".join(json.dumps(item, sort_keys=True) + "\n" for item in events))
        harness.go_evidence = {"required_go_version": required_version,
                               "actual_go_version": None if event("ready") is None else event("ready").get("go_version"),
                               "exit_code": process.returncode, "exact_child_reaped": process.returncode is not None,
                               "reader_joined": not reader.is_alive(),
                               "events": events, "logical_do_calls_required": 1,
                               "old_release": release_evidence,
                               "retry_outcome": {"old_result": event("old_result"),
                                                 "new_result": event("new_result"),
                                                 "done": event("done"),
                                                 "origin_attempt_counts": harness.evidence()["origin_attempt_counts"]},
                               "cleanup_observation": cleanup_observation,
                               "normal_cleanup_checked_before_retry_assertion": normal_cleanup_checked,
                               "forced_go_cleanup": forced_go_cleanup,
                               "cleanup_scope": "After Go client process exit, before harness teardown; includes client CloseIdleConnections/closeRaw, not proxy-first GOAWAY proof.",
                               "scope": GO_SCOPE["go_retry_bounded_old" if bounded_old else "go_retry"],
                               "proxy_replay": False, "non_idempotent_guarantee": False}


def fingerprint():
    def git(*args):
        return subprocess.check_output(["git", *args], cwd=REPO, text=True).strip()
    versions = {}
    for name in PINS:
        try:
            versions[name] = importlib.metadata.version(name)
        except importlib.metadata.PackageNotFoundError:
            versions[name] = None
    paths = sorted(MITMSCRIPTS.glob("*.py")) + [Path(__file__), OBSERVER,
        HERE / "test_credential_bound_runtime.py", HERE / "fixtures/h2_retry_client.go",
        REPO / "components/egress/pkg/constants/configuration.go"]
    return {"commit": git("rev-parse", "HEAD"), "tree": git("rev-parse", "HEAD^{tree}"),
            "working_tree_status": git("status", "--porcelain"),
            "source_sha256": {str(path.relative_to(REPO)): hashlib.sha256(path.read_bytes()).hexdigest()
                              for path in paths if path.exists()},
            "python": sys.version, "python_executable": sys.executable,
            "platform": platform.platform(), "machine": platform.machine(),
            "dependencies": versions, "required_dependencies": PINS}


def preflight(metadata):
    if sys.implementation.name != "cpython" or sys.version_info[:2] not in ((3, 10), (3, 11)):
        raise Blocked("H2 adapter requires CPython 3.10 or 3.11; eager-task runtimes are unsupported")
    executable = Path(sys.executable).parent / "mitmdump"
    if not executable.is_file():
        raise Blocked("mitmdump must exist beside this pinned venv Python; no install attempted")
    if metadata["dependencies"] != PINS:
        raise Blocked("pinned mitmproxy/h2/hyperframe dependencies do not match")
    if not hasattr(socket, "AF_UNIX"):
        raise Blocked("authenticated AF_UNIX revision IPC is unavailable")
    constants = (REPO / "components/egress/pkg/constants/configuration.go").read_text()
    require("DefaultRevisionDrainTimeoutSeconds = 30" in constants,
            "production default changed; review actual-30s acceptance before running")


def run_case(name, artifacts, args):
    protocol = "h1" if name.endswith("_h1") or name in GO_CASES else "h2"
    scenario = "d3_pending_server_connected" if name == "d3_pending_writer" else "go_retry" if name in GO_CASES else "normal"
    seconds = 30 if name == "expiry_default_30s_h2" else 2 if name == "expiry_short_h2" else 20 if name in GO_CASES else 8
    harness = None
    result = {"case": name, "status": "FAIL", "started_ns": time.monotonic_ns()}
    try:
        harness = RuntimeHarness(artifacts / name, protocol, scenario, seconds)
        harness.start()
        if name.startswith("expiry_"):
            case_expiry(harness)
        elif name == "d3_pending_writer":
            case_d3(harness)
        elif name in GO_CASES:
            case_go_retry(harness, args.go_client, args.require_go_version,
                          bounded_old=name == "go_retry_bounded_old")
        else:
            globals()["case_" + name.rsplit("_", 1)[0]](harness)
        require(name == "d3_pending_writer" or harness.process.poll() is None,
                "normal control unexpectedly exited the mitmdump child")
        require(all(count == 1 for count in harness.evidence()["origin_attempt_counts"].values()),
                "origin handled a synthetic logical request more than once")
        result["status"] = "PASS"
    except Blocked as error:
        result.update(status="BLOCKED", reason=str(error))
    except Exception as error:  # noqa: BLE001 - preserve sanitized failure and cleanup evidence
        # Assertion strings are fixed metadata. Unexpected exceptions get only
        # their type, rather than a potentially header-bearing library message.
        result["reason"] = str(error) if isinstance(error, AssertionError) else type(error).__name__
    finally:
        if harness is not None:
            try:
                harness.finish()
            except Exception as error:  # noqa: BLE001 - preserve sanitized failure and cleanup evidence
                result.update(status="FAIL", cleanup_failure=str(error) if isinstance(error, AssertionError) else type(error).__name__)
            result["evidence"] = harness.evidence()
            for attribute in ("expiry_evidence", "d3_evidence", "go_evidence"):
                if hasattr(harness, attribute):
                    result[attribute] = getattr(harness, attribute)
        result["finished_ns"] = time.monotonic_ns()
    return result


def summarize_results(cases, selected):
    """Keep the manual, held-old and bounded-old evidence denominators distinct."""
    groups = {"manual_peer": CASES, "go_held_old": ("go_retry",),
              "go_bounded_old": ("go_retry_bounded_old",)}
    vocabulary = ("PASS", "FAIL", "BLOCKED", "NOT_RUN")

    def group(names):
        statuses = [item["status"] for item in cases if item["case"] in names]
        status = ("FAIL" if "FAIL" in statuses else "BLOCKED" if "BLOCKED" in statuses
                  else "NOT_RUN" if "NOT_RUN" in statuses or not statuses else "PASS")
        return {"denominator": len(names), "status": status,
                "counts": {value: statuses.count(value) for value in vocabulary}}

    return {"status": group(selected)["status"],
            "summary": {value: sum(item["status"] == value for item in cases)
                        for value in vocabulary},
            "denominator_summaries": {name: group(names) for name, names in groups.items()}}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-real-runtime", action="store_true")
    parser.add_argument("--list", action="store_true", help="List the frozen denominator without starting any network")
    parser.add_argument("--artifacts", type=Path)
    parser.add_argument("--case", action="append", choices=[*CASES, *GO_CASES], dest="cases")
    parser.add_argument("--go-client", type=Path, help="Previously built, independently verified synthetic Go fixture")
    parser.add_argument("--require-go-version", help="Exact externally verified goX.Y.Z; never inferred from the candidate binary")
    args = parser.parse_args(argv)
    if args.list:
        print(json.dumps({"status": "NOT_RUN", "manual_denominator": CASES,
                          "optional_client_cases": GO_CASES, "go_scope": GO_SCOPE,
                          "missing_matrix": MISSING}, indent=2))
        return 0
    if not args.run_real_runtime:
        print(json.dumps({"status": "NOT_RUN", "reason": "Explicit --run-real-runtime authorization gate was not supplied"}))
        return 2
    if args.artifacts is None:
        parser.error("--artifacts NEW_DIR is required")
    if bool(args.go_client) != bool(args.require_go_version):
        parser.error("--go-client and --require-go-version must be supplied together")
    if args.go_client and not args.go_client.is_file():
        parser.error("--go-client must name an existing previously built fixture")
    selected = args.cases or [*CASES, *(GO_CASES if args.go_client else [])]
    if any(name in GO_CASES for name in selected) and not args.go_client:
        parser.error("Go cases require --go-client and independently verified --require-go-version")
    if len(set(selected)) != len(selected):
        parser.error("duplicate --case selections are not allowed")
    args.artifacts = args.artifacts.resolve()
    args.artifacts.mkdir(mode=0o700, parents=False, exist_ok=False)
    report = {"scope": "P2 production addon / loopback SOCKS5 TLS / authenticated Unix IPC only",
              "manual_denominator": CASES, "optional_client_cases": GO_CASES, "go_scope": GO_SCOPE,
              "missing_matrix": MISSING,
              "selected": selected, "cases": [{"case": name, "status": "NOT_RUN"} for name in [*CASES, *GO_CASES]],
              "status": "NOT_RUN", "fingerprint": fingerprint()}
    report.update(summarize_results(report["cases"], selected))
    if args.go_client:
        args.go_client = args.go_client.resolve()
        report["go_fixture"] = {"required_go_version": args.require_go_version,
                                "binary_sha256": hashlib.sha256(args.go_client.read_bytes()).hexdigest()}
    destination = args.artifacts / "report.json"

    def save():
        destination.write_text(json.dumps(report, sort_keys=True, indent=2) + "\n")

    save()
    try:
        preflight(report["fingerprint"])
    except (Blocked, AssertionError) as error:
        report.update(status="BLOCKED", reason=str(error))
        save()
        print(json.dumps({"status": "BLOCKED", "reason": str(error)}), flush=True)
        return 2
    for name in selected:
        result = run_case(name, args.artifacts, args)
        report["cases"][[item["case"] for item in report["cases"]].index(name)] = result
        report.update(summarize_results(report["cases"], selected))
        save()
        print(json.dumps({"case": name, "status": result["status"], "reason": result.get("reason")}), flush=True)
        if result["status"] == "BLOCKED":
            # A denied/occupied canonical port is not an invitation to retry
            # through a different route or start the other identical fixtures.
            break
    report.update(summarize_results(report["cases"], selected))
    save()
    print(json.dumps({"status": report["status"], "summary": report["summary"],
                      "denominator_summaries": report["denominator_summaries"], "report": str(destination), "missing_matrix_count": len(MISSING)}), flush=True)
    return {"PASS": 0, "FAIL": 1, "BLOCKED": 2, "NOT_RUN": 2}[report["status"]]


if __name__ == "__main__":
    raise SystemExit(main())
