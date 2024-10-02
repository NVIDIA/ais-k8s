#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Define the source and destination paths
sources=(
    "$PROJECT_ROOT/pkg/services/aisapi.go"
    "$PROJECT_ROOT/pkg/services/client_manager.go"
)
destinations=(
    "$PROJECT_ROOT/pkg/services/mocks/mock_ais_client.go"
    "$PROJECT_ROOT/pkg/services/mocks/mock_client_manager.go"
)

# Loop through the sources and generate mocks
for i in "${!sources[@]}"; do
    source="${sources[$i]}"
    destination="${destinations[$i]}"

    echo "Generating mock for $source"
    mockgen -source="$source" -destination="$destination"

    if [ $? -eq 0 ]; then
        echo "Successfully generated mock: $destination"
    else
        echo "Failed to generate mock for $source"
    fi
    echo
done

echo "Mock generation complete"
