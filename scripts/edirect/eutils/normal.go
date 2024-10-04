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
// File Name:  normal.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"html"
	"os"
	"strings"
)

var combiningAccents = map[string]string{
	"Combining Grave Accent":                           "",
	"Combining Acute Accent":                           "",
	"Combining Circumflex Accent":                      "",
	"Combining Tilde":                                  "",
	"Combining Macron":                                 "",
	"Combining Overline":                               "",
	"Combining Breve":                                  "",
	"Combining Dot Above":                              "",
	"Combining Diaeresis":                              "",
	"Combining Hook Above":                             "",
	"Combining Ring Above":                             "",
	"Combining Double Acute Accent":                    "",
	"Combining Caron":                                  "",
	"Combining Vertical Line Above":                    "",
	"Combining Double Vertical Line Above":             "",
	"Combining Double Grave Accent":                    "",
	"Combining Candrabindu":                            "",
	"Combining Inverted Breve":                         "",
	"Combining Turned Comma Above":                     "",
	"Combining Comma Above":                            "",
	"Combining Reversed Comma Above":                   "",
	"Combining Comma Above Right":                      "",
	"Combining Grave Accent Below":                     "",
	"Combining Acute Accent Below":                     "",
	"Combining Left Tack Below":                        "",
	"Combining Right Tack Below":                       "",
	"Combining Left Angle Above":                       "",
	"Combining Horn":                                   "",
	"Combining Left Half Ring Below":                   "",
	"Combining Up Tack Below":                          "",
	"Combining Down Tack Below":                        "",
	"Combining Plus Sign Below":                        "",
	"Combining Minus Sign Below":                       "",
	"Combining Palatalized Hook Below":                 "",
	"Combining Retroflex Hook Below":                   "",
	"Combining Dot Below":                              "",
	"Combining Diaeresis Below":                        "",
	"Combining Ring Below":                             "",
	"Combining Comma Below":                            "",
	"Combining Cedilla":                                "",
	"Combining Ogonek":                                 "",
	"Combining Vertical Line Below":                    "",
	"Combining Bridge Below":                           "",
	"Combining Inverted Double Arch Below":             "",
	"Combining Caron Below":                            "",
	"Combining Circumflex Accent Below":                "",
	"Combining Breve Below":                            "",
	"Combining Inverted Breve Below":                   "",
	"Combining Tilde Below":                            "",
	"Combining Macron Below":                           "",
	"Combining Low Line":                               "",
	"Combining Double Low Line":                        "",
	"Combining Tilde Overlay":                          "",
	"Combining Short Stroke Overlay":                   "",
	"Combining Long Stroke Overlay":                    "",
	"Combining Short Solidus Overlay":                  "",
	"Combining Long Solidus Overlay":                   "",
	"Combining Right Half Ring Below":                  "",
	"Combining Inverted Bridge Below":                  "",
	"Combining Square Below":                           "",
	"Combining Seagull Below":                          "",
	"Combining X Above":                                "",
	"Combining Vertical Tilde":                         "",
	"Combining Double Overline":                        "",
	"Combining Grave Tone Mark":                        "",
	"Combining Acute Tone Mark":                        "",
	"Combining Greek Perispomeni":                      "",
	"Combining Greek Koronis":                          "",
	"Combining Greek Dialytika Tonos":                  "",
	"Combining Greek Ypogegrammeni":                    "",
	"Combining Bridge Above":                           "",
	"Combining Equals Sign Below":                      "",
	"Combining Double Vertical Line Below":             "",
	"Combining Left Angle Below":                       "",
	"Combining Not Tilde Above":                        "",
	"Combining Homothetic Above":                       "",
	"Combining Almost Equal To Above":                  "",
	"Combining Left Right Arrow Below":                 "",
	"Combining Upwards Arrow Below":                    "",
	"Combining Grapheme Joiner":                        "",
	"Combining Right Arrowhead Above":                  "",
	"Combining Left Half Ring Above":                   "",
	"Combining Fermata":                                "",
	"Combining X Below":                                "",
	"Combining Left Arrowhead Below":                   "",
	"Combining Right Arrowhead Below":                  "",
	"Combining Right Arrowhead And Up Arrowhead Below": "",
	"Combining Right Half Ring Above":                  "",
	"Combining Dot Above Right":                        "",
	"Combining Asterisk Below":                         "",
	"Combining Double Ring Below":                      "",
	"Combining Zigzag Above":                           "",
	"Combining Double Breve Below":                     "",
	"Combining Double Breve":                           "",
	"Combining Double Macron":                          "",
	"Combining Double Macron Below":                    "",
	"Combining Double Tilde":                           "",
	"Combining Double Inverted Breve":                  "",
	"Combining Double Rightwards Arrow Below":          "",
	"Combining Latin Small Letter A":                   "",
	"Combining Latin Small Letter E":                   "",
	"Combining Latin Small Letter I":                   "",
	"Combining Latin Small Letter O":                   "",
	"Combining Latin Small Letter U":                   "",
	"Combining Latin Small Letter C":                   "",
	"Combining Latin Small Letter D":                   "",
	"Combining Latin Small Letter H":                   "",
	"Combining Latin Small Letter M":                   "",
	"Combining Latin Small Letter R":                   "",
	"Combining Latin Small Letter T":                   "",
	"Combining Latin Small Letter V":                   "",
	"Combining Latin Small Letter X":                   "",
}

