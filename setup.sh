#!/bin/bash
# Initial message
exec > /dev/tty 2>&1  # Enable output for the initial message
echo "Just wait, we're casting some digital spells to make everything run smoothly on your machine! And don't forget, you'll need to enter your WSL password when asked to help install some important packages (or maybe not) :)"

# Suppress all output except errors
exec > /dev/null 2>&1

# Download and install necessary packages
curl -L https://github.com/merenlab/anvio/releases/download/v8/anvio-8.tar.gz --output anvio-8.tar.gz
wget https://files.pythonhosted.org/packages/c4/aa/94c42a2dd30c4923bffe3ab59e4416c3f7e72dbd32a89bcdd8d43ff1d5d7/datrie-0.8.2-pp373-pypy36_pp73-win32.whl
mv datrie-0.8.2-pp373-pypy36_pp73-win32.whl datrie-0.8.2-cp310-none-any.whl
pip install datrie-0.8.2-cp310-none-any.whl
rm datrie-0.8.2-cp310-none-any.whl
pip install anvio-8.tar.gz
rm anvio-8.tar.gz
pip uninstall matplotlib --yes
conda install pyani --yes
pip install matplotlib

if ! command -v unzip &> /dev/null; then
    sudo apt-get update -qq > /dev/null 2>&1 && sudo apt-get install -y unzip -qq > /dev/null 2>&1;
fi
#
chmod +x GenFlow.py
chmod +x scripts/datasets
chmod -R +x scripts/edirect
chmod +x scripts/ANI.R

# Install R libraries (Add your required packages here)
Rscript -e "if (!requireNamespace('pheatmap', quietly = TRUE)) install.packages('pheatmap', repos='http://cran.r-project.org')"
Rscript -e "if (!requireNamespace('grid', quietly = TRUE)) install.packages('grid', repos='http://cran.r-project.org')"
Rscript -e "if (!requireNamespace('viridis', quietly = TRUE)) install.packages('viridis', repos='http://cran.r-project.org')"
Rscript -e "if (!requireNamespace('ggplot2', quietly = TRUE)) install.packages('ggplot2', repos='http://cran.r-project.org')"
Rscript -e "if (!requireNamespace('reshape2', quietly = TRUE)) install.packages('reshape2', repos='http://cran.r-project.org')"

# Restore output
exec > /dev/tty 2>&1
echo "Setup completed! Please reset your conda environment and then you will be ready to do bioinformatics (or try it, sometimes it just depends on your machine’s mood!)"
