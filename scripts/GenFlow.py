import os
import subprocess
import sys
import time
import argparse
import glob

# Set up argument parser
parser = argparse.ArgumentParser(description="GenFlow: A tool to perform genomic analysis workflows.")
parser.add_argument('-f', '--fasta', type=str, help="FASTA files, separated by commas. If you do not specify, GenFlow will use all the .fasta files in the folder")
parser.add_argument('-g', '--genomes', type=str, default="genomes.txt", help="Text file containing genome accession numbers (default: genomes.txt)")
parser.add_argument('-t', '--threads', type=int, default=8, help="Number of threads (default: 8)")
parser.add_argument('-G', '--geometric_index', type=float, default=0.8, help="Geometric Index value for selecting core genes (default: 0.8)")
parser.add_argument('-F', '--functional_index', type=float, default=0.8, help="Functional Index value for selecting core genes (default: 0.8)")
parser.add_argument('-N', '--dna_mode', action='store_true', help="Enable DNA mode  (default: False)")
parser.add_argument('-I', '--mcl_inflation', type=int, default=10, help="MCL inflation value (default: 10)")
parser.add_argument('-M', '--minbit', type=float, default=0.5, help="Minimum bit score (default: 0.5)")
parser.add_argument('-P', '--min_percent_identity', type=float, default=0, help="Minimum percent identity for pan-genome (default: 0)")
args = parser.parse_args()

# Default values if no FASTA files specified
if not args.fasta:
    fasta_files = [f for f in os.listdir() if f.endswith(".fasta")]
else:
    # Handle wildcard in -f argument
    if '*' in args.fasta:
        fasta_files = glob.glob(args.fasta)  # Expand wildcard to matching files
    else:
        fasta_files = args.fasta.split(',')

# Set up conda environment and paths
CONDA_ENV_PATH = $(conda info --base)/envs/$(basename "$CONDA_PREFIX")
dataset = f"{CONDA_ENV_PATH}/scripts/./datasets"
esearch = f"{CONDA_ENV_PATH}/scripts/edirect/esearch"
esummary = f"{CONDA_ENV_PATH}/scripts/edirect/esummary"
xtract = f"{CONDA_ENV_PATH}/scripts/edirect/xtract"
rscript = f"{CONDA_ENV_PATH}/scripts/./run_script.sh"

# Validate files
if not os.path.isfile(args.genomes):
    print(f"Error: File '{args.genomes}' not found.")
    sys.exit(1)

print(f"Using genomes file: {args.genomes}")
print(f"Using FASTA files: {fasta_files}")
print(f"Geometric Index: {args.geometric_index}")
print(f"Functional Index: {args.functional_index}")
print(f"Threads: {args.threads}")
print(f"Dna Mode: {args.dna_mode}")
print(f"MCL Inflation: {args.mcl_inflation}")
print(f"Minimum Bit Score: {args.minbit}")
print(f"Minimum Percent Identity: {args.min_percent_identity}")

# Time the workflow
start_time = time.time()

# Make necessary directories
os.makedirs('logs', exist_ok=True)
os.makedirs('Intermediate', exist_ok=True)
os.makedirs('results', exist_ok=True)

# Download genomes
print("Downloading genomes...")
subprocess.run(
    [dataset, "download", "genome", "accession", "--inputfile", args.genomes],
    stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True
)
subprocess.run("unzip ncbi*", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("rm *.zip", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mv ncbi_dataset/data/GC*/*.fna Intermediate/", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("rm -r ncbi_dataset md5sum.txt README.md", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Copy FASTA files into intermediate directory
for fasta in fasta_files:
    subprocess.run(f"cp {fasta} Intermediate/", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

print("Renaming genomes...")
# Rename files with taxa names
os.chdir("Intermediate")
subprocess.run("ls -1 *.fna > OLD", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Open the 'NEW' file to write output for each .fna file
with open("NEW", "w") as new_file:
    for f in os.listdir():
        if f.endswith(".fna"):
            term = "_".join(f.split('_', 2)[:2])  # Extract the first two parts of the file name
            # Add a sanity check to ensure the term is valid for esearch
            print(f"Processing file: {f} with search term: {term}")
            try:
                esearch_cmd = f"{esearch} -db assembly -query {term}"
                esummary_cmd = f"{esummary}"
                xtract_cmd = f"{xtract} -pattern DocumentSummary -element Organism,Strain,AssemblyAccession"
                full_cmd = f"{esearch_cmd} | {esummary_cmd} | {xtract_cmd}"
                print(f"Running command: {full_cmd}")  # Debugging line to show command being executed

                result = subprocess.check_output(full_cmd, shell=True, text=True, stderr=subprocess.STDOUT)
                print(f"Raw result: {result}")  # Show the raw output of the command

                result = subprocess.check_output(
                    f"echo '{result}' |  sed 's/\\t/_/g; s/ /_/g; s/://g; s/\\+/\\_/g; s/,/_/g; s/[.]/_/g; s/-/_/g; s/_([^)(]*)//g; s/=//g; s/[;]//g; s/([^)(]*)//g; s/[(]//g; s/[)]//g; s/__/_/g; s/\\_\\_/\\_/g'",
                    shell=True, text=True, stderr=subprocess.STDOUT
                ).strip()

                if result:
                    new_file.write(result + "\n")
                else:
                    print(f"No result found for {term}. File not renamed.")
            except subprocess.CalledProcessError as e:
                print(f"Error processing {f}: {e}")
                print(f"Error output: {e.output}")

subprocess.run("paste OLD NEW | while read -r OLD NEW; do mv \"$OLD\" \"$NEW\"; done", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Proceed with renaming files
with open('NEW', 'r') as new_file:
    new_names = new_file.readlines()

for line in new_names:
    line = line.strip()
    new_name = f"{line}.fasta"
    if os.path.isfile(line):
        subprocess.run(f"mv {line} {new_name}", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
    else:
        print(f"File not found: {line}")

print("Creating  anvi'o databases...")
# Reformating FASTA files and create anvi'o databases
for fasta in os.listdir():
    if fasta.endswith(".fasta"):
        base_name = fasta.split(".fasta")[0]
        subprocess.run(
            f"anvi-script-reformat-fasta {fasta} -o {base_name}.fa -l 1000 --simplify-names --seq-type NT",
            shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True
        )
        subprocess.run(
            f"anvi-gen-contigs-database -f {base_name}.fa -o {base_name}.db -T {args.threads}",
            shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True
        )
        subprocess.run(
            f"anvi-run-hmms -c {base_name}.db -T {args.threads}",
            shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True
        )

# Generate external-genomes.txt
subprocess.run("ls -1 *.db > path.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("sed -i '1s/^/contigs_db_path\\n/' path.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("ls -1 *.db | sed 's/.db//' > name.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("sed -i '1s/^/contigs_db_name\\n/' name.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("paste path.txt name.txt > external-genomes.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Execute the main analysis script
subprocess.run(f"sh {rscript} {args}", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# End timing the workflow
end_time = time.time()
print(f"Workflow completed in {end_time - start_time:.2f} seconds.")
