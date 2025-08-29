#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
# This script is a lightweight utility for converting the single alert-rules.yaml
# into individual alert yaml files that match the format used in our other repositories. 
# These individual files are then parsed and uploaded by another script in the 
# format for Grafana provisioned alerts
#

import argparse
from pathlib import Path

import yaml

def load_rules(input_file):
    with open(input_file, "r") as f:
        data = yaml.safe_load(f)
    return data["additionalPrometheusRulesMap"]["ais-rules"]["groups"][0]["rules"]

def write_yaml(file, content):
    with open(file, "w") as out:
        yaml.dump(content, out, width=float("inf"))

def process_rules(rules, output_dir, datasources):
    for rule in rules:
        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)
        alert_name = rule.get("alert")
        output_file = output_path.joinpath(f"{alert_name}.yaml")
        rule["regions"] = datasources
        labels = rule.get("labels", {})
        labels["team"] = "aistore"
        rule["labels"] = labels
        write_yaml(output_file, rule)


def main(args):
    rules = load_rules(args.input)
    process_rules(rules, args.output, args.datasources)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Input file')
    parser.add_argument('--input', type=str, default='../values/alert-rules.yaml', required=False, help='Path to the input alert rules yaml.')
    parser.add_argument(
        '--datasources',
        type=lambda s: s.split(','),
        default=['mimir-us-east-1', 'mimir-us-west-2', 'mimir-eu-north-1'],
        metavar='DATASOURCE1,DATASOURCE2',
        help='Comma-separated list of datasource names, e.g. mimir-us-east-1,mimir-us-west-2'
    )
    parser.add_argument(
        '--output',
        type=str,
        default='Storage_AIStore',
        help='Output folder for alert rules'
    )

    input_args = parser.parse_args()
    main(input_args)