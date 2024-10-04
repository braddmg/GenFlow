// ===========================================================================
//
//                            PUBLIC DOMAIN NOTICE
//            National Center for Biotechnology Information (NCBI)
//
//  This software/database is a "United States Government Work" under the
//  terms of the United States Copyright Act. It was written as part of
//  the author's official duties as a United States Government employee and
//  thus cannot be copyrighted. This software/database is freely available
//  to the public for use. The National Library of Medicine and the U.S.
//  Government do not place any restriction on its use or reproduction.
//  We would, however, appreciate having the NCBI and the author cited in
//  any work or product based on this material.
//
//  Although all reasonable efforts have been taken to ensure the accuracy
//  and reliability of the software and data, the NLM and the U.S.
//  Government do not and cannot warrant the performance or results that
//  may be obtained by using this software or data. The NLM and the U.S.
//  Government disclaim all warranties, express or implied, including
//  warranties of performance, merchantability or fitness for any particular
//  purpose.
//
// ===========================================================================
//
// File Name:  citref.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

func cit2json(query string) string {

	if query == "" {
		return ""
	}

	q := url.Values{}
	q.Add("method", "heuristic")
	q.Add("raw-text", query)
	params := q.Encode()

	base := "https://pubmed.ncbi.nlm.nih.gov/api/citmatch"
	path := fmt.Sprintf("%s?%s", base, params)

	// persistent HTTP connection by default
	resp, err := http.Get(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return ""
	}

	// client must read and close response body to keep connection alive
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return ""
	}

	jsn := string(body) + "\n"

	return jsn
}

func json2pmid(jsn string) (bool, string, string) {

	if jsn == "" {
		return false, "empty", "0"
	}

	if strings.Index(jsn, "Too Many Requests") > 1 {
		return true, "overuse", "0"
	}

	if strings.Index(jsn, "doctype") > 1 {
		return false, "failed", "0"
	}

	if strings.Index(jsn, "internal server error") > 1 {
		return false, "error", "0"
	}

	if strings.Index(jsn, "\"success\":false,") > 1 {
		return false, "failure", "0"
	}

	if strings.Index(jsn, "\"count\":1,") > 1 {
		pos := strings.Index(jsn, "pubmed")
		if pos > 1 {
			npmid := jsn[pos+9:]
			pos = strings.Index(npmid, "\"")
			if pos > 0 {
				npmid = npmid[:pos]
				if IsAllDigits(npmid) {
					value, err := strconv.Atoi(npmid)
					if err == nil && value > 0 {
						// success
						return true, "citmatch", npmid
					}
				}
			}
		}
		return true, "unrecognized", "0"
	}

	if strings.Index(jsn, "\"count\":0,") < 0 {
		return false, "unknown", "0"
	}

	return true, "unmatched", "0"
}

// Citation2PMID is a shortcut for extracting the PMID from the citMatch network service result
func Citation2PMID(query string) string {

	if query == "" {
		return ""
	}

	jsn := cit2json(query)

	ok, _, pmid := json2pmid(jsn)

	if !ok {
		// try a second time on server failure
		jsn = cit2json(query)
		_, _, pmid = json2pmid(jsn)
	}

	return pmid
}

func regenerateCitText(citFields map[string]string) string {

	if citFields == nil {
		return ""
	}

	var cmtx []string

	faut := citFields["FAUT"]
	laut := citFields["LAUT"]
	csrt := citFields["CSRT"]
	titl := citFields["TITL"]
	jour := citFields["JOUR"]
	vol := citFields["VOL"]
	iss := citFields["ISS"]
	page := citFields["PAGE"]
	year := citFields["YEAR"]

	ttl := titl
	ttl = strings.Replace(ttl, ".", "", -1)
	ttl = strings.Replace(ttl, "(", "", -1)
	ttl = strings.Replace(ttl, ")", "", -1)

	jnl := jour
	jnl = strings.Replace(jnl, ".", "", -1)
	jnl = strings.Replace(jnl, "(", "", -1)
	jnl = strings.Replace(jnl, ")", "", -1)

	pg, _ := SplitInTwoLeft(page, "-")

	// populate TEXT items
	addItem := func(val string) {
		if val != "" {
			cmtx = append(cmtx, val)
		}
	}

	addItem(faut)
	if laut != faut {
		addItem(laut)
	}
	if faut == "" && laut == "" {
		addItem(csrt)
	}
	addItem(ttl)
	addItem(jnl)
	addItem(vol)
	addItem(iss)
	addItem(pg)
	addItem(year)

	cmtxt := strings.Join(cmtx, " ")
	cmtxt = strings.TrimSpace(cmtxt)
	cmtxt = CompressRunsOfSpaces(cmtxt)

	return cmtxt
}

// NormalizeCitation moves PMID to ORIG, and creates TEXT, if not already present
func NormalizeCitation(text string) string {

	if text == "" {
		return ""
	}

	citFields := make(map[string]string)

	// stream tokens, collecting XML values in map
	StreamValues(text[:], "CITATION", func(tag, attr, content string) { citFields[tag] = content })

	// remove original TEXT field
	delete(citFields, "TEXT")

	// regenerate TEXT value
	citFields["TEXT"] = regenerateCitText(citFields)

	// collect fields in desired order
	flds := []string{"ACCN", "DIV", "REF", "FAUT", "LAUT", "CSRT", "ATHR", "TITL",
		"JOUR", "VOL", "ISS", "PAGE", "YEAR", "TEXT", "STAT", "ORIG", "PMID", "NOTE"}

	var arry []string

	for _, fld := range flds {
		nxt, ok := citFields[fld]
		if ok && nxt != "" {
			arry = append(arry, "<"+fld+">"+nxt+"</"+fld+">")
		}
	}

	body := strings.Join(arry, "")

	res := "<CITATION>" + body + "</CITATION>"

	return res
}

