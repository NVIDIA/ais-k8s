# AIS Operator Helper Docker Image

The `ais-operator-helper` Docker image contains essential utilities for the AIS Operator. These tools assist in various operational tasks to maintain and manage the AIS cluster efficiently.

| Executable Name | Description |
|-----------------|-------------|
| [`cleanup-helper`](src/cleanup-helper.go) | The `cleanup-helper` is designed to perform cleanup operations across all nodes within an AIS cluster. It deletes all files matching the `.ais.*` pattern within a specified directory.<br>**Usage:**<br>`/cleanup-helper -dir=/etc/ais`<br>This command in the docker image will delete all files matching the pattern in the `/etc/ais` directory. |
