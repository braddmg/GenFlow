# ANI.R
# GenFlow heatmap
############################

# Load required libraries
library(pheatmap)
library(grid)
library(viridis)
library(ggplot2)
library(reshape2)

# Check if a filename was provided
args <- commandArgs(trailingOnly = TRUE)
if (length(args) == 0) {
  stop("No input file provided. Please specify a filename.")
}

# Use the provided filename
input_file <- args[1]

# Read data from the specified file
data <- read.table(input_file, sep = "\t", header = TRUE, row.names = 1)

# Convert the data to a matrix (assuming it’s already in the correct format)
data_matrix <- as.matrix(data)

# Multiply all values by 100 for percentage
data_matrix <- data_matrix * 100

# Melt the data to long format
data_long <- melt(data_matrix)

# Custom formatting to remove decimal places for whole numbers
formatted_numbers <- apply(data_matrix, c(1, 2), function(x) {
  if (x %% 1 == 0) {
    return(as.character(as.integer(x)))  # Convert to integer and then to character
  } else {
    return(as.character(round(x, 1)))  # Round to one decimal place for non-whole numbers
  }
})

# Function to create and save heatmap
create_heatmap <- function(data_matrix, filename, color_palette) {
  heatmap_plot <- pheatmap(
    data_matrix,
    cluster_rows = TRUE,           # Cluster the rows (to maintain order)
    cluster_cols = TRUE,           # Cluster the columns
    clustering_method = "complete", # Complete linkage clustering
    clustering_distance_rows = "euclidean", # Euclidean distance for rows
    clustering_distance_cols = "euclidean", # Euclidean distance for columns
    color = color_palette,          # Use specified color palette
    display_numbers = formatted_numbers, # Use custom formatted numbers
    fontsize_number = 8,           # Increase font size of values to 8
    number_color = "black",        # Set number color to black
    cellwidth = 20,                # Increase cell width for better spacing
    cellheight = 20,               # Increase cell height for better spacing
    scale = "none",                # No scaling of ANI values
    border_color = NA,             # Remove borders around cells
    angle_col = 90,                # Rotate x-axis labels 90 degrees
    legend = TRUE,                 # Show legend
    treeheight_row = 200,          # Increase row dendrogram height for larger branches
    treeheight_col = 200,          # Increase column dendrogram height for larger branches
    show_colnames = TRUE,          # Show x-axis labels
    show_rownames = TRUE,          # Show y-axis labels
    margin = c(10, 10, 10, 10)     # Increase margins (bottom, left, top, right)
  )
  
  svg(filename, width = 18, height = 18)  # Save heatmap to a PDF with appropriate dimensions
  print(heatmap_plot)                       # Print the heatmap to the PDF device
  dev.off()                                 # Close the PDF device
}

# Create and save different heatmaps with specified color palettes
create_heatmap(data_matrix, "heatmap_v1.svg", viridis(100, option = "D"))
create_heatmap(data_matrix, "heatmap_v2.svg", colorRampPalette(c("white", "red"))(100))
create_heatmap(data_matrix, "heatmap_v3.svg", colorRampPalette(c("white", "orange", "red"))(100))
