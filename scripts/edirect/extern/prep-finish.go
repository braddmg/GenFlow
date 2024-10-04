// prep-finish.go

// Public domain notice for all NCBI EDirect scripts is located at:
// https://www.ncbi.nlm.nih.gov/books/NBK179288/#chapter6.Public_Domain_Notice

package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
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

func displayWarning(format string, params ...any) {

	str := fmt.Sprintf(format, params...)
	fmt.Fprintf(os.Stderr, "\n%s WARNING: %s %s%s\n", INVT, LOUD, str, INIT)
}

func prepareIndices(chunk int, target, prefix string) {

	zipp := true

	suffix := "e2x"
	sfx := suffix
	if zipp {
		sfx += ".gz"
	}

	fnum := 0

	scanr := bufio.NewScanner(os.Stdin)

	processChunk := func() bool {

		// map for combined index
		indexed := make(map[string][]string)

		writeChunk := func() {

			var (
				fl   *os.File
				wrtr *bufio.Writer
				zpr  *gzip.Writer
				err  error
			)

			fnum++
			fpath := fmt.Sprintf("%s/%s%03d.%s", target, prefix, fnum, sfx)
			fl, err = os.Create(fpath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				return
			}
			defer fl.Close()

			pth := fmt.Sprintf("%s%03d.%s", prefix, fnum, suffix)
			os.Stderr.WriteString(pth + "\n")

			var out io.Writer

			out = fl

			if zipp {

				zpr, err = gzip.NewWriterLevel(fl, gzip.BestSpeed)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err.Error())
					return
				}

				out = zpr
			}

			wrtr = bufio.NewWriter(out)
			if wrtr == nil {
				displayWarning("Unable to create bufio.NewWriter")
				return
			}

			var buffer strings.Builder
			count := 0

			buffer.WriteString("<IdxDocumentSet>\n")

			// sort fields in alphabetical order
			keys := slices.Sorted(maps.Keys(indexed))

			for _, idx := range keys {

				item, ok := indexed[idx]
				if !ok {
					continue
				}

				uid := item[0]
				data := item[1:]

				if uid == "" || len(data) < 1 {
					continue
				}

				// do not sort data now that it has field and value pairs

				buffer.WriteString("  <IdxDocument>\n")
				buffer.WriteString("    <IdxUid>")
				buffer.WriteString(uid)
				buffer.WriteString("</IdxUid>\n")
				buffer.WriteString("    <IdxSearchFields>\n")

				prevf := ""
				prevv := ""
				for len(data) > 0 {
					fld := data[0]
					val := data[1]
					data = data[2:]

					if fld == prevf && val == prevv {
						continue
					}

					buffer.WriteString("      <")
					buffer.WriteString(fld)
					buffer.WriteString(">")
					buffer.WriteString(val)
					buffer.WriteString("</")
					buffer.WriteString(fld)
					buffer.WriteString(">\n")

					prevf = fld
					prevv = val
				}

				buffer.WriteString("    </IdxSearchFields>\n")
				buffer.WriteString("  </IdxDocument>\n")

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
			}

			buffer.WriteString("</IdxDocumentSet>\n")

			txt := buffer.String()
			if txt != "" {
				// print current buffer
				wrtr.WriteString(txt[:])
			}
			buffer.Reset()

			wrtr.Flush()

			if zpr != nil {
				zpr.Close()
			}
		}

		lineCount := 0
		okay := false

		// read lines of dependency paths, scores for each theme
		for scanr.Scan() {

			line := scanr.Text()

			cols := strings.Split(line, "\t")
			if len(cols) != 3 {
				displayWarning("Mismatched -thesis columns in '%s'", line)
				continue
			}

			uid := cols[0]
			fd := cols[1]
			val := cols[2]
			if uid == "" || fd == "" || val == "" {
				continue
			}

			val = strings.ToLower(val)
			// convert angle brackets in chemical names
			val = html.EscapeString(val)

			data, ok := indexed[uid]
			if !ok {
				data = make([]string, 0, 3)
				// first entry on new slice is uid
				data = append(data, uid)
			}
			data = append(data, fd)
			data = append(data, val)
			// always need to update indexed, since data may be reallocated
			indexed[uid] = data

			okay = true

			lineCount++
			if lineCount > chunk {
				break
			}
		}

		if okay {
			writeChunk()
			return true
		}

		return false
	}

	for processChunk() {
		// loop until scanner runs out of lines
	}
}

func main() {

	// skip past executable name
	args := os.Args[1:]

	if len(args) < 3 {
		displayError("Insufficient arguments for -thesis")
		os.Exit(1)
	}

	// e.g., 250000 "$target" "biocchem"
	chunk, err := strconv.Atoi(args[0])
	if err != nil {
		displayError("Unrecognized count - '%s'", err.Error())
		os.Exit(1)
	}
	target := strings.TrimSuffix(args[1], "/")
	prefix := args[2]

	prepareIndices(chunk, target, prefix)
}