func genBankRefIndex(inp io.Reader, deStop, doStem bool) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create GenBank converter channel")
		os.Exit(1)
	}

	const twelvespaces = "            "
	const twentyonespaces = "                     "

	indexGenBank := func(inp io.Reader, out chan<- string) {

		// close channel when all records have been sent
		defer close(out)

		var rec strings.Builder

		var arry []string

		var cmtx []string

		scanr := bufio.NewScanner(inp)

		row := 0

		prevTitles := make(map[string]bool)

		nextLine := func() string {

			for scanr.Scan() {
				line := scanr.Text()
				if line == "" {
					continue
				}
				return line
			}
			return ""

		}

		for {
			arry = nil
			cmtx = nil

			division := ""
			dirSub := false

			// read first line of next record
			line := nextLine()
			if line == "" {
				break
			}

			row++

			for {
				if line == "" {
					break
				}
				if !strings.HasPrefix(line, "LOCUS") {
					// skip release file header information
					line = nextLine()
					row++
					continue
				}
				break
			}

			readContinuationLines := func(str string) string {

				for {
					// read next line
					line = nextLine()
					row++
					if !strings.HasPrefix(line, twelvespaces) {
						// if not continuation line, break out of loop
						break
					}
					// append subsequent line and continue with loop
					txt := strings.TrimPrefix(line, twelvespaces)
					str += " " + txt
				}

				str = CompressRunsOfSpaces(str)
				str = strings.TrimSpace(str)

				return str
			}

			writeOneElement := func(spaces, tag, value string) {

				rec.WriteString(spaces)
				rec.WriteString("<")
				rec.WriteString(tag)
				rec.WriteString(">")
				value = html.EscapeString(value)
				rec.WriteString(value)
				rec.WriteString("</")
				rec.WriteString(tag)
				rec.WriteString(">\n")
			}

			// each section will exit with the next line ready to process

			if strings.HasPrefix(line, "LOCUS") {

				// start of record

				// do not break if given artificial multi-line LOCUS
				str := readContinuationLines(line)

				cols := strings.Fields(str)
				ln := len(cols)
				if ln == 8 {
					division = cols[6]
				} else if ln == 7 {
					division = cols[5]
				}

				// read next line and continue - handled by readContinuationLines above
				// line = nextLine()
				// row++
			}

			if strings.HasPrefix(line, "DEFINITION") {
				readContinuationLines(line)
			}

			// record accession
			accn := ""

			if strings.HasPrefix(line, "ACCESSION") {

				txt := strings.TrimPrefix(line, "ACCESSION")
				str := readContinuationLines(txt)
				accessions := strings.Fields(str)
				ln := len(accessions)
				if ln > 0 {
					accn = accessions[0]
				}
			}

			if strings.HasPrefix(line, "VERSION") {

				cols := strings.Fields(line)
				if len(cols) == 2 || len(cols) == 3 {
					accn = cols[1]
				}

				// read next line and continue
				line = nextLine()
				row++
			}

			if strings.HasPrefix(line, "DBLINK") {
				readContinuationLines(line)
			}

			if strings.HasPrefix(line, "DBSOURCE") {
				readContinuationLines(line)
			}

			if strings.HasPrefix(line, "KEYWORDS") {
				readContinuationLines(line)
			}

			if strings.HasPrefix(line, "SOURCE") {
				readContinuationLines(line)
			}

			if strings.HasPrefix(line, "  ORGANISM") {

				line = nextLine()
				row++
				if strings.HasPrefix(line, twelvespaces) {
					readContinuationLines(line)
				}
			}

			// beginning of reference section
			for {
				if !strings.HasPrefix(line, "REFERENCE") {
					// exit out of reference section
					break
				}

				rec.Reset()

				rec.WriteString("  <CITATION>\n")

				if accn != "" {
					writeOneElement("    ", "ACCN", accn)
				}

				if division != "" {
					writeOneElement("    ", "DIV", division)
				}

				txt := strings.TrimPrefix(line, "REFERENCE")
				str := readContinuationLines(txt)
				str = CompressRunsOfSpaces(str)
				str = strings.TrimSpace(str)
				idx := strings.Index(str, "(")
				ref := ""
				if idx > 0 {
					ref = strings.TrimSpace(str[:idx])
				} else {
					ref = strings.TrimSpace(str)
				}
				// reference number
				writeOneElement("    ", "REF", ref)
				row++

				faut := ""
				laut := ""

				var athr []string

				if strings.HasPrefix(line, "  AUTHORS") {

					count := 0

					txt := strings.TrimPrefix(line, "  AUTHORS")
					auths := readContinuationLines(txt)

					authors := strings.Split(auths, ", ")
					for _, auth := range authors {
						auth = strings.TrimSpace(auth)
						if auth == "" {
							continue
						}
						pair := strings.Split(auth, " and ")
						for _, name := range pair {

							// convert GenBank author to searchable form
							name = GenBankToMedlineAuthors(name)

							if faut == "" {
								faut = name
							}
							laut = name

							athr = append(athr, name)
							// writeOneElement("    ", "AUTH", name)

							count++
						}
					}

					if faut != "" {
						writeOneElement("    ", "FAUT", faut)
						cmtx = append(cmtx, faut)
					}
					if laut != "" {
						writeOneElement("    ", "LAUT", laut)
						if laut != faut {
							cmtx = append(cmtx, laut)
						}
					}
					if len(athr) > 0 {
						athrs := strings.Join(athr, ", ")
						writeOneElement("    ", "ATHR", athrs)
					}
				}

				if strings.HasPrefix(line, "  CONSRTM") {

					txt := strings.TrimPrefix(line, "  CONSRTM")
					cons := readContinuationLines(txt)

					writeOneElement("    ", "CSRT", cons)
					if faut == "" && laut == "" {
						cmtx = append(cmtx, cons)
					}
				}

				inPress := false

				if strings.HasPrefix(line, "  TITLE") {

					txt := strings.TrimPrefix(line, "  TITLE")
					titl := readContinuationLines(txt)

					if titl != "" {
						writeOneElement("    ", "TITL", titl)
						ttl := titl
						ttl = strings.Replace(ttl, ".", "", -1)
						ttl = strings.Replace(ttl, "(", "", -1)
						ttl = strings.Replace(ttl, ")", "", -1)
						cmtx = append(cmtx, ttl)
					}

					if titl == "Direct Submission" {
						dirSub = true
					}
				}

				if strings.HasPrefix(line, "  JOURNAL") {

					txt := strings.TrimPrefix(line, "  JOURNAL")
					jour := readContinuationLines(txt)

					if strings.HasSuffix(jour, " In press") {
						inPress = true
					}

					if dirSub {
						if len(jour) > 23 && strings.HasPrefix(jour, "Submitted (") {
							year := jour[18:22]
							year = strings.TrimSpace(year)
							if year != "" {
								writeOneElement("    ", "YEAR", year)
							}
						}
					} else if inPress {
						jour = strings.TrimSuffix(jour, " In press")
						if strings.HasSuffix(jour, ")") {
							idx0 := strings.LastIndex(jour, "(")
							if idx0 >= 0 {
								doi := ""
								year := jour[idx0:]
								jour := jour[:idx0]
								year = strings.TrimPrefix(year, "(")
								year = strings.TrimSuffix(year, ")")
								year = strings.TrimSpace(year)
								jour = strings.TrimSpace(jour)
								idxd := strings.Index(jour, ", doi:")
								if idxd >= 0 {
									doi = jour[idxd:]
									jour = jour[:idxd]
									doi = strings.TrimPrefix(doi, ", doi:")
									jour = strings.TrimSpace(jour)
									doi = strings.TrimSpace(doi)
								}
								// truncate journal at comma
								cmma := strings.Index(jour, ",")
								if cmma >= 0 {
									jour = jour[:cmma]
								}
								// truncate journal at first digit
								for i, r := range jour {
									if unicode.IsDigit(r) {
										jour = jour[:i]
										break
									}
								}
								// check for parenthetical city, e.g., "Mol. Biol. (Mosk.)"
								if jour != "" && doi == "" && year != "" && !IsAllDigits(year) {
									year = strings.TrimSuffix(year, ".")
									jour += " (" + year + ")"
									year = ""
								}
								jour = strings.Replace(jour, "&#39;", "'", -1)
								jour = strings.Replace(jour, "'", "", -1)
								jour = strings.Replace(jour, ".", " ", -1)
								jour = strings.TrimSpace(jour)
								jour = CompressRunsOfSpaces(jour)
								if jour != "" {
									writeOneElement("    ", "JOUR", jour)
									jnl := jour
									jnl = strings.Replace(jnl, ".", "", -1)
									jnl = strings.Replace(jnl, "(", "", -1)
									jnl = strings.Replace(jnl, ")", "", -1)
									cmtx = append(cmtx, jnl)
								}
								if doi != "" {
									writeOneElement("    ", "DOI", doi)
								}
								if year != "" && IsAllDigits(year) {
									writeOneElement("    ", "YEAR", year)
									cmtx = append(cmtx, year)
								}
							}
						} else {
							// no year, just get in-press journal
							jour = strings.Replace(jour, "&#39;", "'", -1)
							jour = strings.Replace(jour, "'", "", -1)
							jour = strings.Replace(jour, ".", " ", -1)
							jour = strings.TrimSpace(jour)
							jour = CompressRunsOfSpaces(jour)
							if jour != "" {
								writeOneElement("    ", "JOUR", jour)
								jnl := jour
								jnl = strings.Replace(jnl, ".", "", -1)
								jnl = strings.Replace(jnl, "(", "", -1)
								jnl = strings.Replace(jnl, ")", "", -1)
								cmtx = append(cmtx, jnl)
							}
						}
					} else {
						journal := ""
						volume := ""
						issue := ""
						pages := ""
						year := ""
						lft, rgt := SplitInTwoLeft(jour, ",")
						if lft != "" && rgt != "" {
							if strings.HasSuffix(lft, ")") {
								idx1 := strings.LastIndex(lft, "(")
								if idx1 >= 0 {
									issue = lft[idx1:]
									lft = lft[:idx1]
									issue = strings.TrimPrefix(issue, "(")
									issue = strings.TrimSuffix(issue, ")")
									issue = strings.TrimSpace(issue)
									lft = strings.TrimSpace(lft)
								}
							}
							idx2 := strings.LastIndex(lft, " ")
							if idx2 >= 0 {
								volume = lft[idx2:]
								lft = lft[:idx2]
								volume = strings.TrimSpace(volume)
								lft = strings.TrimSpace(lft)
							}
							journal = lft
							if strings.HasSuffix(rgt, ")") {
								idx3 := strings.LastIndex(rgt, "(")
								if idx3 >= 0 {
									year = rgt[idx3:]
									rgt = rgt[:idx3]
									year = strings.TrimPrefix(year, "(")
									year = strings.TrimSuffix(year, ")")
									year = strings.TrimSpace(year)
									rgt = strings.TrimSpace(rgt)
								}
							}
							pages = rgt
							journal = strings.TrimSpace(journal)
							pages = strings.TrimSpace(pages)
							journal = strings.Replace(journal, ".", " ", -1)
							journal = strings.TrimSpace(journal)
							journal = CompressRunsOfSpaces(journal)
							if journal != "" {
								writeOneElement("    ", "JOUR", journal)
								jnl := journal
								jnl = strings.Replace(jnl, ".", "", -1)
								jnl = strings.Replace(jnl, "(", "", -1)
								jnl = strings.Replace(jnl, ")", "", -1)
								cmtx = append(cmtx, jnl)
							}
							if volume != "" {
								writeOneElement("    ", "VOL", volume)
								cmtx = append(cmtx, volume)
							}
							if issue != "" {
								writeOneElement("    ", "ISS", issue)
								cmtx = append(cmtx, issue)
							}
							if pages != "" {
								writeOneElement("    ", "PAGE", pages)
								pg, _ := SplitInTwoLeft(pages, "-")
								cmtx = append(cmtx, pg)
							}
							if year != "" {
								writeOneElement("    ", "YEAR", year)
								cmtx = append(cmtx, year)
							}
						}
					}

				}

				cmtxt := strings.Join(cmtx, " ")
				cmtxt = strings.TrimSpace(cmtxt)
				cmtxt = CompressRunsOfSpaces(cmtxt)
				if cmtxt != "" {
					// write TEXT field for citmatch service
					writeOneElement("    ", "TEXT", cmtxt)
				}

				pmid := ""
				if strings.HasPrefix(line, "   PUBMED") {

					txt := strings.TrimPrefix(line, "   PUBMED")
					pmid = readContinuationLines(txt)

					pmid = strings.TrimSpace(pmid)
				}

				stat := ""
				if dirSub {
					stat = "dirsub"
				} else if inPress {
					stat = "inpress"
				} else if pmid != "" {
					stat = "published"
				} else {
					stat = "unpub"
				}

				isDuplicate := false
				prv := cmtxt
				if prv != "" {
					if prevTitles[prv] {
						isDuplicate = true
					} else {
						prevTitles[prv] = true
					}
				}

				writeOneElement("    ", "STAT", stat)

				if strings.HasPrefix(line, "  MEDLINE") {
					// old MEDLINE uid not supported
					readContinuationLines(line)
				}

				if strings.HasPrefix(line, "  REMARK") {
					readContinuationLines(line)
				}

				if pmid != "" {
					// write PMID immediately before end of CITATION object
					writeOneElement("    ", "ORIG", pmid)
				}

				rec.WriteString("  </CITATION>")

				txt = rec.String()

				if !isDuplicate {

					// record nonredundant reference
					arry = append(arry, txt)
				}

				// reset working buffer
				rec.Reset()
				// continue to next reference
			}
			// end of reference section

			if strings.HasPrefix(line, "COMMENT") {
				readContinuationLines(line)
			}

			if strings.HasPrefix(line, "PRIMARY") {
				readContinuationLines(line)
			}

			if strings.HasPrefix(line, "FEATURES") {

				line = nextLine()
				row++

				for {
					if !strings.HasPrefix(line, "     ") {
						// exit out of features section
						break
					}
					if len(line) < 22 {
						line = nextLine()
						row++
						continue
					}

					for {
						line = nextLine()
						row++
						if !strings.HasPrefix(line, twentyonespaces) {
							break
						}
						txt := strings.TrimPrefix(line, twentyonespaces)
						if strings.HasPrefix(txt, "/") {
							// if not continuation of location, break out of loop
							break
						}
						// append subsequent line and continue with loop
					}

					for {
						if !strings.HasPrefix(line, twentyonespaces) {
							// if not qualifier line, break out of loop
							break
						}
						txt := strings.TrimPrefix(line, twentyonespaces)
						if strings.HasPrefix(txt, "/") {
							// read new qualifier and start of value

							for {
								line = nextLine()
								row++
								if !strings.HasPrefix(line, twentyonespaces) {
									break
								}
								txt := strings.TrimPrefix(line, twentyonespaces)
								if strings.HasPrefix(txt, "/") {
									// if not continuation of qualifier, break out of loop
									break
								}
								// append subsequent line to value and continue with loop
							}
						}
					}
					// end of this feature
					// continue to next feature
				}
			}
			// TSA, TLS, WGS, or CONTIG lines may be next

			altName := ""

			if strings.HasPrefix(line, "TSA") ||
				strings.HasPrefix(line, "TLS") ||
				strings.HasPrefix(line, "WGS") {

				line = line[3:]
			}

			if strings.HasPrefix(line, "WGS_CONTIG") ||
				strings.HasPrefix(line, "WGS_SCAFLD") {

				line = line[10:]
			}

			if altName != "" {

				for {
					// read next line
					line = nextLine()
					row++
					if !strings.HasPrefix(line, twelvespaces) {
						// if not continuation of contig, break out of loop
						break
					}
					// append subsequent line and continue with loop
				}
			}

			if strings.HasPrefix(line, "CONTIG") {

				for {
					// read next line
					line = nextLine()
					row++
					if !strings.HasPrefix(line, twelvespaces) {
						// if not continuation of contig, break out of loop
						break
					}
					// append subsequent line and continue with loop
				}
			}

			if strings.HasPrefix(line, "BASE COUNT") {
				readContinuationLines(line)
				// not supported
			}

			if strings.HasPrefix(line, "ORIGIN") {

				line = nextLine()
				row++
			}

			// remainder should be sequence

			for line != "" {

				if strings.HasPrefix(line, "//") {

					for _, txt := range arry {
						// send formatted record down channel
						out <- txt
					}

					// go to top of loop for next record
					break
				}

				// read next line and continue
				line = nextLine()
				row++

			}

			// continue to next record
		}
	}

	// launch single indexer goroutine
	go indexGenBank(inp, out)

	return out
}

