#!/bin/bash

#GenoFlow script
CONDA_ENV_PATH=$(conda info --base)/envs/$(basename "$CONDA_PREFIX")
export script=$CONDA_ENV_PATH/scripts/GenFlow.sh
alias GenFlow="bash $script"
