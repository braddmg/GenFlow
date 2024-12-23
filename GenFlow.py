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

# Base directory for the GenFlow installation
# Paths to required scripts
esearch = "/opt/GenFlow/scripts/edirect/esearch"
esummary = "/opt/GenFlow/scripts/edirect/esummary"
xtract = "/opt/GenFlow/scripts/edirect/xtract"
dataset = os.path.join("scripts/./datasets")
rscript = "/opt/GenFlow/scripts/ANI.R"


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
            try:
                esearch_cmd = f"{esearch} -db assembly -query {term}"
                esummary_cmd = f"{esummary}"
                xtract_cmd = f"{xtract} -pattern DocumentSummary -element Organism,Strain,AssemblyAccession"
                full_cmd = f"{esearch_cmd} | {esummary_cmd} | {xtract_cmd}"
                result = subprocess.check_output(full_cmd, shell=True, text=True, stderr=subprocess.STDOUT)
                result = subprocess.check_output(
                    f"echo '{result}' |  sed 's/\\t/_/g; s/ /_/g; s/://g; s/\\+/\\_/g; s/,/_/g; s/[.]/_/g; s/-/_/g; s/_([^)(]*)//g; s/=//g; s/[;]//g; s/([^)(]*)//g; s/[(]//g; s/[)]//g; s/__/_/g; s/\\_\\_/\\_/g'",
                    shell=True, text=True, stderr=subprocess.STDOUT
                ).strip()
                if result:
                    new_file.write(result + "\n")
            except subprocess.CalledProcessError as e:
                print(f"Error processing {f}: {e}")

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
subprocess.run("sed -i 's/[.]/_/g; s/-/_/g; s/,/_/g; s/ /_/g' name.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("sed -i '1s/^/name\\n/' name.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("paste name.txt path.txt > external-genomes.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("rm name.txt path.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

print("Generating pan-genome...")
# Generating the pan-genome
subprocess.run(f"anvi-gen-genomes-storage -e external-genomes.txt -o Filo-GENOMES.db", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run(f"anvi-pan-genome -g Filo-GENOMES.db --project-name Filo --num-threads {args.threads} --mcl-inflation {args.mcl_inflation} --minbit {args.minbit} --min-percent-identity {args.min_percent_identity}", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Count number of FASTA files
NG = len([f for f in os.listdir() if f.endswith(".fa")])

# Get sequences for core genes
if args.dna_mode:
    subprocess.run(f"anvi-get-sequences-for-gene-clusters -g Filo-GENOMES.db -p Filo/Filo-PAN.db -o dna-sequences.fasta --max-num-genes-from-each-genome 1 --min-num-genomes-gene-cluster-occurs {NG} --concatenate-gene-clusters --min-geometric-homogeneity-index {args.geometric_index} --min-functional-homogeneity-index {args.functional_index} --report-DNA-sequences", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
else:
    subprocess.run(f"anvi-get-sequences-for-gene-clusters -g Filo-GENOMES.db -p Filo/Filo-PAN.db -o proteins-sequences.fasta --max-num-genes-from-each-genome 1 --min-num-genomes-gene-cluster-occurs {NG} --concatenate-gene-clusters --min-geometric-homogeneity-index {args.geometric_index} --min-functional-homogeneity-index {args.functional_index}", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

print("Aligning sequences and creating phylogenomic tree...")
# Align sequences and generate phylogenomic tree
if args.dna_mode:
    subprocess.run(f"mafft --retree 1 --thread {args.threads} --maxiterate 0 dna-sequences.fasta > dna-sequences-aligned.fasta", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
    subprocess.run(f"FastTree -fastest -no2nd -gtr -nt < dna-sequences-aligned.fasta > ../results/phylogenomic-tree.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
else:
    subprocess.run(f"mafft --retree 1 --thread {args.threads} --maxiterate 0 proteins-sequences.fasta > proteins-sequences-aligned.fasta", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
    subprocess.run(f"anvi-gen-phylogenomic-tree -f proteins-sequences-aligned.fasta -o ../results/phylogenomic-tree.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

print("Performing ANI analysis...")
# Prepare input for ANI analysis
subprocess.run("ls -1 *.fa > path.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("ls -1 *.fa | sed 's/.fa//' > name.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("sed -i 's/[.]/_/g; s/-/_/g; s/,/_/g; s/ /_/g' name.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("paste path.txt name.txt > classes.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("cp classes.txt labels.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("rm name.txt path.txt", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mkdir fasta_files", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mv *.fa fasta_files", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Perform ANI analysis
subprocess.run(f"average_nucleotide_identity.py -i fasta_files -o pyANI --labels labels.txt --classes classes.txt -g --gmethod seaborn --gformat svg,png -v -l pyANI.log --workers {args.threads}", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Run the ANI.R script
subprocess.run(f"Rscript {rscript} pyANI/ANIm_percentage_identity.tab", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# Organize the outputs
subprocess.run("mv heatmap* ../results", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mv *aligned.fasta* ../results", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mkdir Anvio", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mv *.db Anvio/", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)
subprocess.run("mv Filo Anvio/", shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT, check=True)

# End time and calculate duration
end_time = time.time()
duration = end_time - start_time
hours = int(duration // 3600)
minutes = int((duration % 3600) // 60)

# Final message
print(f"Your analysis is ready, now you have some pretty phylogenomic plots.")
print(f"Time elapsed: {hours} hour(s) and {minutes} minute(s).")

