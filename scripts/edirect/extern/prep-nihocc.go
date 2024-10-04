// prep-nihocc.go

// Public domain notice for all NCBI EDirect scripts is located at:
// https://www.ncbi.nlm.nih.gov/books/NBK179288/#chapter6.Public_Domain_Notice

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
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

func createNIHOCC(hasMaxPMID bool, maxPMID string) {

	var buffer strings.Builder
	count := 0
	okay := false

	wrtr := bufio.NewWriter(os.Stdout)

	scanr := bufio.NewScanner(os.Stdin)

	// skip first line with column heading names
	for scanr.Scan() {

		line := scanr.Text()
		if line != "citing,referenced" {
			displayError("Unrecognized header '%s' in open_citation_collection.csv", line)
			os.Exit(1)
		}
		break
	}

	// read lines of PMID link information
	for scanr.Scan() {

		line := scanr.Text()

		cols := strings.Split(line, ",")
		if len(cols) != 2 {
			continue
		}

		fst := cols[0]
		scd := cols[1]

		if fst == "0" || scd == "0" {
			continue
		}

		pdFst := padNumericID(fst)
		pdScd := padNumericID(scd)

		// string comparison works as integer comparison on left zero-padded numbers
		if hasMaxPMID {
			if pdFst > maxPMID || pdScd > maxPMID {
				continue
			}
		}

		buffer.WriteString(fst + "\tCITED\t" + pdScd + "\n")
		buffer.WriteString(scd + "\tCITES\t" + pdFst + "\n")

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

	// e.g., -nihocc 37939422
	hasMaxPMID := false
	maxPMID := "0"
	if len(args) > 0 {
		val, err := strconv.Atoi(args[0])
		if err == nil && val > 0 {
			hasMaxPMID = true
			maxPMID = args[0]
		}
	}
	maxPMID = padNumericID(maxPMID)

	createNIHOCC(hasMaxPMID, maxPMID)
}
