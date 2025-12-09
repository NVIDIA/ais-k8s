#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
from enum import Enum
from typing import List


class AISMetadata(Enum):
    smap = ".ais.smap"
    conf = ".ais.conf"
    bmd = ".ais.bmd"
    rmd = ".ais.rmd"
    override = ".ais.override_config"
    all = ".ais.*"

    @staticmethod
    def get_options() -> List[str]:
        return [e.name for e in AISMetadata]

    @staticmethod
    def get_options_str() -> str:
        return ",".join(AISMetadata.get_options())
