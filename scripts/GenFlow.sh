#!/bin/bash
# Start time
start_time=$(date +%s)

############################################################
# Help                                                     #
############################################################
Help() {
    echo "Don't worry; sometimes we also don't know what to do."
    echo
    echo "Syntax: GenFlow [-f|-h|-g|-t|-G|-F|-N]"
    echo "Options:"
    echo "-g     A txt file containing all accession numbers of reference genomes (default: genomes.txt)"
    echo "-h     Print this Help."
    echo "-f     FASTA file(s). If you have more than one, provide file names separated by commas:"
    echo "       file1.fasta,file2.fasta,etc. If you do not specify it, GenFlow will use all fasta in the folder."
    echo "-t     Number of threads—don't overwhelm your computer."
    echo "-G     Geometric Index value for selecting core genes (default: 0.8)."
    echo "-F     Functional Index value for selecting core genes (default: 0.8)."
    echo "-N     Use this option to work with nucleotide sequences instead of amino acid sequences. It requires a lot of RAM!!"
}

############################################################
# Main program                                             #
############################################################

# Set default variables
Fasta=()
threads="8"
G="0.8"
F="0.8"
genomes="genomes.txt"  # Default to genomes.txt
DNA_mode=false  # Default to not using DNA mode

# Process the input options
while getopts ":hf:g:t:G:F:N" option; do
    case $option in
        h) # Display Help
            Help
            exit;;
        f) # FASTA file(s)
            IFS=',' read -r -a Fasta <<< "$OPTARG" ;;  # Allow multiple files separated by commas
        g) # Accession numbers in txt format
            genomes="$OPTARG";;  # Set genomes file from -g argument
        t) # Number of threads
            threads="$OPTARG";;
        G) # Geometric Index
            G="$OPTARG";;
        F) # Functional Index
            F="$OPTARG";;
        N) # Enable DNA mode
            DNA_mode=true;;
        \?) # Invalid option
            echo "Error: Invalid option"
            Help
            exit;;
    esac
done

