# GenFlow: A Phylogenomic Analysis Pipeline

GenFlow is a streamlined phylogenomic analysis pipeline that combines several tools, including NCBI’s download tool, EDirect, Anvi’o, MAFFT, and FastTree, to provide users with an easy, one-command solution for performing phylogenomic analysis.

## Repository
All the necessary data to perform the analysis is available in the [GenFlow GitHub repository](https://github.com/braddmg/GenFlow).

## Prerequisites

Make sure you have the following installed:
- **Conda** for environment management.

## Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/braddmg/GenFlow.git
   cd GenFlow
   ```
2. Create the Conda environment: Run the following command to create the environment from the GenFlow.yml file provided:
 ```bash
  conda env create -f GenFlow.yml
 ```
3. Set up the Conda environment: After creating the environment, run the setup.sh file to install all necessary dependencies and set up the pipeline:
```bash
  bash setup.sh
 ```
4. Reload the Conda environment: After running the setup.sh script, you will need to reset your Conda environment by deactivating and reactivating it:
```bash
 conda deactivate
conda activate GenFlow
 ```
5. Run the GenFlow command to verify installation: The GenFlow command is now pre-installed as part of the environment. You can check that everything is installed correctly by running the help option:
```bash
GenFlow -h
 ```
This will show this message:

Don't worry; sometimes we also don't know what to do.
Syntax: script [-f|-h|-g|-t|-G|-F]
Options:
-g     A txt file containing all accession numbers of reference genomes (default: genomes.txt)
-h     Print this Help.
-f     FASTA file(s). If you have more than one, provide file names separated by coma:
       file1.fasta,file2.fasta,etc. If you do not specify it, GenFlow will use all fasta in the folder.
-t     Number of threads—don't overwhelm your computer.
-G     Geometric Index value for selecting core genes (default: 0.8).
-F     Functional Index value for selecting core genes (default: 0.8).

## Genome Data
Inside the repository, you'll find a Data folder with three fasta files and two sets of reference genomes:

Short Dataset: Contains 12 reference genomes. We recommend starting with this smaller dataset for your first run.
Large Dataset: Contains a larger number of reference genomes for more comprehensive analysis.
You can use either dataset based on your computational capacity and time constraints.

Example Command
For your first run, we suggest using the short dataset to test the pipeline. Here is an example command to execute the analysis:
```bash
cd Data
GenFlow -g short.txt -f INISA09F.fasta,INISA10F.fasta,INISA16F.fasta -t 8 -G 0.8 -F 0.8
 ```
In this example:

-g specifies the file containing the accession numbers of the reference genomes.
-f specifies the FASTA files for the analysis.
-t specifies the number of threads to use (recommended 8).
-G and -F specify the geometric and functional index values for core gene selection. Here we selected 0.8 but it is also the default value for both options.

## Results
Once the analysis completes, you will find the results in a newly created results/ directory. This folder contains:

Phylogenomic Tree: The output of the phylogenomic analysis.
pyANI Results: The results of the Average Nucleotide Identity (ANI) analysis.
The analysis process will culminate in a message indicating that your phylogenomic plots and ANI results are ready.