func emblRefIndex(inp io.Reader, deStop, doStem bool) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create EMBL converter channel")
		os.Exit(1)
	}

	indexEmbl := func(inp io.Reader, out chan<- string) {

		// close channel when all records have been sent
		defer close(out)

		var rec strings.Builder

		var cmtx []string

		scanr := bufio.NewScanner(inp)

		row := 0
		line := ""

		prevTitles := make(map[string]bool)

		nextLine := func() string {

			for scanr.Scan() {
				line := scanr.Text()
				if line == "" {
					continue
				}
				return line
			}
			return ""

		}

		for {
			if line == "" {
				break
			}
			if !strings.HasPrefix(line, "ID   ") {
				// skip release file header information
				line = nextLine()
				row++
				continue
			}
			break
		}

		for {

			accn := ""

			for {

				// read next line of record
				line = nextLine()
				if line == "" {
					break
				}

				row++

				if strings.HasPrefix(line, "ID   ") {
					continue
				}

				if strings.HasPrefix(line, "XX") {
					continue
				}

				if strings.HasPrefix(line, "AC   ") {

					// read accession
					accn = strings.TrimPrefix(line, "AC   ")
					pos := strings.Index(accn, ";")
					if pos >= 0 {
						accn = accn[:pos]
					}
					accn = strings.TrimSpace(accn)
					continue
				}

				if strings.HasPrefix(line, "SV   ") {

					// accession.version overrides accession
					accn = strings.TrimPrefix(line, "SV   ")
					accn = strings.TrimSpace(accn)
					continue
				}

				for strings.HasPrefix(line, "RN   ") {

					cmtx = nil

					// start of publication
					rn := strings.TrimPrefix(line, "RN   [")
					pos := strings.Index(rn, "]")
					if pos >= 0 {
						rn = rn[:pos]
					}

					// read next line
					line = nextLine()
					row++

					writeOneElement := func(spaces, tag, value string) {

						rec.WriteString(spaces)
						rec.WriteString("<")
						rec.WriteString(tag)
						rec.WriteString(">")
						value = html.EscapeString(value)
						rec.WriteString(value)
						rec.WriteString("</")
						rec.WriteString(tag)
						rec.WriteString(">\n")
					}

					isContinuation := func(txt string) bool {

						if txt == "" {
							return false
						}
						if txt == "XX" || txt == "//" {
							return false
						}
						if len(txt) < 6 {
							return true
						}
						if txt[0] >= 'A' && txt[0] <= 'Z' &&
							txt[1] >= 'A' && txt[1] <= 'Z' &&
							txt[2] == ' ' && txt[3] == ' ' && txt[4] == ' ' {
							return false
						}
						return true
					}

					collectRefStrings := func(pfx string) string {

						if !strings.HasPrefix(line, pfx) {
							return ""
						}

						var arry []string

						str := strings.TrimPrefix(line, pfx)
						str = strings.TrimSpace(str)
						arry = append(arry, str)

						res := ""
						for {
							line = nextLine()
							row++

							if strings.HasPrefix(line, pfx) {
								str = strings.TrimPrefix(line, pfx)
								arry = append(arry, str)
							} else if isContinuation(line) {
								arry = append(arry, line)
							} else {
								res = strings.Join(arry, " ")
								break
							}
						}

						res = strings.TrimSuffix(res, ";")
						if strings.HasPrefix(res, "\"") && strings.HasSuffix(res, "\"") {
							res = strings.TrimPrefix(res, "\"")
							res = strings.TrimSuffix(res, "\"")
						}
						res = strings.TrimSpace(res)
						return res
					}

					// collect publication fields

					collectRefStrings("RP   ")
					collectRefStrings("RC   ")
					rx := collectRefStrings("RX   ")
					ra := collectRefStrings("RA   ")
					rg := collectRefStrings("RG   ")
					rt := collectRefStrings("RT   ")
					rl := collectRefStrings("RL   ")

					isolateAuthors := func(ra string) (string, string, string) {

						if ra == "" {
							return "", "", ""
						}

						ra = strings.Replace(ra, ".", "", -1)
						ra = strings.TrimSpace(ra)

						var auths []string

						auths = strings.Split(ra, ", ")
						alen := len(auths)
						if alen < 1 {
							return "", "", ""
						}

						fst := auths[0]
						lst := auths[alen-1]
						if lst == fst {
							lst = ""
						}

						return fst, lst, ra
					}

					faut, laut, athr := isolateAuthors(ra)

					pmid := ""
					if strings.HasPrefix(rx, "PubMed=") {
						rx = strings.TrimPrefix(rx, "PubMed=")
						pos := strings.Index(rx, ";")
						if pos >= 0 {
							rx = rx[:pos]
						}
						rx = strings.TrimSpace(rx)
						pmid = rx
					}

					titl := rt

					jour := ""
					vol := ""
					page := ""
					year := ""

					submitted := false
					if strings.HasPrefix(rl, "Submitted") {
						submitted = true
					} else if strings.HasSuffix(rl, ").") {
						pos := strings.LastIndex(rl, "(")
						if pos >= 0 {
							fst := rl[:pos]
							year = rl[pos:]
							year = strings.TrimPrefix(year, "(")
							year = strings.TrimSuffix(year, ").")
							pos = strings.Index(fst, ":")
							if pos >= 0 {
								pos = strings.LastIndex(fst, " ")
								if pos >= 0 {
									jour = fst[:pos]
									nxt := fst[pos+1:]
									vol, page = SplitInTwoLeft(nxt, ":")
									page, _ = SplitInTwoLeft(page, "-")
								}
							}
						}
					}

					rec.Reset()

					// if no journal, skip
					if submitted {
						continue
					}

					rec.WriteString("  <CITATION>\n")

					if accn != "" {
						writeOneElement("    ", "ACCN", accn)
					}

					writeOneElement("    ", "REF", rn)

					if faut != "" {
						writeOneElement("    ", "FAUT", faut)
						cmtx = append(cmtx, faut)
					}
					if laut != "" {
						writeOneElement("    ", "LAUT", laut)
						if laut != faut {
							cmtx = append(cmtx, laut)
						}
					}
					if athr != "" {
						writeOneElement("    ", "ATHR", athr)
					}

					if rg != "" {
						writeOneElement("    ", "CSRT", rg)
						if faut == "" && laut == "" {
							cmtx = append(cmtx, rg)
						}
					}

					if titl != "" {
						writeOneElement("    ", "TITL", titl)
						ttl := titl
						ttl = strings.Replace(ttl, ".", "", -1)
						ttl = strings.Replace(ttl, "(", "", -1)
						ttl = strings.Replace(ttl, ")", "", -1)
						cmtx = append(cmtx, ttl)
					}

					if jour != "" {
						jour = strings.Replace(jour, "&#39;", "'", -1)
						jour = strings.Replace(jour, "'", "", -1)
						jour = strings.Replace(jour, ".", " ", -1)
						jour = strings.TrimSpace(jour)
						jour = CompressRunsOfSpaces(jour)
						writeOneElement("    ", "JOUR", jour)
						jnl := jour
						jnl = strings.Replace(jnl, ".", "", -1)
						jnl = strings.Replace(jnl, "(", "", -1)
						jnl = strings.Replace(jnl, ")", "", -1)
						cmtx = append(cmtx, jnl)
					}
					if vol != "" {
						writeOneElement("    ", "VOL", vol)
						cmtx = append(cmtx, vol)
					}
					if page != "" {
						writeOneElement("    ", "PAGE", page)
						pg, _ := SplitInTwoLeft(page, "-")
						cmtx = append(cmtx, pg)
					}
					if year != "" {
						writeOneElement("    ", "YEAR", year)
						cmtx = append(cmtx, year)
					}

					cmtxt := strings.Join(cmtx, " ")
					cmtxt = strings.TrimSpace(cmtxt)
					cmtxt = CompressRunsOfSpaces(cmtxt)
					if cmtxt != "" {
						// write TEXT field for citmatch service
						writeOneElement("    ", "TEXT", cmtxt)
					}

					isDuplicate := false
					prv := cmtxt
					if prv != "" {
						if prevTitles[prv] {
							isDuplicate = true
						} else {
							prevTitles[prv] = true
						}
					}

					if pmid != "" {
						// write PMID immediately before end of CITATION object
						writeOneElement("    ", "ORIG", pmid)
					}

					rec.WriteString("  </CITATION>")

					txt := rec.String()

					if !isDuplicate {

						// record nonredundant reference
						out <- txt
					}
				}

				if strings.HasPrefix(line, "//") {

					// go to top of loop for next record
					break
				}

				// continue reading lines
			}

			line = nextLine()
			if line == "" {
				break
			}
		}
	}

	// launch single indexer goroutine
	go indexEmbl(inp, out)

	return out
}