# If no FASTA files were specified, try to default to all .fasta files in the directory
if [[ ${#Fasta[@]} -eq 0 ]]; then
    Fasta=(*.fasta)
fi

# Debugging: Print the genomes file being checked
echo "Using genomes file: $genomes"

# Check if the genomes file exists
if [[ ! -f "$genomes" ]]; then
    echo "Error: File '$genomes' not found in the directory."
    exit 1
fi

# Print all FASTA files being used
echo "Using FASTA files: ${Fasta[@]}"

# Check if any FASTA files were found
if [[ ${#Fasta[@]} -eq 0 ]]; then
    echo "Error: No FASTA files found."
    exit 1
fi

# Suppress all output except errors
exec > /dev/null 2>&1

# Set up conda environment and paths
CONDA_ENV_PATH=$(conda info --base)/envs/$(basename "$CONDA_PREFIX")
dataset="$CONDA_ENV_PATH/scripts/./datasets"
esearch="$CONDA_ENV_PATH/scripts/edirect/esearch"
esummary="$CONDA_ENV_PATH/scripts/edirect/esummary"
xtract="$CONDA_ENV_PATH/scripts/edirect/xtract"

# Create logs folder
mkdir -p logs

# Download reference genomes
"$dataset" download genome accession --inputfile "$genomes" >> logs/download.log 2>&1
unzip ncbi*
rm *.zip
mkdir -p Intermediate
mv ncbi_dataset/data/GC*/*.fna Intermediate/
rm -r ncbi_dataset md5sum.txt README.md
cp "${Fasta[@]}" Intermediate
cd Intermediate || exit

# Replace names with taxa
for f in GC* ; do
    term=$(echo "$f" | cut -f1,2 -d'_')
    $esearch -db assembly -query "$term" | $esummary | \
    $xtract -pattern DocumentSummary -sep ' ' -element Organism,Sub_value,AssemblyAccession | \
    sed 's/ /_/g; s/://g; s+/+_+g; s/,/_/g; s/[.]//g; s/-/_/g; s/_([^)(]*)//; s/=//g; s/[;]//g; s/([^)(]*)//'
done > NEW

# Rename files
ls -1 *.fna > OLD
paste OLD NEW | while read -r OLD NEW; do mv "$OLD" "$NEW"; done

for i in *GCF*; do mv "$i" "$i.fasta"; done
for i in *GCA*; do mv "$i" "$i.fasta"; done

# Reformat fasta and create anvi'o databases
for i in $(ls -1 *.fasta | sed 's/.fasta//'); do
    anvi-script-reformat-fasta "$i.fasta" \
                               -o "$i.fa" \
                               -l 1000 \
                               --simplify-names --seq-type NT >> ../logs/reformat.log 2>&1
    anvi-gen-contigs-database -f "$i.fa" -o "$i.db" -T "$threads" >> ../logs/database.log 2>&1
    anvi-run-hmms -c "$i.db" -T "$threads" >> ../logs/hmms.log 2>&1    
done

# Generate external-genomes.txt
ls -1 *.db > path.txt
sed -i '1s/^/contigs_db_path\n/' path.txt
ls -1 *.db | sed 's/.db//' > name.txt
sed -i '1s/^/name\n/' name.txt
paste name.txt path.txt > external-genomes.txt
rm name.txt path.txt

# Generate genomes storage and pan-genome
anvi-gen-genomes-storage -e external-genomes.txt -o Filo-GENOMES.db >> ../logs/storage.log 2>&1
anvi-pan-genome -g Filo-GENOMES.db --project-name Filo --num-threads "$threads" >> ../logs/pangenome.log 2>&1

# Get sequences for core genes
NG=$(ls *.fa | wc -l)

if [ "$DNA_mode" = true ]; then
    anvi-get-sequences-for-gene-clusters -g Filo-GENOMES.db -p Filo/Filo-PAN.db -o dna-sequences.fasta \
                                         --max-num-genes-from-each-genome 1 \
                                         --min-num-genomes-gene-cluster-occurs "$NG" \
                                         --concatenate-gene-clusters \
                                         --min-geometric-homogeneity-index "$G" \
                                         --min-functional-homogeneity-index "$F" \
                                         --report-DNA-sequences >> ../logs/core.log 2>&1
else
    anvi-get-sequences-for-gene-clusters -g Filo-GENOMES.db -p Filo/Filo-PAN.db -o proteins-sequences.fasta \
                                         --max-num-genes-from-each-genome 1 \
                                         --min-num-genomes-gene-cluster-occurs "$NG" \
                                         --concatenate-gene-clusters \
                                         --min-geometric-homogeneity-index "$G" \
                                         --min-functional-homogeneity-index "$F" >> ../logs/core.log 2>&1
fi

mkdir -p ../results

# Align sequences and generate phylogenomic tree
if [ "$DNA_mode" = true ]; then
    mafft --retree 1 --thread "$threads" --maxiterate 0 dna-sequences.fasta > dna-sequences-aligned.fasta 2>> ../logs/maft.log
    FastTree -gtr -nt < dna-sequences-aligned.fasta > ../results/phylogenomic-tree.txt 2>> ../logs/tree.log
else
    mafft --retree 1 --thread "$threads" --maxiterate 0 proteins-sequences.fasta > proteins-sequences-aligned.fasta 2>> ../logs/maft.log
    anvi-gen-phylogenomic-tree -f proteins-sequences-aligned.fasta \
                               -o ../results/phylogenomic-tree.txt 2>> ../logs/tree.log
fi

# Prepare input for ANI analysis
ls -1 *.fa > path.txt
ls -1 *.fa | sed 's/.fa//' > name.txt
sed -i 's/[.]/_/g; s/-/_/g; s/,/_/g; s/ /_/g' name.txt
paste path.txt name.txt > classes.txt
cp classes.txt labels.txt
rm name.txt path.txt
mkdir fa
mv *.fa fa

# Perform ANI analysis
average_nucleotide_identity.py -i fa \
                               -o ../results/pyANI \
                               --labels labels.txt \
                               --classes classes.txt \
                               -g --gmethod seaborn --gformat svg,png -v -l pyANI.log >> ../logs/pyANI.log 2>&1

mv *aligned.fasta* ../results
rm *.txt
rm *.fasta
# End time and calculate duration
end_time=$(date +%s)
duration=$((end_time - start_time))
hours=$((duration / 3600))
minutes=$(((duration % 3600) / 60))
exec > /dev/tty 2>&1

# Final message
echo "Your analysis is ready, now you have some pretty phylogenomic plots."
echo "Time elapsed: ${hours} hour(s) and ${minutes} minute(s)."
