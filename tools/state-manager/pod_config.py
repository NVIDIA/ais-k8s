#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
from dataclasses import dataclass, field
from typing import List, Dict

@dataclass
class PodConfig:
    name: str
    image: str
    container_name: str
    command: List[str]
    exec_cmd: str
    labels: Dict[str, str] = field(default_factory=dict)

    @property
    def label_selector(self) -> str:
        return ','.join(f'{key}={value}' for key, value in self.labels.items())