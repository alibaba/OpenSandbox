# Copyright 2026 The OpenSandbox Authors
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

"""Hermetic runner tests: fake Go, sudo, and unshare; no root/namespace operations."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("ci-privileged-go-test.sh")
MOUNT_TESTS = [
    "TestAtomicPolicyDirectoryBindMount",
    "TestAtomicPolicyAbsentDirectoryBindMount",
    "TestAtomicPolicySingleFileBindMountRejected",
    "TestAtomicPolicyReadOnlyDirectoryBindMountRejected",
    "TestAtomicPolicySymlinkRejected",
]
NFT_TESTS = [
    "TestDynamicElementRenewal",
    "TestNftQuiescenceAfterCommittedStaticError",
    "TestNftStartFailurePreservesKernelAndRetry",
    "TestNftRealMissingTableFallback",
]
OWNER_TESTS = [
    "TestRevisionAtomicPolicyDirectoryBindMount",
    "TestRevisionAtomicPolicySingleFileBindMountRejected",
]

FAKE_COMMAND = r'''
import json
import os
from pathlib import Path
import shutil
import signal
import sys

kind = os.getenv("FAKE_KIND", "mount")
opt_in = "OPENSANDBOX_NFT_TEST" if kind == "nft" else "OPENSANDBOX_ATOMIC_POLICY_MOUNT_TEST"
command = Path(sys.argv[0]).name
args = sys.argv[1:]
mode = os.environ["FAKE_MODE"]
tests = json.loads(os.environ["FAKE_TESTS"])
target = tests[-1]
with open(os.environ["FAKE_EVENTS"], "a") as stream:
    stream.write(json.dumps({"command": command, "args": args,
                             "opt_in": os.getenv(opt_in)}) + "\n")

if command == "go":
    assert args[:3] == ["test", "-c", "-o"] and len(args) == 5, args
    binary = Path(args[3])
    binary.write_text("partial compile")
    if mode == "compile_failure":
        sys.exit(23)
    if mode.startswith("signal_"):
        os.kill(os.getppid(), getattr(signal, "SIG" + mode.removeprefix("signal_")))
        sys.exit(0)
    shutil.copyfile(__file__, binary)
    binary.chmod(0o755)
elif command == "sudo":
    if kind == "nft":
        assert args[:2] == ["env", "OPENSANDBOX_NFT_TEST=1"], args
        if mode == "unshare_failure":
            sys.exit(24)
    else:
        assert args[:3] == ["env", "OPENSANDBOX_ATOMIC_POLICY_MOUNT_TEST=1", "unshare"], args
    os.execvp(args[0], args)
elif command == "unshare":
    assert args[:3] == ["--mount", "--propagation", "private"], args
    assert os.environ["OPENSANDBOX_ATOMIC_POLICY_MOUNT_TEST"] == "1"
    if mode == "unshare_failure":
        sys.exit(24)
    os.execv(args[3], args[3:])
elif command == "policy.test":
    if args[0] == "-test.list":
        assert len(args) == 2 and args[1].startswith("^") and args[1].endswith("$"), args
        name = args[1][1:-1]
        assert name in tests, name
        if mode == "missing_test" and name == target:
            sys.exit(0)
        print(name + "Extra" if mode == "inexact_discovery" and name == target else name)
        if mode == "discovery_failure" and name == target:
            sys.exit(25)
    else:
        assert args == ["-test.run", "^(" + "|".join(tests) + ")$", "-test.v", "-test.timeout=5m"], args
        assert os.environ[opt_in] == "1"
        for name in tests:
            if mode != "absent_run" or name != target:
                print("=== RUN   " + name + ("Extra" if mode == "inexact_run" and name == target else ""))
            if mode != "absent_pass" or name != target:
                print("--- PASS: " + name + ("Extra" if mode == "inexact_pass" and name == target else "") + " (0.01s)")
        if mode == "skip":
            print("--- SKIP: " + tests[0] + " (0.00s)")
        if mode == "test_failure":
            sys.exit(26)
else:
    raise AssertionError(command)
'''


class PrivilegedGoTestRunnerTests(unittest.TestCase):
    script = SCRIPT
    kind = "mount"
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="privileged-runner-test-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.tmp = self.root / "tmp with spaces"
        self.tmp.mkdir()
        self.sentinel = self.tmp / "unrelated-file"
        self.sentinel.write_text("keep")
        self.events = self.root / "events.jsonl"
        self.bash = shutil.which("bash")
        # A closed PATH prevents a missing fake from invoking real sudo/unshare.
        for name in ("env", "mktemp", "rm", "grep", "tee"):
            (self.bin / name).symlink_to(shutil.which(name))
        for name in ("go", "sudo", "unshare", "nft"):
            fake = self.bin / name
            fake.write_text(f"#!{sys.executable}\n" + FAKE_COMMAND)
            fake.chmod(0o755)

    def run_script(self, mode="success", package="./pkg/policy", tests=None):
        if tests is None:
            tests = MOUNT_TESTS
        env = {
            **os.environ,
            "PATH": str(self.bin),
            "TMPDIR": str(self.tmp),
            "FAKE_MODE": mode,
            "FAKE_KIND": self.kind,
            "FAKE_TESTS": json.dumps(tests),
            "FAKE_EVENTS": str(self.events),
        }
        result = subprocess.run(
            [self.bash, str(self.script), package, *tests],
            env=env,
            capture_output=True,
            text=True,
            timeout=10,
        )
        self.assertEqual(list(self.tmp.iterdir()), [self.sentinel], result.stdout + result.stderr)
        self.assertEqual(self.sentinel.read_text(), "keep")
        events = [json.loads(line) for line in self.events.read_text().splitlines()] if self.events.exists() else []
        self.events.unlink(missing_ok=True)
        return result, events

    def test_success_for_both_atomic_policy_groups(self):
        binary_paths = []
        for package, tests in (("./pkg/policy", MOUNT_TESTS), (".", OWNER_TESTS)):
            with self.subTest(package=package):
                result, events = self.run_script(package=package, tests=tests)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(events[0]["args"][-1], package)
                binary_paths.append(events[0]["args"][3])
                self.assertEqual(
                    [event["args"] for event in events if event["args"][0] == "-test.list"],
                    [["-test.list", f"^{name}$"] for name in tests],
                )
                self.assertEqual([event["command"] for event in events].count("sudo"), 1)
                self.assertEqual([event["command"] for event in events].count("unshare"), 0 if self.kind == "nft" else 1)
                self.assertEqual(events[-1]["opt_in"], "1")
        self.assertNotEqual(*binary_paths)

    def test_compile_failure_cleans_partial_binary(self):
        result, events = self.run_script("compile_failure")
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual([event["command"] for event in events], ["go"])

    def test_discovery_must_succeed_and_match_exactly(self):
        for mode in ("missing_test", "inexact_discovery", "discovery_failure"):
            with self.subTest(mode=mode):
                result, events = self.run_script(mode)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn("sudo", [event["command"] for event in events])

    def test_unshare_failure_is_not_hidden_by_tee(self):
        result, events = self.run_script("unshare_failure")
        self.assertEqual(result.returncode, 24, result.stderr)
        self.assertEqual(events[-1]["command"], "sudo" if self.kind == "nft" else "unshare")

    def test_test_failure_is_not_hidden_by_tee(self):
        result, _ = self.run_script("test_failure")
        self.assertEqual(result.returncode, 26, result.stderr)

    def test_skip_and_missing_or_inexact_evidence_are_rejected(self):
        for mode in ("skip", "absent_run", "absent_pass", "inexact_run", "inexact_pass"):
            with self.subTest(mode=mode):
                result, _ = self.run_script(mode)
                self.assertNotEqual(result.returncode, 0, result.stdout)

    def test_missing_unshare_fails_before_compilation(self):
        (self.bin / "unshare").unlink()
        result, events = self.run_script()
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(events, [])

    def test_malformed_names_fail_before_compilation(self):
        for name in ("TestFoo|TestBar", "TestFoo.*", "TestFoo/subtest", "-test.run", "TestFoo\nTestBar", ""):
            with self.subTest(name=name):
                result, events = self.run_script(tests=[name])
                self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
                self.assertIn("Invalid top-level Go test name:", result.stderr)
                self.assertEqual(events, [])

    def test_missing_test_arguments_are_rejected(self):
        result, events = self.run_script(tests=[])
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("Usage:", result.stderr)
        self.assertEqual(events, [])

    def test_signals_clean_partial_binary(self):
        for name, exit_code in (("HUP", 129), ("INT", 130), ("TERM", 143)):
            with self.subTest(signal=name):
                result, events = self.run_script("signal_" + name)
                self.assertEqual(result.returncode, exit_code, result.stdout + result.stderr)
                self.assertEqual([event["command"] for event in events], ["go"])


class NftGoTestRunnerTests(PrivilegedGoTestRunnerTests):
    script = SCRIPT.with_name("ci-nft-go-test.sh")
    kind = "nft"

    def test_success_for_nft_and_owner_groups(self):
        for package, tests in (("./pkg/nftables", NFT_TESTS), (".", ["TestRevisionNftEffectsRealFileAndKernel"])):
            with self.subTest(package=package):
                result, events = self.run_script(package=package, tests=tests)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(events[0]["args"][-1], package)
                self.assertEqual(events[-1]["opt_in"], "1")
                self.assertEqual([event["command"] for event in events].count("sudo"), 1)
                self.assertNotIn("unshare", [event["command"] for event in events])

    def test_missing_nft_fails_before_compilation(self):
        (self.bin / "nft").unlink()
        result, events = self.run_script()
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(events, [])


if __name__ == "__main__":
    unittest.main()
