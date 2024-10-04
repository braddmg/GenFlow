// prep-geneinfo.go

// Public domain notice for all NCBI EDirect scripts is located at:
// https://www.ncbi.nlm.nih.gov/books/NBK179288/#chapter6.Public_Domain_Notice

package main

import (
	"bufio"
	"fmt"
	"html"
	"os"
	"strings"
	"unicode"
)

// copied from EDirect's eutils library

// ANSI escape codes for terminal color, highlight, and reverse
const (
	RED  = "\033[31m"
	BLUE = "\033[34m"
	BOLD = "\033[1m"
	RVRS = "\033[7m"
	INIT = "\033[0m"
	LOUD = INIT + RED + BOLD
	INVT = LOUD + RVRS
)

func displayError(format string, params ...any) {

	str := fmt.Sprintf(format, params...)
	fmt.Fprintf(os.Stderr, "\n%s ERROR: %s %s%s\n", INVT, LOUD, str, INIT)
}

func isAllDigits(str string) bool {

	for _, ch := range str {
		if !unicode.IsDigit(ch) {
			return false
		}
	}

	return true
}

func padNumericID(id string) string {

	if len(id) > 64 {
		return id
	}

	str := id

	if isAllDigits(str) {

		// pad numeric identifier to 8 characters with leading zeros
		ln := len(str)
		if ln < 8 {
			zeros := "00000000"
			str = zeros[ln:] + str
		}
	}

	return str
}

func createGeneInfo() {

	var buffer strings.Builder
	count := 0
	okay := false

	wrtr := bufio.NewWriter(os.Stdout)

	scanr := bufio.NewScanner(os.Stdin)

	// skip first line with column heading names
	for scanr.Scan() {

		line := scanr.Text()
		cols := strings.Split(line, "\t")
		if len(cols) != 16 {
			displayError("Unexpected number of columns (%d) in gene_info.gz", len(cols))
			os.Exit(1)
		}
		if len(cols) != 16 || cols[0] != "#tax_id" {
			displayError("Unrecognized contents in gene_info.gz")
			os.Exit(1)
		}
		break
	}

	buffer.WriteString("<Set>\n")

	// read lines of gene information
	for scanr.Scan() {

		line := scanr.Text()

		cols := strings.Split(line, "\t")
		if len(cols) != 16 {
			continue
		}

		gene := cols[2]
		// skip NEWLINE entries
		if gene == "NEWENTRY" {
			continue
		}

		id := cols[1]
		ltag := cols[3]
		syns := cols[4]
		desc := cols[8]
		auth := cols[10]

		buffer.WriteString("  <Rec>\n")

		buffer.WriteString("    <Id>" + id + "</Id>\n")
		buffer.WriteString("    <Gene>" + html.EscapeString(gene) + "</Gene>\n")

		if ltag != "-" {
			buffer.WriteString("    <Ltag>" + html.EscapeString(ltag) + "</Ltag>\n")
		}
		if syns != "-" {
			buffer.WriteString("    <Syns>" + html.EscapeString(syns) + "</Syns>\n")
		}
		if desc != "-" {
			buffer.WriteString("    <Desc>" + html.EscapeString(desc) + "</Desc>\n")
		}
		if auth != "-" {
			buffer.WriteString("    <Auth>" + html.EscapeString(auth) + "</Auth>\n")
		}

		buffer.WriteString("  </Rec>\n")

		count++

		if count >= 1000 {
			count = 0
			txt := buffer.String()
			if txt != "" {
				// print current buffer
				wrtr.WriteString(txt[:])
			}
			buffer.Reset()
		}

		okay = true
	}

	buffer.WriteString("</Set>\n")

	if okay {
		txt := buffer.String()
		if txt != "" {
			// print current buffer
			wrtr.WriteString(txt[:])
		}
	}
	buffer.Reset()

	wrtr.Flush()
}

func main() {

	createGeneInfo()
}