// GenBankRefIndex reads GenBank or EMBL flatfiles and sends reference index XML records down a channel
func GenBankRefIndex(inp io.Reader, deStop, doStem bool) <-chan string {

	if inp == nil {
		return nil
	}

	const FirstBuffSize = 4096

	getFirstBlock := func() string {

		buffer := make([]byte, FirstBuffSize)
		n, err := inp.Read(buffer)
		if err != nil && err != io.EOF {
			DisplayError("Unable to read first block: %s", err.Error())
			// os.Exit(1)
		}
		bufr := buffer[:n]
		return string(bufr)
	}

	first := getFirstBlock()

	mlt := io.MultiReader(strings.NewReader(first), inp)

	isGbf := false
	isEbi := false
	matched := 0

	// auto-detect GenBank/GenPept or EBI/EMBL/UniProt format
	if first != "" {
		posG1 := strings.Index(first, "LOCUS")
		posG2 := strings.Index(first, "DEFINITION")
		if posG1 >= 0 && posG2 >= 0 && posG1 < posG2 {
			isGbf = true
			matched++
		} else {
			posG1 = FirstBuffSize
			posG2 = FirstBuffSize
		}
		posE1 := strings.Index(first, "ID  ")
		posE2 := strings.Index(first, "DE  ")
		if posE1 >= 0 && posE2 >= 0 && posE1 < posE2 {
			isEbi = true
			matched++
		} else {
			posE1 = FirstBuffSize
			posE2 = FirstBuffSize
		}
		if matched > 1 {
			if posG1 < posE1 && posG2 < posE2 {
				isEbi = false
			} else {
				isGbf = false
			}
		}
	}

	if isGbf {
		return genBankRefIndex(mlt, deStop, doStem)
	} else if isEbi {
		return emblRefIndex(mlt, deStop, doStem)
	}

	// neither format detected
	return nil
}

