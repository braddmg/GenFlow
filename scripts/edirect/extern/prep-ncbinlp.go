// prep-ncbinlp.go

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

// parser character type lookup tables
var inBlank [256]bool

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

func compressRunsOfSpaces(str string) string {

	whiteSpace := false
	var buffer strings.Builder

	for _, ch := range str {
		if ch < 127 && inBlank[ch] {
			if !whiteSpace {
				buffer.WriteRune(' ')
			}
			whiteSpace = true
		} else {
			buffer.WriteRune(ch)
			whiteSpace = false
		}
	}

	return buffer.String()
}

func splitInTwoLeft(str, chr string) (string, string) {

	slash := strings.SplitN(str, chr, 2)
	if len(slash) > 1 {
		return slash[0], slash[1]
	}

	return str, ""
}

// initialize tables
func init() {

	for i := range inBlank {
		inBlank[i] = false
	}
	inBlank[' '] = true
	inBlank['\t'] = true
	inBlank['\n'] = true
	inBlank['\r'] = true
	inBlank['\f'] = true
}

func createNCBINLP(tf string) {

	transform := make(map[string]string)

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

	var buffer strings.Builder
	count := 0
	okay := false

	wrtr := bufio.NewWriter(os.Stdout)

	scanr := bufio.NewScanner(os.Stdin)

	currpmid := ""

	// read lines of PMIDs and extracted concepts
	for scanr.Scan() {

		line := scanr.Text()

		cols := strings.Split(line, "\t")
		if len(cols) != 5 {
			continue
		}

		pmid := cols[0]
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

		typ := cols[1]
		val := cols[2]
		switch typ {
		case "Gene":
			genes := strings.Split(val, ";")
			for _, gene := range genes {
				if gene == "None" {
					continue
				}
				addItemtoIndex("GENE", gene)
				gn, ok := transform[gene]
				if !ok || gn == "" {
					continue
				}
				addItemtoIndex("PREF", gn)
				addItemtoIndex("GENE", gn)
			}
		case "Disease":
			if strings.HasPrefix(val, "MESH:") {
				diszs := strings.Split(val[5:], "|")
				for _, disz := range diszs {
					addItemtoIndex("DISZ", disz)
					dn, ok := transform[disz]
					if !ok || dn == "" {
						continue
					}
					addItemtoIndex("DISZ", dn)
				}
			} else if strings.HasPrefix(val, "OMIM:") {
				omims := strings.Split(val[5:], "|")
				for _, omim := range omims {
					// was OMIM, now fused with DISZ, tag OMIM identifiers with M prefix
					addItemtoIndex("DISZ", "M"+omim)
				}
			}
		case "Chemical":
			if strings.HasPrefix(val, "MESH:") {
				chems := strings.Split(val[5:], "|")
				for _, chem := range chems {
					addItemtoIndex("CHEM", chem)
					ch, ok := transform[chem]
					if !ok || ch == "" {
						continue
					}
					addItemtoIndex("CHEM", ch)
				}
			} else if strings.HasPrefix(val, "CHEBI:") {
				chebs := strings.Split(val[6:], "|")
				for _, cheb := range chebs {
					// was CEBI, now fused with CHEM, tag CHEBI identifiers with H prefix
					addItemtoIndex("CHEM", "H"+cheb)
				}
			}
		case "Species":
		case "Mutation":
		case "CellLine":
		default:
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
		displayError("Insufficient arguments for -ncbinlp")
		os.Exit(1)
	}

	tf := args[0]

	if tf == "" {
		displayError("Empty transformation table for -ncbinlp")
		os.Exit(1)
	}

	createNCBINLP(tf)
}
