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

# Stub out the kubernetes client and pod_config imports (cluster-only deps).
# The V1* factories return plain namespaces so manifests keep their fields.
_k8s_stub = mock.MagicMock()
_k8s_stub.client.V1ObjectMeta.side_effect = (
    lambda name=None, labels=None: SimpleNamespace(name=name, labels=labels)
)
_k8s_stub.client.V1PersistentVolumeClaim.side_effect = (
    lambda metadata=None, spec=None: SimpleNamespace(metadata=metadata, spec=spec)
)
sys.modules.setdefault("kubernetes", _k8s_stub)
sys.modules.setdefault("pod_config", mock.MagicMock())

from restore_runner import RestoreRunner  # noqa: E402  pylint: disable=wrong-import-position


class TestRestoreRunnerValidatePvcs(unittest.TestCase):
    """PVC-name derivation from per-PVC backup filenames."""

    def _make_runner(self, pvc_backups: Path, existing_pvcs=None) -> RestoreRunner:
        manager = mock.MagicMock()
        manager.find_pvcs.return_value = existing_pvcs or []
        with mock.patch.object(
            RestoreRunner, "init_pvc_backups_dir", return_value=pvc_backups
        ):
            return RestoreRunner(manager, Path("dummy.tar.gz"), mock.MagicMock())

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