// CitCache structure used to prevent duplicate processing of same reference in pop/phy/mut/eco set components
type CitCache struct {
	// citation cache variables
	mlock            sync.Mutex
	matchRingList    []string
	matchResultCache map[string]string
	inUse            map[string]bool
	maximum          int
}

// NewCitCache allows server application to maintain cache over multiple calls to CreateCitMatchers
func NewCitCache(max int) *CitCache {

	// 0 defaults to keeping most recent 500 matches
	if max == 0 {
		max = 500
	}
	// -1 eliminates practical limits in the match cache
	if max == -1 {
		max = math.MaxInt32
	}

	return &CitCache{
		matchResultCache: make(map[string]string),
		inUse:            make(map[string]bool),
		maximum:          max,
	}
}

// PreloadCitCache reads -format compact CITATION XML after ref2pmid lookup
func PreloadCitCache(fileName string, cache *CitCache) {

	if fileName == "" || cache == nil {
		return
	}

	inFile, err := os.Open(fileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return
	}

	defer inFile.Close()

	// force unlimited cache size
	cache.maximum = math.MaxInt32

	scanr := bufio.NewScanner(inFile)

	addToCache := func(text string) {

		citFields := make(map[string]string)

		// stream tokens, collecting XML values in map
		StreamValues(text[:], "CITATION", func(tag, attr, content string) { citFields[tag] = content })

		// identifying string for use in citation cache
		ident := citFields["TEXT"]

		pmid := citFields["PMID"]
		note := citFields["NOTE"]

		pm := ""
		if pmid != "" {
			pm = "<PMID>" + pmid + "</PMID>"
		}
		nt := ""
		if note != "" {
			nt = "<NOTE>" + note + "</NOTE>"
		}

		suffix := pm + nt

		// cache result (cached PMID + NOTE can be empty)
		cache.matchResultCache[ident] = suffix
	}

	// loop through input lines
	for scanr.Scan() {
		line := scanr.Text()
		addToCache(line)
	}
}

