#!/bin/bash
CONDA_ENV_PATH=$(conda info --base)/envs/$(basename "$CONDA_PREFIX")
export ANI=$CONDA_ENV_PATH/scripts/ANI.R

# Check if the input file is provided
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <input_file>"
    exit 1
fi

# Run the R script with the provided input file
Rscript "$ANI" "$1"
