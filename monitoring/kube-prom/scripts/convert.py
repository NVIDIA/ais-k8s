#
# Copyright (c) 2025-2026, NVIDIA CORPORATION. All rights reserved.
#
# Lightweight utility for converting output of the local `alert-rules` Helm chart into individual per-alert YAML files
# matching the format used in our other repositories.
# The per-alert files can then be parsed and uploaded separately as Grafana-provisioned alerts.
#

import argparse
import subprocess
import sys
from pathlib import Path

import yaml

DEFAULT_CHART = Path(__file__).resolve().parent.parent / "alert-rules"


def render_chart(chart_path: Path, values_files):
    """Render the alert-rules chart via `helm template` and return its stdout."""
    cmd = ["helm", "template", "alert-rules", str(chart_path)]
    for vf in values_files:
        cmd.extend(["-f", str(vf)])
    try:
        result = subprocess.run(
            cmd, check=True, capture_output=True, text=True
        )
    except FileNotFoundError:
        sys.exit("error: `helm` not found on PATH; install Helm to render the chart")
    except subprocess.CalledProcessError as e:
        sys.exit(f"error: helm template failed:\n{e.stderr}")
    return result.stdout


def extract_rules(rendered_yaml: str):
    """Parse the rendered manifest stream and return the alert rules list."""
    rules = []
    for doc in yaml.safe_load_all(rendered_yaml):
        if not doc:
            continue
        if doc.get("kind") != "PrometheusRule":
            continue
        for group in doc.get("spec", {}).get("groups", []):
            rules.extend(group.get("rules", []))
    if not rules:
        sys.exit("error: no PrometheusRule rules found in rendered chart output")
    return rules


def write_yaml(file: Path, content):
    with open(file, "w") as out:
        yaml.dump(content, out, width=float("inf"))


def process_rules(rules, output_dir, datasources):
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)
    for rule in rules:
        alert_name = rule.get("alert")
        if not alert_name:
            continue
        output_file = output_path / f"{alert_name}.yaml"
        rule["regions"] = datasources
        labels = rule.get("labels", {})
        labels["team"] = "aistore"
        rule["labels"] = labels
        write_yaml(output_file, rule)


def main(args):
    rendered = render_chart(args.chart, args.values or [])
    rules = extract_rules(rendered)
    process_rules(rules, args.output, args.datasources)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Render the alert-rules Helm chart and emit per-alert YAML "
        "files for the Grafana provisioning pipeline."
    )
    parser.add_argument(
        "--chart",
        type=Path,
        default=DEFAULT_CHART,
        help="Path to the alert-rules Helm chart (default: ../alert-rules).",
    )
    parser.add_argument(
        "--values",
        action="append",
        type=Path,
        default=None,
        metavar="FILE",
        help="Extra values file to pass to `helm template -f`. Repeatable.",
    )
    parser.add_argument(
        "--datasources",
        type=lambda s: s.split(","),
        default=["mimir-us-east-1", "mimir-us-west-2", "mimir-eu-north-1", "mimir-ap-northeast-1"],
        metavar="DATASOURCE1,DATASOURCE2",
        help="Comma-separated list of datasource names, e.g. mimir-us-east-1,mimir-us-west-2",
    )
    parser.add_argument(
        "--output",
        type=str,
        default="Storage_AIStore",
        help="Output folder for alert rules.",
    )

    main(parser.parse_args())