// first tier - fuse content strings within tag, suppressing all inner mixed-content tags
var fuseInPMC = map[string]bool{
	"p":              true,
	"aff":            true,
	"corresp":        true,
	"author-notes":   true,
	"disp-formula":   true,
	"mixed-citation": true,
}

// second tier - remove tag and contents
var dropInPMC = map[string]bool{
	"xref": true,
}

// third tier - remove specific internal mixed-content tags
var skipTagInPMC = map[string]bool{
	"bold":      true,
	"italic":    true,
	"sub":       true,
	"sup":       true,
	"underline": true,
	"ext-link":  true,
	"list-item": true,
}

// NormalizeXML adjusts Entrez XML fields to conform to common conventions
func NormalizeXML(rdr <-chan XMLBlock, db string) <-chan string {

	if rdr == nil || db == "" {
		return nil
	}

	// force -strict cleanup flag for most databases (even after CreateReader and CreateTokenizer are called)
	switch db {
	case "bioc", "biocollections", "clinvar", "dbvar", "gap", "gapplus", "grasp", "pccompound", "pcsubstance":
		doMixed = true
		doStrict = false
	default:
		doStrict = true
		doMixed = false
	}
	allowEmbed = true
	contentMods = true

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create normalize channel")
		os.Exit(1)
	}

	normalizeXML := func(rdr <-chan XMLBlock, out chan<- string) {

		// close channel when all chunks have been sent
		defer close(out)

		tknq := CreateTokenizer(rdr)

		if tknq == nil {
			DisplayError("Unable to create normalize tokenizer")
			os.Exit(1)
		}

		var buffer strings.Builder

		count := 0

		uid := ""
		isDocsum := false
		penultName := ""
		prevName := ""

		// mfix (in misc.go) is a *strings.Replacer for removing mixed content tags
		// rfix (in clean.go) is a *strings.Replacer that reencodes <, >, and & to &lt, &gt, and &amp

		cleanCombiningAccent := func(name string) string {

			goOn := true

			// loop to handle multiple combining accent phrases in a single string
			for goOn {

				// flank with spaces to catch unbracketed accent phrase at beginning or end
				name = " " + name + " "

				goOn = false
				if strings.Index(name, " Combining ") >= 0 {
					for ky, rep := range combiningAccents {
						pos := strings.Index(name, " "+ky+" ")
						if pos >= 0 {
							ln := len(ky) + 2
							lft := name[0:pos]
							rgt := name[pos+ln:]
							name = lft + rep + rgt
							goOn = true
						}
					}
				}
				if strings.Index(name, "[Combining ") >= 0 {
					for ky, rep := range combiningAccents {
						pos := strings.Index(name, "["+ky+"]")
						if pos >= 0 {
							ln := len(ky) + 2
							lft := name[0:pos]
							rgt := name[pos+ln:]
							name = lft + rep + rgt
							goOn = true
						}
					}
				}

				name = strings.TrimSpace(name)
				name = CompressRunsOfSpaces(name)
			}

			return name
		}

		cleanPubmedEncoding := func(name string) string {

			// convert &lt;b&gt; - &#181; - &#x...; - remove mixed content tags
			name = RemoveHTMLDecorations(name)
			// reencode < and > to avoid breaking XML
			name = encodeAngleBracketsAndAmpersand(name)

			name = strings.TrimSpace(name)
			name = CompressRunsOfSpaces(name)

			return name
		}

		for tkn := range tknq {

			tag := tkn.Tag
			name := tkn.Name
			attr := tkn.Attr

			switch tag {
			case STARTTAG:
				if name == "Id" && uid != "" {
					uid = ""
				}
				if uid != "" {
					// if object after DocumentSummary is not already Id, create Id from rescued attribute
					buffer.WriteString("<Id>\n")
					buffer.WriteString(uid)
					buffer.WriteString("\n</Id>\n")
					// clear until next docsum
					uid = ""
				}
				if name == "DocumentSummary" {
					isDocsum = true
					atts := ParseAttributes(attr)
					for i := 0; i < len(atts)-1; i += 2 {
						if atts[i] == "uid" {
							// store uid from DocumentSummary
							uid = atts[i+1]
							// if uid found, remove all attributes
							attr = ""
						}
					}
				}
				buffer.WriteString("<")
				buffer.WriteString(name)
				if attr != "" {
					attr = strings.TrimSpace(attr)
					attr = CompressRunsOfSpaces(attr)
					buffer.WriteString(" ")
					buffer.WriteString(attr)
				}
				buffer.WriteString(">\n")
				penultName = prevName
				prevName = name
			case SELFTAG:
				buffer.WriteString("<")
				buffer.WriteString(name)
				if attr != "" {
					attr = strings.TrimSpace(attr)
					attr = CompressRunsOfSpaces(attr)
					buffer.WriteString(" ")
					buffer.WriteString(attr)
				}
				buffer.WriteString("/>\n")
			case STOPTAG:
				buffer.WriteString("</")
				buffer.WriteString(name)
				buffer.WriteString(">\n")
				penultName = name
			case CONTENTTAG:
				if isDocsum {
					if db == "pubmed" && prevName == "Title" {
						if strings.ContainsAny(name, "<>&") {
							ctype := tkn.Cont
							name = CleanupContents(name, (ctype&ASCII) != 0, (ctype&AMPER) != 0, (ctype&MIXED) != 0)
							name = cleanPubmedEncoding(name)
						}
					} else if db == "pubmed" && penultName == "Author" && prevName == "Name" {
						if strings.Index(name, "Combining ") >= 0 || strings.Index(name, "[Combining ") >= 0 {
							name = cleanCombiningAccent(name)
						}
					} else if db == "gene" && prevName == "Summary" {
						if strings.Contains(name, "&amp;") {
							if HasFlankingSpace(name) {
								name = strings.TrimSpace(name)
							}
							name = html.UnescapeString(name)
							// reencode < and > to avoid breaking XML
							if strings.ContainsAny(name, "<>&") {
								name = rfix.Replace(name)
							}
						}
					} else if (db == "biosample" && prevName == "SampleData") ||
						(db == "medgen" && prevName == "ConceptMeta") ||
						(db == "sra" && prevName == "ExpXml") ||
						(db == "sra" && prevName == "Runs") {
						if strings.Contains(name, "&lt;") && strings.Contains(name, "&gt;") {
							if HasFlankingSpace(name) {
								name = strings.TrimSpace(name)
							}
							name = html.UnescapeString(name)
						}
					}
				} else {
					if db == "pubmed" {
						ctype := tkn.Cont
						name = CleanupContents(name, (ctype&ASCII) != 0, (ctype&AMPER) != 0, (ctype&MIXED) != 0)
						if HasFlankingSpace(name) {
							name = strings.TrimSpace(name)
						}
						if prevName == "ArticleTitle" || (penultName == "Abstract" && prevName == "AbstractText") {
							name = cleanPubmedEncoding(name)
						}
						if (penultName == "Author" && prevName == "LastName") ||
							(penultName == "LastName" && prevName == "ForeName") ||
							(penultName == "AffiliationInfo" && prevName == "Affiliation") {
							if strings.Index(name, "Combining ") >= 0 || strings.Index(name, "[Combining ") >= 0 {
								name = cleanCombiningAccent(name)
							}
							name = cleanPubmedEncoding(name)
						}
					} else if db == "bioc" {
						name = CleanupContents(name, true, true, true)
						if HasFlankingSpace(name) {
							name = strings.TrimSpace(name)
						}
						name = html.UnescapeString(name)
						// reencode < and > to avoid breaking XML
						if strings.ContainsAny(name, "<>&") {
							name = rfix.Replace(name)
						}
					}
				}
				// content normally printed
				if HasFlankingSpace(name) {
					name = strings.TrimSpace(name)
				}
				buffer.WriteString(name)
				buffer.WriteString("\n")
			case CDATATAG:
				if isDocsum {
					if db == "assembly" && prevName == "Meta" {
						if strings.Contains(name, "<") && strings.Contains(name, ">") {
							// if CDATA contains embedded XML, simply remove CDATA wrapper
							if HasFlankingSpace(name) {
								name = strings.TrimSpace(name)
							}
							buffer.WriteString(name)
							buffer.WriteString("\n")
						}
					} else if db == "gtr" && prevName == "Extra" {
						// remove entire CDATA contents
					}
				}
			case COMMENTTAG:
				if !isDocsum {
					if db == "sra" {
						if strings.Contains(name, "<") && strings.Contains(name, ">") {
							// if comment contains embedded XML, remove comment wrapper and trim to leading < bracket
							pos := strings.Index(name, "<")
							if pos > 0 {
								name = name[pos:]
							}
							if HasFlankingSpace(name) {
								name = strings.TrimSpace(name)
							}
							buffer.WriteString(name)
							buffer.WriteString("\n")
						}
					}
				}
			case DOCTYPETAG:
				doctype := strings.TrimSpace(name)
				if strings.HasPrefix(doctype, "<") {
					doctype = doctype[1:]
				}
				if strings.HasPrefix(doctype, "!") {
					doctype = doctype[1:]
				}
				if strings.HasPrefix(doctype, "DOCTYPE") {
					doctype = doctype[7:]
				}
				if strings.HasPrefix(doctype, " ") {
					doctype = doctype[1:]
				}
				doctype = strings.TrimSuffix(doctype, ">")
				doctype = strings.TrimSpace(doctype)

				buffer.WriteString("<!DOCTYPE ")
				buffer.WriteString(doctype)
				buffer.WriteString(">")
			case PROCESSTAG:
			case NOTAG:
			case ISCLOSED:
				// now handled at end of calling function
			default:
			}

			count++
			if count > 1000 {
				count = 0
				txt := buffer.String()
				if txt != "" {
					// send current result through output channel
					out <- txt
				}
				buffer.Reset()
			}
		}

		txt := buffer.String()
		if txt != "" {
			// send remaining result through output channel
			out <- txt
		}
	}

	// launch single normalize goroutine
	go normalizeXML(rdr, out)

	return out
}
