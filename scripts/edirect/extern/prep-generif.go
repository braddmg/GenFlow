// prep-generif.go

// Public domain notice for all NCBI EDirect scripts is located at:
// https://www.ncbi.nlm.nih.gov/books/NBK179288/#chapter6.Public_Domain_Notice

package main

import (
	"bufio"
	"fmt"
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

func splitInTwoLeft(str, chr string) (string, string) {

	slash := strings.SplitN(str, chr, 2)
	if len(slash) > 1 {
		return slash[0], slash[1]
	}

	return str, ""
}

func createGeneRIF(tf, sn string) {

	transform := make(map[string]string)

	synonyms := make(map[string]string)

	readMappingTable := func(tf string, tbl map[string]string) {

		inFile, err := os.Open(tf)
		if err != nil {
			displayError("Unable to open transformation file %s - %s", tf, err.Error())
			os.Exit(1)
		}

		defer inFile.Close()

		scanr := bufio.NewScanner(inFile)

		// populate transformation map
		for scanr.Scan() {

			line := scanr.Text()
			frst, scnd := splitInTwoLeft(line, "\t")

			tbl[frst] = scnd
		}
	}

	readMappingTable(tf, transform)

	if sn != "" {
		// read optional gene synonym file
		readMappingTable(sn, synonyms)
	}

	var buffer strings.Builder
	count := 0
	okay := false

	wrtr := bufio.NewWriter(os.Stdout)

	scanr := bufio.NewScanner(os.Stdin)

	currpmid := ""

	// skip first line with column heading names
	for scanr.Scan() {

		line := scanr.Text()
		cols := strings.Split(line, "\t")
		if len(cols) != 5 {
			displayError("Unexpected number of columns (%d) in generifs_basic.gz", len(cols))
			os.Exit(1)
		}
		if len(cols) != 5 || cols[0] != "#Tax ID" {
			displayError("Unrecognized contents in generifs_basic.gz")
			os.Exit(1)
		}
		break
	}

	// read lines of PMIDs and gene references
	for scanr.Scan() {

		line := scanr.Text()

		cols := strings.Split(line, "\t")
		if len(cols) != 5 {
			continue
		}

		val := cols[2]
		pmids := strings.Split(val, ",")
		for _, pmid := range pmids {
			if currpmid != pmid {
				// end current block
				currpmid = pmid

				if pmid == "" {
					continue
				}

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

			addItemtoIndex := func(fld, val string) {

				buffer.WriteString(pmid)
				buffer.WriteString("\t")
				buffer.WriteString(fld)
				buffer.WriteString("\t")
				buffer.WriteString(val)
				buffer.WriteString("\n")
			}

			gene := cols[1]
			addItemtoIndex("GENE", gene)
			gn, ok := transform[gene]
			if ok && gn != "" {
				addItemtoIndex("GRIF", gn)
				addItemtoIndex("PREF", gn)
				addItemtoIndex("GENE", gn)
			}
			sn, ok := synonyms[gene]
			if ok && sn != "" {
				syns := strings.Split(sn, "|")
				for _, syn := range syns {
					addItemtoIndex("GSYN", syn)
					addItemtoIndex("GENE", syn)
				}
			}
		}
	}

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

	// skip past executable name
	args := os.Args[1:]

	if len(args) < 1 {
		displayError("Insufficient arguments for -generif")
		os.Exit(1)
	}

	tf := args[0]

	if tf == "" {
		displayError("Empty transformation table for -generif")
		os.Exit(1)
	}

	sn := ""
	if len(args) > 1 {
		sn = args[1]
	}

	createGeneRIF(tf, sn)
}