// CreateCitMatchers reads CITATION XML and returns matching PMIDs
func CreateCitMatchers(inp <-chan XMLRecord, options []string, deStop, doStem bool, cache *CitCache, jtaMap map[string]string) <-chan XMLRecord {

	if inp == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create citmatch channel")
		os.Exit(1)
	}

	if cache == nil {
		cache = NewCitCache(0)
	}

	// flags from command-line arguments
	strict := false
	remote := false
	verify := false
	local := true
	verbose := false
	debug := false
	slower := false

	for _, rgs := range options {
		opts := strings.Split(rgs, ",")
		for _, opt := range opts {
			if opt == "" {
				continue
			}
			switch opt {
			case "strict":
				strict = true
			case "remote":
				remote = true
			case "verify":
				verify = true
			case "test":
				//skip verify and edirect tests
				local = false
				// citmatch only
				remote = true
			case "verbose":
				verbose = true
			case "debug":
				debug = true
			case "slow", "slower":
				slower = true
			default:
				DisplayError("Unrecognized -options choice '%s'", opt)
				os.Exit(1)
			}
		}
	}

	archiveBase := ""

	if local {

		// obtain path from environment variable
		base, _ := GetLocalArchivePaths("pubmed")

		if base == "" {

			DisplayError("Unable to get local archive paths")
			os.Exit(1)
		}

		archiveBase = base + "Archive"
		postingsBase := base + "Postings"

		// check to make sure local archive is mounted
		_, err := os.Stat(archiveBase)
		if err != nil && os.IsNotExist(err) {
			DisplayError("Local archive is not mounted")
			os.Exit(1)
		}

		// check to make sure local archive is mounted
		_, err = os.Stat(postingsBase)
		if err != nil && os.IsNotExist(err) {
			DisplayError("Local search index is not mounted")
			os.Exit(1)
		}
	}

	errataQuery := "comment on [PROP] OR erratum for [PROP] OR expression of concern for [PROP] OR reprint of [PROP] OR republished in [PROP] OR retracted and republished in [PROP] OR retraction in [PROP] OR update in [PROP]"

	toExclude := ProcessQuery("pubmed", errataQuery, false, false, false, deStop)

	isExcluded := make(map[string]bool)
	if isExcluded != nil {
		for _, item := range toExclude {
			idx := strconv.Itoa(int(item))
			isExcluded[idx] = true
		}
	}

	// gbCitMatch reads partitioned XML from channel and looks up candidate PMIDs
	gbCitMatch := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- XMLRecord) {

		// report when this matcher has no more records to process
		defer wg.Done()

		sortedWords := func(titl string) []string {

			temp := make(map[string]bool)

			titl = strings.ToLower(titl)

			// break phrases into individual words
			words := strings.FieldsFunc(titl, func(c rune) bool {
				return !unicode.IsLetter(c) && !unicode.IsDigit(c)
			})

			for _, item := range words {
				// continuing if IsStopWord i or ii here prevented correct matching of PMIDs 4411932 and 4747603 (neurotoxins I and II)
				temp[item] = true
			}

			var arry []string

			for item := range temp {
				if temp[item] {
					arry = append(arry, item)
				}
			}

			slices.Sort(arry)

			return arry
		}

		getTitle := func(uid int32) string {

			if uid < 1 {
				return ""
			}

			val := strconv.Itoa(int(uid))

			var buf bytes.Buffer
			pma := fetchOneXMLRecord(val, archiveBase, "", ".xml", true, buf)
			pma = strings.TrimSpace(pma)
			if pma == "" {
				return ""
			}

			refFields := make(map[string]string)

			StreamValues(pma[:], "PubmedArticle", func(tag, attr, content string) { refFields[tag] = content })

			titl := refFields["ArticleTitle"]

			return titl
		}

		uniqueWords := func(strs []string) []string {

			rs := make([]string, 0, len(strs))
			mp := make(map[string]bool)

			for _, val := range strs {
				_, ok := mp[val]
				if !ok {
					mp[val] = true
					rs = append(rs, val)
				}
			}

			return rs
		}

		intersectWords := func(a, b []string) int {

			temp := make(map[string]bool)
			num := 0

			for _, item := range a {
				temp[item] = true
			}

			for _, item := range b {
				if temp[item] {
					num++
				}
			}

			return num
		}

		unionWords := func(a, b []string) int {

			temp := make(map[string]bool)
			num := 0

			for _, item := range a {
				if !temp[item] {
					num++
				}
				temp[item] = true
			}

			for _, item := range b {
				if !temp[item] {
					num++
				}
				temp[item] = true
			}

			return num
		}

		// look for closest match to actual title among candidate PMIDs
		jaccard := func(titl string, ids []int32) int32 {

			if len(ids) < 1 {
				return 0
			}

			titl = NormalizeTitle(titl)
			titl = strings.ToLower(titl)
			titleWords := sortedWords(titl)
			titleWords = uniqueWords(titleWords)

			bestScore := 0
			bestPMID := int32(0)

			if debug {
				fmt.Fprintf(os.Stderr, "             %s\n", titl)
			}

			for _, uid := range ids {
				ttl := getTitle(uid)
				if ttl != "" {
					ttl = NormalizeTitle(ttl)
					ttl = strings.ToLower(ttl)
					ttlWords := sortedWords(ttl)
					ttlWords = uniqueWords(ttlWords)

					intrs := intersectWords(titleWords, ttlWords)
					union := unionWords(titleWords, ttlWords)
					score := intrs * 100 / union

					if debug {
						fmt.Fprintf(os.Stderr, "%8d %3d %s\n", uid, score, ttl)
					}

					// highest score should prefer original paper over errata and corrigenda
					if score > bestScore {
						bestScore = score
						bestPMID = uid
					}
				}
			}

			// require score of at least 60 to filter out false positives
			if bestScore < 60 {
				return 0
			}

			return bestPMID
		}

		intersectMatches := func(a, b []int32) []int32 {

			temp := make(map[int32]bool)
			var res []int32

			for _, item := range a {
				temp[item] = true
			}

			for _, item := range b {
				if temp[item] {
					res = append(res, item)
				}
			}

			return res
		}

		excludeMatches := func(a, b []int32) []int32 {

			temp := make(map[int32]bool)
			var res []int32

			for _, item := range b {
				temp[item] = true
			}

			for _, item := range a {
				if !temp[item] {
					res = append(res, item)
				}
			}

			return res
		}

		wordPairs := func(titl string) []string {

			var arry []string

			titl = strings.ToLower(titl)

			// break phrases into individual words
			words := strings.FieldsFunc(titl, func(c rune) bool {
				return !unicode.IsLetter(c) && !unicode.IsDigit(c)
			})

			// word pairs (or isolated singletons) separated by stop words
			if len(words) > 0 {
				past := ""
				run := 0
				for _, item := range words {
					if IsStopWord(item) {
						if run == 1 && past != "" {
							arry = append(arry, past)
						}
						past = ""
						run = 0
						continue
					}
					if item == "" {
						past = ""
						continue
					}
					if past != "" {
						arry = append(arry, past+" "+item)
					}
					past = item
					run++
				}
				if run == 1 && past != "" {
					arry = append(arry, past)
				}
			}

			return arry
		}

		singleWords := func(titl string) []string {

			var arry []string

			titl = strings.ToLower(titl)

			// break phrases into individual words
			words := strings.FieldsFunc(titl, func(c rune) bool {
				return !unicode.IsLetter(c) && !unicode.IsDigit(c)
			})

			for _, item := range words {
				if item == "" {
					continue
				}
				if IsStopWord(item) {
					continue
				}
				arry = append(arry, item)
			}

			return arry
		}

		/*
			wordRuns := func(titl string) []string {

				var arry []string

				titl = strings.ToLower(titl)

				words := strings.FieldsFunc(titl, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsDigit(c)
				})

				// phrases (or isolated singletons) separated by stop words
				if len(words) > 1 {
					i := 0
					j := 0
					item := ""
					for j, item = range words {
						if IsStopWord(item) {
							if j > i {
								term := strings.Join(words[i:j], " ")
								arry = append(arry, term)
							}
							i = j + 1
						}
					}
					if j > i {
						term := strings.Join(words[i:j], " ")
						arry = append(arry, term)
					}
				}

				return arry
			}
		*/

		searchTitleParts := func(terms []string, field string) []int32 {

			var candidates []int32

			if len(terms) < 1 {
				return candidates
			}

			// histogram of counts for PMIDs matching one or more word pair queries
			pmatch := make(map[int32]int)

			for _, item := range terms {
				arry := ProcessQuery("pubmed", item+" ["+field+"]", false, false, false, deStop)
				for _, uid := range arry {
					val := pmatch[uid]
					val++
					pmatch[uid] = val
				}
			}

			// find maximum number of adjacent overlapping word pair matches
			max := 0
			for _, vl := range pmatch {
				if vl > max {
					max = vl
				}
			}

			// require at least 4 matching pairs to avoid false positives
			if max < 4 {
				return candidates
			}

			// collect up to 25 PMIDs with maximum adjacent overlapping word pair count
			num := 0
			for ky, vl := range pmatch {
				if vl == max {
					candidates = append(candidates, ky)
					num++
					if num >= 25 {
						break
					}
				}
			}

			// histogram showing distribution of partial matches
			if debug {
				if num > 0 {
					top, mid, low := 0, 0, 0
					for _, vl := range pmatch {
						if vl == max {
							top++
						} else if vl == max-1 {
							mid++
						} else if vl == max-2 {
							low++
						}
					}
					sep := "matches: "
					if top > 0 {
						fmt.Fprintf(os.Stderr, "%s%d @ [%d]", sep, top, max)
						sep = ", "
					}
					if mid > 0 {
						fmt.Fprintf(os.Stderr, "%s%d @ [%d]", sep, mid, max-1)
						sep = ", "
					}
					if low > 0 {
						fmt.Fprintf(os.Stderr, "%s%d @ [%d]", sep, low, max-2)
					}
					fmt.Fprintf(os.Stderr, "\n")
				}

			}

			return candidates
		}

		matchByTitle := func(titl string) ([]int32, string) {

			var byTitle []int32

			titl = NormalizeTitle(titl)
			titl = strings.ToLower(titl)
			if debug {
				fmt.Fprintf(os.Stderr, "title:   %s\n", titl)
			}

			if titl == "" {
				return byTitle, "missing title"
			}

			// adjacent overlapping word pairs, plus single words between stop words or at ends
			pairs := wordPairs(titl)
			if len(pairs) < 1 {
				return byTitle, "title has no matching word pairs in search index"
			}

			byTitle = searchTitleParts(pairs, "PAIR")
			if len(byTitle) < 1 {
				if slower {
					words := singleWords(titl)
					if len(words) < 1 {
						return byTitle, "title has no matching words in search index"
					}

					byTitle = searchTitleParts(words, "TITL")
				}
				if len(byTitle) < 1 {
					return byTitle, "no candidates for matching title"
				}
			}

			return byTitle, ""
		}

		matchByAuthor := func(faut, laut, csrt string) ([]int32, string, string) {

			var byAuthor []int32

			faut = NormalizeAuthor(faut)
			faut = strings.ToLower(faut)

			laut = NormalizeAuthor(laut)
			laut = strings.ToLower(laut)

			if faut == "" && laut == "" && csrt == "" {
				return byAuthor, "empty authors", ""
			}

			if faut == "" && laut == "" {

				// no authors, just consortium
				csrt = NormalizeTitle(csrt)
				csrt = strings.ToLower(csrt)

				query := csrt + " [CSRT]"

				if debug {
					fmt.Fprintf(os.Stderr, "consortia: %s\n", query)
				}

				// find PMIDs indexed under consortium
				byAuthor = ProcessQuery("pubmed", query, false, false, false, deStop)
				if len(byAuthor) < 1 {
					return byAuthor, "unrecognized consortium '" + csrt + "'", csrt
				}

				return byAuthor, "", csrt
			}

			// authors present, ignore any consortium
			query := faut
			if strings.Index(faut, " ") < 0 {
				// if just last name, space plus asterisk to wildcard on initials
				query += " "
			}
			// otherwise, if space between last name and initials, immediate asterisk for term truncation
			if strict {
				query += "* [FAUT]"
			} else {
				query += "* [AUTH]"
			}
			if faut != laut && laut != "" {
				query += " OR " + laut
				if strings.Index(laut, " ") < 0 {
					query += " "
				}
				if strict {
					query += "* [LAUT]"
				} else {
					query += "* [AUTH]"
				}
			}
			if debug {
				fmt.Fprintf(os.Stderr, "authors: %s\n", query)
			}

			names := faut
			if faut != laut && laut != "" {
				names += " OR " + laut
			}

			// find PMIDs indexed under first or last author, use wildcard after truncating to single initial
			byAuthor = ProcessQuery("pubmed", query, false, false, false, deStop)
			if len(byAuthor) < 1 {
				return byAuthor, "unrecognized author '" + names + "'", names
			}

			return byAuthor, "", names
		}

		matchByJournal := func(jour string) ([]int32, string, string) {

			var byJournal []int32

			jour = NormalizeJournal(jour)
			jour = strings.ToLower(jour)

			if jour == "" {
				return byJournal, "empty journal", ""
			}

			jta, ok := jtaMap[jour]
			if !ok || jta == "" {
				return byJournal, "unmappable journal '" + jour + "'", jour
			}

			jour = NormalizeJournal(jta)
			jour = strings.ToLower(jour)

			if strings.Index(jour, "|") >= 0 {
				return byJournal, "ambiguous journal '" + jour + "'", jour
			}

			query := jour + " [JOUR]"
			if debug {
				fmt.Fprintf(os.Stderr, "journal: %s\n", query)
			}

			byJournal = ProcessQuery("pubmed", query, false, false, false, deStop)
			if len(byJournal) < 1 {
				return byJournal, "unrecognized journal '" + jour + "'", jour
			}

			return byJournal, "", jour
		}

		matchByYear := func(year string) ([]int32, string, string) {

			var byYear []int32

			if year == "" {
				return byYear, "", ""
			}

			yr, err := strconv.Atoi(year)
			if err != nil {
				return byYear, "unrecognized year '" + year + "'", year
			}
			lst := strconv.Itoa(yr - 1)
			nxt := strconv.Itoa(yr + 1)
			if strict {
				lst = strconv.Itoa(yr)
			}

			query := lst + ":" + nxt + " [YEAR]"
			if debug {
				fmt.Fprintf(os.Stderr, "year:    %s\n", query)
			}

			span := lst + ":" + nxt

			byYear = ProcessQuery("pubmed", query, false, false, false, deStop)
			if len(byYear) < 1 {
				return byYear, "unrecognized year range '" + span + "'", span
			}

			return byYear, "", span
		}

		// citFind returns PMID and optional message containing reason for failure
		citFind := func(citFields map[string]string) (int32, string) {

			if citFields == nil {
				return 0, "map missing"
			}

			note := ""
			between := ""

			// initial candidates based on most matches to overlapping word pairs in title

			titl := citFields["TITL"]

			byTitle, reasonT := matchByTitle(titl)
			if reasonT != "" {
				return 0, reasonT
			}

			// prepare postings subsets to filter candidates by author, journal, and year

			faut := citFields["FAUT"]
			laut := citFields["LAUT"]
			csrt := citFields["CSRT"]

			byAuthor, reasonA, labelA := matchByAuthor(faut, laut, csrt)
			if reasonA != "" {
				if strict {
					return 0, reasonA
				}
				note += between + reasonA
				between = ", "
			}

			jour := citFields["JOUR"]

			byJournal, reasonJ, labelJ := matchByJournal(jour)
			if reasonJ != "" {
				if strict {
					return 0, reasonJ
				}
				note += between + reasonJ
				between = ", "
			}

			year := citFields["YEAR"]

			byYear, reasonY, labelY := matchByYear(year)
			if reasonY != "" {
				if strict {
					return 0, reasonY
				}
				note += between + reasonY
				between = ", "
			}

			// interesections

			working := byTitle

			// exclude erratum for, retraction in, etc.
			if len(working) > 0 {
				temp := excludeMatches(working, toExclude)
				if len(temp) > 0 {
					working = temp
				}
			}

			// restrict by author name
			if len(byAuthor) > 0 {
				temp := intersectMatches(working, byAuthor)
				if len(temp) < 1 {
					if strict {
						return 0, "author does not match title"
					}
					note += between + "title does not match author '" + labelA + "'"
					return 0, note + ", exiting"
				}
				working = temp
			} else if strict {
				return 0, "no author match"
			}

			// restrict by journal name, but ignore if no match
			if len(byJournal) > 0 {
				temp := intersectMatches(working, byJournal)
				if len(temp) < 1 {
					if strict {
						return 0, "journal does not match title"
					}
					note += between + "title does not match journal '" + labelJ + "'"
					between = ", "
				} else {
					working = temp
				}
			} else if strict {
				return 0, "no journal match"
			}

			// restrict by year range, but ignore if no match
			if len(byYear) > 0 {
				temp := intersectMatches(working, byYear)
				if len(temp) < 1 {
					if strict {
						return 0, "year range does not match title"
					}
					note += between + "title does not match year range '" + labelY + "'"
					between = ", "
				} else {
					working = temp
				}
			} else if strict {
				return 0, "no year match"
			}

			if len(working) < 1 {
				return 0, "match not found"
			}

			// get best matching candidate
			pmid := jaccard(titl, working)
			if pmid != 0 {
				return pmid, note
			}

			note += between + "jaccard failed"
			return pmid, note
		}

		// collect citation fields, without sequence accession or reference number
		citBody := func(citFields map[string]string) string {

			var arry []string

			flds := []string{"FAUT", "LAUT", "CSRT", "ATHR", "TITL", "JOUR", "VOL", "ISS", "PAGE", "YEAR", "TEXT", "STAT", "ORIG"}

			for _, fld := range flds {
				nxt, ok := citFields[fld]
				if ok && nxt != "" {
					arry = append(arry, "<"+fld+">"+nxt+"</"+fld+">")
				}
			}

			return strings.Join(arry, "")
		}

		citPrefix := func(citFields map[string]string) string {

			var arry []string

			flds := []string{"ACCN", "DIV", "REF"}

			for _, fld := range flds {
				nxt, ok := citFields[fld]
				if ok && nxt != "" {
					arry = append(arry, "<"+fld+">"+nxt+"</"+fld+">")
				}
			}

			return strings.Join(arry, "")
		}

		pma2ref := func(pma string) map[string]string {

			refFields := make(map[string]string)

			pat := ParseRecord(pma[:], "PubmedArticle")

			var athr []string

			VisitNodes(pat, "AuthorList/Author", func(auth *XMLNode) {

				lastname := ""
				initials := ""

				VisitElements(auth, "LastName", "", func(str string) {
					lastname = str
				})

				VisitElements(auth, "Initials", "", func(str string) {
					initials = str
				})

				name := NormalizeAuthor(lastname + " " + initials)
				name = strings.ToLower(name)

				athr = append(athr, name)
			})

			ln := len(athr)
			if ln > 0 {
				refFields["FAUT"] = athr[0]
				refFields["LAUT"] = athr[ln-1]
				refFields["ATHR"] = strings.Join(athr, ", ")
			}

			VisitElements(pat, "ArticleTitle", "", func(str string) {
				str = NormalizeTitle(str)
				refFields["TITL"] = strings.ToLower(str)
			})

			VisitNodes(pat, "Article/Journal", func(jour *XMLNode) {

				VisitElements(jour, "ISOAbbreviation", "", func(str string) {
					str = NormalizeJournal(str)
					refFields["JOUR"] = strings.ToLower(str)
				})

				VisitElements(jour, "JournalIssue/Volume", "", func(str string) {
					refFields["VOL"] = strings.ToLower(str)
				})

				VisitElements(jour, "JournalIssue/Issue", "", func(str string) {
					refFields["ISS"] = strings.ToLower(str)
				})

				VisitElements(jour, "PubDate/Year", "", func(str string) {
					refFields["YEAR"] = strings.ToLower(str)
				})
			})

			VisitElements(pat, "Pagination/MedlinePgn", "", func(str string) {
				str = NormalizePage(str)
				refFields["PAGE"] = strings.ToLower(str)
			})

			return refFields
		}

		checkCitedPMID := func(citFields, refFields map[string]string) bool {

			compareFields := func(fld string, required, period bool, clean func(str string) string) bool {

				if fld == "" {
					return false
				}

				cit := citFields[fld]
				ref := refFields[fld]

				if cit != "" && ref != "" {
					if clean != nil {
						cit = clean(cit)
						ref = clean(ref)
					}

					cit = strings.ToLower(cit)
					ref = strings.ToLower(ref)

					if period {
						cit = strings.TrimSuffix(cit, ".")
						ref = strings.TrimSuffix(ref, ".")
					}

					if cit == ref {
						return true
					}

					// fmt.Fprintf(os.Stderr, "ACCN %s, FLD %s mismatch -\n'%s'\n'%s'\n\n", citFields["ACCN"], fld, cit, ref)

					// both items present but values not equal
					return false
				}

				if required {

					// fmt.Fprintf(os.Stderr, "ACCN %s, FLD %s mismatch -\n'%s'\n'%s'\n\n", citFields["ACCN"], fld, cit, ref)

					// one or both items missing but required
					return false
				}

				// both items missing but optional
				return true
			}

			if citFields["FAUT"] != "" && citFields["LAUT"] != "" {
				// first author in citation must match first author in record,
				// OR last author in citation must match last author in record
				if !compareFields("FAUT", true, false, NormalizeAuthor) && !compareFields("LAUT", true, false, NormalizeAuthor) {
					return false
				}
			} else if citFields["CSRT"] != "" {
				// consortium in citation must match consortium in record
				if !compareFields("CSRT", true, true, NormalizeTitle) {
					return false
				}
			}
			if !compareFields("TITL", true, true, NormalizeTitle) {
				// if failure, check again without non-alphanumeric characters
				relaxTitle := func(str string) string {

					if str == "" {
						return ""
					}

					str = NormalizeTitle(str)
					str = strings.ToLower(str)

					str = RelaxString(str)
					str = strings.Replace(str, " ", "", -1)

					return str
				}

				ctk := relaxTitle(citFields["TITL"])
				rfk := relaxTitle(refFields["TITL"])

				// compare compressed alphanumeric keys
				if ctk != rfk {
					return false
				}

				// compressed keys matched exactly, so continue with tests
			}
			if !compareFields("JOUR", true, false, NormalizeJournal) {
				return false
			}
			if !compareFields("VOL", false, false, nil) {
				return false
			}
			if !compareFields("ISS", false, false, nil) {
				return false
			}
			if !compareFields("PAGE", false, false, NormalizePage) {
				return false
			}
			if !compareFields("YEAR", true, false, nil) {
				// if year is present in citation, require an exact match to year in record
				return false
			}

			return true
		}

		re, _ := regexp.Compile(">[ \n\r\t]+<")

		matchCit := func(text string) string {

			citFields := make(map[string]string)

			// stream tokens, collecting XML values in map
			StreamValues(text[:], "CITATION", func(tag, attr, content string) { citFields[tag] = content })

			// identifying string for use in citation cache
			ident := citFields["TEXT"]

			// citation fields
			body := citBody(citFields)

			// current ACCN, DIV, and REF fields
			prefix := citPrefix(citFields)

			attempts := 5
			keepChecking := true

			for keepChecking && attempts > 0 {
				cache.mlock.Lock()
				// check if same citation is already in progress in another goroutine
				isInUse, _ := cache.inUse[ident]
				cache.mlock.Unlock()

				if isInUse {
					time.Sleep(time.Second)
					attempts--
				} else {
					keepChecking = false
				}
			}

			// check if same reference was processed recently
			cache.mlock.Lock()
			cachedText, ok := cache.matchResultCache[ident]
			cache.mlock.Unlock()
			if ok {
				// return cached result of previous lookup (cached PMID + NOTE can be empty)
				return "<CITATION>" + prefix + body + cachedText + "</CITATION>"
			}

			// set inUse flag for this citation
			cache.mlock.Lock()
			cache.inUse[ident] = true
			cache.mlock.Unlock()

			orig := citFields["ORIG"]

			pmid := ""
			note := ""

			if local {

				// conditionally try to verify original PMID in citation
				if orig != "" && verify && IsAllDigits(orig) {

					var buf bytes.Buffer
					pma := fetchOneXMLRecord(orig, archiveBase, "", ".xml", true, buf)
					pma = strings.TrimSpace(pma)

					refFields := pma2ref(pma)

					if checkCitedPMID(citFields, refFields) {

						if isExcluded != nil && !isExcluded[orig] {
							pmid = orig
							note = "verified"
						} else {
							note = "excluded"
						}
					}
				}

				// do citation lookup calculations
				if pmid == "" {
					pid, nte := citFind(citFields)
					if pid > 0 {
						pmid = strconv.Itoa(int(pid))
						if verbose {
							note = nte
						} else if note == "excluded" {
							if pmid == orig {
								note = "confirmed"
							} else {
								note = "replaced"
							}
						} else {
							note = "edirect"
						}
					}
				}

				if debug {
					fmt.Fprintf(os.Stderr, "pmid %s, orig %s, reason %s\n", pmid, orig, note)
				}
			}

			// conditionally try citmatch network service if local EDirect matcher failed
			if pmid == "" && remote {

				jsn := cit2json(ident)

				ok := false
				nte := ""
				ok, nte, pmid = json2pmid(jsn)

				if !ok {
					// try a second time on server failure
					jsn = cit2json(ident)
					_, nte, pmid = json2pmid(jsn)
				}

				if pmid != "" {
					if verbose {
						note = nte
					} else if note == "excluded" {
						if pmid == orig {
							note = "accepted"
						} else {
							note = "overridden"
						}
					} else {
						note = nte
					}
				}
			}

			// non-verbose note is simple - verified, edirect, citmatch, overuse, failed, unmatched
			if !verbose && pmid == "" && note == "" {
				note = "unmatched"
			}

			pm := ""
			if pmid != "" {
				pm = "<PMID>" + pmid + "</PMID>"
			}
			nt := ""
			if note != "" {
				nt = "<NOTE>" + note + "</NOTE>"
			}

			suffix := pm + nt

			res := "<CITATION>" + prefix + body + suffix + "</CITATION>"

			if re != nil {
				text = re.ReplaceAllString(res, "><")
			}

			cache.mlock.Lock()
			// clear inUse flag
			delete(cache.inUse, ident)

			// cache result (cached PMID + NOTE can be empty)
			cache.matchResultCache[ident] = suffix

			// limit cache to the most recent citation strings
			cache.matchRingList = append(cache.matchRingList, ident)
			if len(cache.matchRingList) > cache.maximum {
				// extract oldest entry from ring buffer
				oldest := cache.matchRingList[0]
				cache.matchRingList = cache.matchRingList[1:]
				// delete oldest entry from map
				delete(cache.matchResultCache, oldest)
			}
			cache.mlock.Unlock()

			// return record with newly-matched PMID (or reason for failure)
			return res
		}

		// read partitioned XML from producer channel
		for ext := range inp {

			text := ext.Text

			// rename original PMID
			text = strings.Replace(text, "<PMID>", "<ORIG>", -1)
			text = strings.Replace(text, "</PMID>", "</ORIG>", -1)

			rsult := matchCit(text[:])

			out <- XMLRecord{Index: ext.Index, Text: rsult}
		}
	}

	var wg sync.WaitGroup

	// launch multiple citmatch goroutines
	for range numServe {
		wg.Add(1)
		go gbCitMatch(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all citation matchers are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
