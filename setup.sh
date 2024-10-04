#!/bin/bash
# Suppress all output except errors
exec > /dev/null 2>&1
# Get the current Conda environment path
CONDA_ENV_PATH=$(conda info --base)/envs/$(basename "$CONDA_PREFIX")
curl -L https://github.com/merenlab/anvio/releases/download/v8/anvio-8.tar.gz \
        --output anvio-8.tar.gz
wget https://files.pythonhosted.org/packages/c4/aa/94c42a2dd30c4923bffe3ab59e4416c3f7e72dbd32a89bcdd8d43ff1d5d7/datrie-0.8.2-pp373-pypy36_pp73-win32.whl
mv datrie-0.8.2-pp373-pypy36_pp73-win32.whl datrie-0.8.2-cp310-none-any.whl
pip install datrie-0.8.2-cp310-none-any.whl
rm datrie-0.8.2-cp310-none-any.whl
pip install anvio-8.tar.gz
rm anvio-8.tar.gz
pip uninstall matplotlib --yes
conda install pyani --yes
pip install matplotlib 
if ! command -v unzip &> /dev/null; then sudo apt-get update -qq > /dev/null 2>&1 && sudo apt-get install -y unzip -qq > /dev/null 2>&1; fi

# Create directories for activation and deactivation scripts
mkdir -p "$CONDA_ENV_PATH/etc/conda/activate.d"
mkdir -p "$CONDA_ENV_PATH/etc/conda/deactivate.d"
mkdir -p "$CONDA_ENV_PATH/scripts"

# Copy the activation and deactivation scripts
cp scripts/variables.sh "$CONDA_ENV_PATH/etc/conda/activate.d/"
cp scripts/clear_variables.sh "$CONDA_ENV_PATH/etc/conda/deactivate.d/"

# Copy custom scripts
cp scripts/GenFlow.sh "$CONDA_ENV_PATH/scripts/"
cp scripts/datasets "$CONDA_ENV_PATH/scripts/"
cp -r scripts/edirect "$CONDA_ENV_PATH/scripts/"

# Make the scripts executable
chmod +x "$CONDA_ENV_PATH/etc/conda/activate.d/variables.sh"
chmod +x "$CONDA_ENV_PATH/etc/conda/deactivate.d/clear_variables.sh"
chmod +x "$CONDA_ENV_PATH/scripts/GenFlow.sh"
chmod +x "$CONDA_ENV_PATH/scripts/datasets"
chmod -R +x "$CONDA_ENV_PATH/scripts/edirect"
exec > /dev/tty 2>&1
echo "Setup completed! Please reset your conda environment and then you will be are ready to do bioinformatics (or try it)"