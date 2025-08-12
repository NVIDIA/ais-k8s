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

def process_rules(rules, output_dir, datasource):
    for rule in rules:
        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)
        alert_name = rule.get("alert")
        output_file = output_path.joinpath(f"{alert_name}.yaml")
        rule["data"] = {"datasourceUid": datasource}
        write_yaml(output_file, rule)


class ParseOutputs(argparse.Action):
    def __call__(self, parser, namespace, values, option_string=None):
        result = {}
        for value in values:
            folder, datasource = value.split('=', 1)
            result[folder] = datasource
        setattr(namespace, self.dest, result)


def main(args):
    rules = load_rules(args.input)
    for output_dir, datasource in args.outputs.items():
        process_rules(rules, output_dir, datasource)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Input file')
    parser.add_argument('--input', type=str, default='../values/alert-rules.yaml', required=False, help='Path to the input alert rules yaml.')
    parser.add_argument(
        '--outputs',
        nargs='*',  # accept multiple folder=datasource pairs
        action=ParseOutputs,
        default={
            'Storage_AIStore_EAST': 'mimir-us-east-1',
            'Storage_AIStore_WEST': 'mimir-us-west-2'
        },
        metavar='FOLDER=DATASOURCE',
        help='Map output folders to their data sources, e.g. AIS-West=mimir-west AIS-East=mimir-east'
    )

    input_args = parser.parse_args()
    main(input_args)