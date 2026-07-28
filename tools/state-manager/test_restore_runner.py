#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
"""
Unit tests for RestoreRunner.validate_pvcs (no cluster required).

Regression coverage for the rstrip-vs-suffix bug: PVC names must be derived
from backup filenames by removing the ".tar.gz" suffix, not by stripping the
character set ".targz" (which mangles names like "ais-target" -> "ais-targe").

Run from tools/state-manager:
    python3 -m unittest test_restore_runner -v
"""

import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock


def _make_k8s_stub():
    """Return a MagicMock pre-configured with V1* factories."""
    stub = mock.MagicMock()
    stub.client.V1ObjectMeta.side_effect = (
        lambda name=None, labels=None: SimpleNamespace(name=name, labels=labels)
    )
    stub.client.V1PersistentVolumeClaim.side_effect = (
        lambda metadata=None, spec=None: SimpleNamespace(metadata=metadata, spec=spec)
    )
    return stub


class TestRestoreRunnerValidatePvcs(unittest.TestCase):
    """PVC-name derivation from per-PVC backup filenames."""

    def setUp(self):
        # Stub cluster-only deps inside a temporary patch so they cannot leak
        # into other tests. Preserve any pre-existing module cache entries.
        self._k8s_stub = _make_k8s_stub()
        self._pod_config_stub = mock.MagicMock()
        self._module_patcher = mock.patch.dict(
            sys.modules,
            {
                "kubernetes": self._k8s_stub,
                "pod_config": self._pod_config_stub,
            },
        )
        self._module_patcher.start()
        # Drop any cached restore_runner so the import below sees the stubs.
        self._prior_restore_runner = sys.modules.pop("restore_runner", None)

        from restore_runner import RestoreRunner  # noqa: E402  pylint: disable=wrong-import-position

        self.RestoreRunner = RestoreRunner

    def tearDown(self):
        self._module_patcher.stop()
        # Restore any prior restore_runner cache entry.
        sys.modules.pop("restore_runner", None)
        if self._prior_restore_runner is not None:
            sys.modules["restore_runner"] = self._prior_restore_runner

    def _make_runner(self, pvc_backups: Path, existing_pvcs=None):
        manager = mock.MagicMock()
        manager.find_pvcs.return_value = existing_pvcs or []
        with mock.patch.object(
            self.RestoreRunner, "init_pvc_backups_dir", return_value=pvc_backups
        ):
            return self.RestoreRunner(manager, Path("dummy.tar.gz"), mock.MagicMock())

    def test_derives_pvc_names_by_suffix_removal(self):
        """Names ending in .targz characters must survive suffix removal."""
        backups = [
            "ais-target.tar.gz",
            "ais-proxy.tar.gz",
            "ais-target-state.tar.gz",
            "my-config.tar.gz",
            "data-tag.tar.gz",
        ]
        expected = sorted(
            ["ais-target", "ais-proxy", "ais-target-state", "my-config", "data-tag"]
        )
        with tempfile.TemporaryDirectory() as tmpdir:
            backup_dir = Path(tmpdir)
            for name in backups:
                backup_dir.joinpath(name).touch()
            runner = self._make_runner(backup_dir)
            desired = runner.validate_pvcs()

        self.assertEqual(sorted(desired), expected)
        # No existing PVCs -> exactly the desired set must be created
        created = [
            c.args[0].metadata.name for c in runner.manager.create_pvc.call_args_list
        ]
        self.assertEqual(sorted(created), expected)

    def test_matching_existing_pvcs_are_accepted(self):
        """Existing PVCs matching the restore file skip creation."""
        with tempfile.TemporaryDirectory() as tmpdir:
            backup_dir = Path(tmpdir)
            backup_dir.joinpath("ais-target.tar.gz").touch()
            runner = self._make_runner(backup_dir, existing_pvcs=["ais-target"])
            desired = runner.validate_pvcs()

        self.assertEqual(desired, ["ais-target"])
        runner.manager.create_pvc.assert_not_called()


if __name__ == "__main__":
    unittest.main()
