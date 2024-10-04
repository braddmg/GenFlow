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
// File Name:  xplore.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"encoding/base64"
	"fmt"
	"github.com/fatih/color"
	"github.com/surgebase/porter2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"html"
	"maps"
	"math"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// SUPPORT CODE FOR xtract DATA EXTRACTION

var (
	rlock sync.Mutex
	replx map[string]*regexp.Regexp
)

// Clause consolidates a dozen function arguments into a single structure
type Clause struct {
	Prev string
	Pfx  string
	Sfx  string
	Plg  string
	Sep  string
	Def  string
	Msk  string
	Reg  string
	Exp  string
	Wth  string
	Gcd  int
	Frm  int
	Wrp  bool
}

// processClause handles comma-separated -element arguments
func processClause(
	curr *XMLNode,
	stages []*Step,
	mask string,
	cls Clause,
	status OpType,
	index int,
	level int,
	variables map[string]string,
	transform map[string]string,
	srchr *FSMSearcher,
	histogram map[string]int,
) (string, bool) {

	if curr == nil || stages == nil {
		return "", false
	}

	pfx := cls.Pfx
	sfx := cls.Sfx
	sep := cls.Sep
	reg := cls.Reg
	exp := cls.Exp
	wrp := cls.Wrp
	msk := cls.Msk

	if replx == nil {
		rlock.Lock()
		if replx == nil {
			replx = make(map[string]*regexp.Regexp)
		}
		rlock.Unlock()
	}

	// field for -indexer derived from -pfx argument set by -wrp
	indexerField := ""

	if status == INDEXER {
		if strings.HasPrefix(pfx, "<") && strings.HasSuffix(pfx, ">") && !strings.Contains(pfx, "/") {
			// take label from -pfx argument minus the angle brackets added by -wrp
			indexerField = strings.TrimPrefix(pfx, "<")
			indexerField = strings.TrimSuffix(indexerField, ">")
			indexerField = strings.TrimSpace(indexerField)
		}
		// clear -wrp derived arguments (even if they weren't set by -wrp)
		pfx = ""
		sfx = ""
		sep = ""
	}

	// processElement handles individual -element constructs
	processElement := func(acc func(string)) {

		if acc == nil {
			return
		}

		// element names combined with commas are treated as a prefix-separator-suffix group
		for _, stage := range stages {

			stat := stage.Type
			item := stage.Value
			prnt := stage.Parent
			match := stage.Match
			attrib := stage.Attrib
			typL := stage.TypL
			strL := stage.StrL
			intL := stage.IntL
			typR := stage.TypR
			strR := stage.StrR
			intR := stage.IntR
			norm := stage.Norm
			wildcard := stage.Wild
			unescape := stage.Unesc

			// exploreElements is a wrapper for ExploreElements, inheriting most common arguments as closures
			exploreElements := func(proc func(string, int)) {
				ExploreElements(curr, mask, prnt, match, attrib, wildcard, unescape, level, proc)
			}

			// sendSlice applies optional [min:max] range restriction and sends result to accumulator
			sendSlice := func(str string) {

				// handle usual situation with no range first
				if norm {
					if wrp && stat != REPLACE {
						str = html.EscapeString(str)
					}
					acc(str)
					return
				}

				// check for [after|before] variant
				if typL == STRINGRANGE || typR == STRINGRANGE {
					if strL != "" {
						// use case-insensitive test
						strL = strings.ToUpper(strL)
						idx := strings.Index(strings.ToUpper(str), strL)
						if idx < 0 {
							// specified substring must be present in original string
							return
						}
						ln := len(strL)
						// remove leading text
						str = str[idx+ln:]
					}
					if strR != "" {
						strR = strings.ToUpper(strR)
						idx := strings.Index(strings.ToUpper(str), strR)
						if idx < 0 {
							// specified substring must be present in remaining string
							return
						}
						// remove trailing text
						str = str[:idx]
					}
					if str != "" {
						if wrp && stat != REPLACE {
							str = html.EscapeString(str)
						}
						acc(str)
					}
					return
				}

				min := 0
				max := 0

				// slice arguments use variable value +- adjustment or integer constant
				if typL == VARIABLERANGE {
					if strL == "" {
						return
					}
					lft, ok := variables[strL]
					if !ok {
						return
					}
					val, err := strconv.Atoi(lft)
					if err != nil {
						return
					}
					// range argument values are inclusive and 1-based, decrement variable start +- offset to use in slice
					min = val + intL - 1
				} else if typL == INTEGERRANGE {
					// range argument values are inclusive and 1-based, decrement literal start to use in slice
					min = intL - 1
				}
				if typR == VARIABLERANGE {
					if strR == "" {
						return
					}
					rgt, ok := variables[strR]
					if !ok {
						return
					}
					val, err := strconv.Atoi(rgt)
					if err != nil {
						return
					}
					if val+intR < 0 {
						// negative value is 1-based inset from end of string (undocumented)
						max = len(str) + val + intR + 1
					} else {
						max = val + intR
					}
				} else if typR == INTEGERRANGE {
					if intR < 0 {
						// negative max is inset from end of string (undocumented)
						max = len(str) + intR + 1
					} else {
						max = intR
					}
				}

				doRevComp := false
				doUpCase := false
				if status == NUCLEIC {
					// -nucleic uses direction of range to decide between forward strand or reverse complement
					if min+1 > max {
						min, max = max-1, min+1
						doRevComp = true
					}
					doUpCase = true
				}

				// numeric range now calculated, apply slice to string
				if min == 0 && max == 0 {
					if doRevComp {
						str = ReverseComplement(str)
					}
					if doUpCase {
						str = strings.ToUpper(str)
					}
					if wrp && stat != REPLACE {
						str = html.EscapeString(str)
					}
					acc(str)
				} else if max == 0 {
					if min > 0 && min < len(str) {
						str = str[min:]
						if str != "" {
							if doRevComp {
								str = ReverseComplement(str)
							}
							if doUpCase {
								str = strings.ToUpper(str)
							}
							if wrp && stat != REPLACE {
								str = html.EscapeString(str)
							}
							acc(str)
						}
					}
				} else if min == 0 {
					if max > 0 && max <= len(str) {
						str = str[:max]
						if str != "" {
							if doRevComp {
								str = ReverseComplement(str)
							}
							if doUpCase {
								str = strings.ToUpper(str)
							}
							if wrp && stat != REPLACE {
								str = html.EscapeString(str)
							}
							acc(str)
						}
					}
				} else {
					if min < max && min > 0 && max <= len(str) {
						str = str[min:max]
						if str != "" {
							if doRevComp {
								str = ReverseComplement(str)
							}
							if doUpCase {
								str = strings.ToUpper(str)
							}
							if wrp && stat != REPLACE {
								str = html.EscapeString(str)
							}
							acc(str)
						}
					}
				}
			}

			switch stat {
			case ELEMENT:
				exploreElements(func(str string, lvl int) {
					if str != "" {
						sendSlice(str)
					}
				})
			case VARIABLE, ACCUMULATOR, CONVERTER:
				// use value of stored variable
				val, ok := variables[match]
				if ok {
					sendSlice(val)
				}
			case NUM, COUNT:
				// common code for -num command (NUM) or "#" object prefix (COUNT), allows counting of container objects
				count := 0

				exploreElements(func(str string, lvl int) {
					count++
				})

				// number of element objects
				val := strconv.Itoa(count)
				acc(val)
			case LENGTH:
				// code only for "%" object prefix (LENGTH), but not for -len command (LEN),
				// sacrifices getting same result even with doubly-specified -len "%object"
				// in order to restore more important use of -len "*" to get length of XML
				length := 0

				exploreElements(func(str string, lvl int) {
					length += len(str)
				})

				// length of element strings
				val := strconv.Itoa(length)
				acc(val)
			case DEPTH:
				if match == "*" {
					// ^* construct visits all objects in scope
					maxlvl := 0
					ExploreNodes(curr, "**", "*", 0, 0, func(crr *XMLNode, idx, lvl int) {
						if lvl > maxlvl {
							maxlvl = lvl
						}
					})
					// to get maximum depth value
					val := strconv.Itoa(maxlvl)
					acc(val)
				} else {
					// otherwise print depth of each element in scope
					exploreElements(func(str string, lvl int) {
						val := strconv.Itoa(lvl)
						acc(val)
					})
				}
			case INDEX:
				// -element "+" prints index of current XML object
				val := strconv.Itoa(index)
				acc(val)
			case INCR:
				// component of -0-based, -1-based, or -ucsc-based, not used for -inc
				exploreElements(func(str string, lvl int) {
					if str != "" {
						num, err := strconv.Atoi(str)
						if err == nil {
							// increment value
							num++
							val := strconv.Itoa(num)
							acc(val)
						}
					}
				})
			case DECR:
				// component of -0-based, -1-based, or -ucsc-based, not used for -dec
				exploreElements(func(str string, lvl int) {
					if str != "" {
						num, err := strconv.Atoi(str)
						if err == nil {
							// decrement value
							num--
							val := strconv.Itoa(num)
							acc(val)
						}
					}
				})
			case QUESTION:
				acc(curr.Name)
			case TILDE:
				acc(curr.Contents)
			case DOT:
				// -element "." prints current XML subtree as ASN.1
				var buffer strings.Builder

				printASNtree(curr,
					func(str string) {
						if str != "" {
							buffer.WriteString(str)
						}
					})

				txt := buffer.String()
				if txt != "" {
					if strings.HasSuffix(txt, "\n") {
						txt = strings.TrimSuffix(txt, "\n")
					}
					acc(txt)
				}
			case PRCNT:
				// -element "%" prints current XML subtree as JSON
				var buffer strings.Builder

				printJSONtree(curr,
					func(str string) {
						if str != "" {
							buffer.WriteString(str)
						}
					})

				txt := buffer.String()
				if txt != "" {
					if strings.HasSuffix(txt, "\n") {
						txt = strings.TrimSuffix(txt, "\n")
					}
					acc(txt)
				}
			case STAR:
				// -element "*" prints current XML subtree on a single line
				style := SINGULARITY
				printAttrs := true

				for _, ch := range item {
					if ch == '*' {
						style++
					} else if ch == '@' {
						printAttrs = false
					}
				}
				if style > WRAPPED {
					style = WRAPPED
				}
				if style < COMPACT {
					style = COMPACT
				}

				var buffer strings.Builder

				printXMLtree(curr, style, msk, printAttrs,
					func(str string) {
						if str != "" {
							buffer.WriteString(str)
						}
					})

				txt := buffer.String()
				if txt != "" {
					acc(txt)
				}
			case DOLLAR:
				for chld := curr.Children; chld != nil; chld = chld.Next {
					acc(chld.Name)
				}
			case ATSIGN:
				if curr.Attributes != "" && curr.Attribs == nil {
					curr.Attribs = ParseAttributes(curr.Attributes)
				}
				for i := 0; i < len(curr.Attribs)-1; i += 2 {
					acc(curr.Attribs[i])
				}
			default:
				exploreElements(func(str string, lvl int) {
					if str != "" {
						sendSlice(str)
					}
				})
			}
		}
	}

	ok := false

	// format results in buffer
	var buffer strings.Builder

	buffer.WriteString(cls.Prev)
	buffer.WriteString(cls.Plg)
	buffer.WriteString(pfx)
	between := ""

	switch status {
	case ELEMENT:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case FIRST:
		single := ""

		processElement(func(str string) {
			ok = true
			if single == "" {
				single = str
			}
		})

		if single != "" {
			buffer.WriteString(between)
			buffer.WriteString(single)
			between = sep
		}

	case LAST:
		single := ""

		processElement(func(str string) {
			ok = true
			single = str
		})

		if single != "" {
			buffer.WriteString(between)
			buffer.WriteString(single)
			between = sep
		}

	case EVEN:
		even := false

		processElement(func(str string) {
			if str != "" {
				if even {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
				even = !even
			}
		})

	case ODD:
		odd := true

		processElement(func(str string) {
			if str != "" {
				if odd {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
				odd = !odd
			}
		})

	case BACKWARD:
		var arry []string

		processElement(func(str string) {
			if str != "" {
				ok = true
				arry = append(arry, str)
			}
		})

		if ok {
			for i := len(arry) - 1; i >= 0; i-- {
				buffer.WriteString(between)
				buffer.WriteString(arry[i])
				between = sep
			}
		}

	case ENCODE:
		processElement(func(str string) {
			if str != "" {
				ok = true
				if !wrp {
					str = html.EscapeString(str)
				}
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case DECODE:
		// equivalent to "transmute -decode64"
		processElement(func(str string) {
			if str != "" {
				txt, err := base64.StdEncoding.DecodeString(str)
				if err == nil {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(string(txt))
					between = sep
				}
			}
		})

	case UPPER:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = strings.ToUpper(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case LOWER:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = strings.ToLower(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case CHAIN:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = strings.Replace(str, " ", "_", -1)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case TITLE:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = strings.ToLower(str)
				// str = strings.Title(str)
				csr := cases.Title(language.English)
				str = csr.String(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case MIRROR:
		processElement(func(str string) {
			if str != "" {
				ok = true
				rev := ReverseString(str)
				buffer.WriteString(between)
				buffer.WriteString(rev)
				between = sep
			}
		})

	case ALPHA:
		processElement(func(str string) {
			if str != "" {
				// split at non-alphabetic characters
				words := strings.FieldsFunc(str, func(c rune) bool {
					return (!unicode.IsLetter(c)) || c > 127
				})
				str = strings.Join(words, " ")
				str = strings.TrimSpace(str)
				str = CompressRunsOfSpaces(str)
				if str != "" {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
			}
		})

	case ALNUM:
		processElement(func(str string) {
			if str != "" {
				// split at non-alphanumeric characters
				words := strings.FieldsFunc(str, func(c rune) bool {
					return (!unicode.IsLetter(c) && !unicode.IsDigit(c)) || c > 127
				})
				str = strings.Join(words, " ")
				str = strings.TrimSpace(str)
				str = CompressRunsOfSpaces(str)
				if str != "" {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
			}
		})

	case BASIC:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = CleanupBasic(str, wrp)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case PLAIN:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = CleanupPlain(str, wrp)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case SIMPLE:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = CleanupSimple(str, wrp)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case AUTHOR:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = CleanupAuthor(str, wrp)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case PROSE:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = CleanupProse(str, wrp)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case JOURNAL:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = NormalizeJournal(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case YEAR:
		year := ""

		processElement(func(str string) {
			if str != "" && year == "" {
				words := strings.FieldsFunc(str, func(c rune) bool {
					return !unicode.IsDigit(c)
				})
				for _, item := range words {
					if len(item) == 4 && IsAllDigits(item) {
						year = item
						ok = true
						// only print integer for first year, e.g., PubDate/MedlineDate "2008 Dec-2009 Jan" is 2008
						break
					}
				}
			}
		})

		if year != "" {
			buffer.WriteString(between)
			buffer.WriteString(year)
			between = sep
		}

	case MONTH:
		month := ""

		processElement(func(str string) {
			if str != "" && month == "" {
				words := strings.FieldsFunc(str, func(c rune) bool {
					return !unicode.IsLetter(c)
				})
				for _, item := range words {
					item = strings.ToLower(item)
					val, found := monthTable[item]
					if found {
						month = strconv.Itoa(val)
						ok = true
						// only print integer for first month, e.g., PubDate/MedlineDate "2008 Dec-2009 Jan" is 12
						break
					}
				}
			}
		})

		if month != "" {
			buffer.WriteString(between)
			buffer.WriteString(month)
			between = sep
		}

	case DATE:
		// xtract -pattern PubmedArticle -unit "PubDate" -date "*"
		// xtract -pattern collection -unit date -date "*"
		year := ""
		month := ""
		day := ""

		extractBetweenTags := func(txt, tag string) string {

			if txt == "" || tag == "" {
				return ""
			}
			_, after, found := strings.Cut(txt, "<"+tag+">")
			if !found || after == "" {
				return ""
			}
			res, _, found := strings.Cut(after, "</"+tag+">")
			if !found || res == "" {
				return ""
			}
			return res
		}

		processElement(func(str string) {
			if str != "" {
				if strings.Contains(str, "MedlineDate") {

					words := strings.FieldsFunc(str, func(c rune) bool {
						return !unicode.IsDigit(c)
					})
					for _, item := range words {
						if len(item) == 4 && IsAllDigits(item) {
							year = item
							// only print integer for first year
							break
						}
					}
					if year != "" {
						words := strings.FieldsFunc(str, func(c rune) bool {
							return !unicode.IsLetter(c)
						})
						for _, item := range words {
							item = strings.ToLower(item)
							val, found := monthTable[item]
							if found {
								month = strconv.Itoa(val)
								// only print integer for first month
								break
							}
						}
					}

				} else if strings.Contains(str, "PubMedPubDate") || strings.Contains(str, "PubDate") {

					year = extractBetweenTags(str, "Year")
					month = extractBetweenTags(str, "Month")
					if month != "" {
						if !IsAllDigits(month) {
							month = strings.ToLower(month)
							val, found := monthTable[month]
							if found {
								month = strconv.Itoa(val)
							}
						}
					}
					day = extractBetweenTags(str, "Day")

				} else if strings.Contains(str, "date") {

					str = html.UnescapeString(str)
					// <date>20201214</date>
					raw := extractBetweenTags(str, "date")
					if len(raw) == 8 {
						year = raw[0:4]
						month = raw[4:6]
						day = raw[6:8]
					} else if len(raw) == 6 {
						year = raw[0:4]
						month = raw[4:6]
					} else if len(raw) == 4 {
						year = raw[0:4]
					}

				} else {

					year = extractBetweenTags(str, "Year")
					month = extractBetweenTags(str, "Month")
					if month != "" {
						if !IsAllDigits(month) {
							month = strings.ToLower(month)
							val, found := monthTable[month]
							if found {
								month = strconv.Itoa(val)
							}
						}
					}
					day = extractBetweenTags(str, "Day")

					/*
						str = extractBetweenTags(str, "PubDate")
						items := strings.Split(str, " ")
						for _, itm := range items {
							if year == "" {
								year = itm
							} else if month == "" {
								month = itm
							} else if day == "" {
								day = itm
							}
						}
						if month != "" {
							if !IsAllDigits(month) {
								month = strings.ToLower(month)
								val, found := monthTable[month]
								if found {
									month = strconv.Itoa(val)
								}
							}
						}
					*/
				}
			}
		})

		slash := "/"
		if reg == "/" && exp != "" {
			slash = exp
		}

		txt := ""
		if year != "" {
			buffer.WriteString(between)
			txt = year
			if month != "" {
				if len(month) == 1 {
					txt += slash + "0" + month
				} else {
					txt += slash + month
				}
				if day != "" {
					if len(day) == 1 {
						txt += slash + "0" + day
					} else {
						txt += slash + day
					}
				}
			}
			ok = true
		}

		if txt != "" {
			buffer.WriteString(between)
			buffer.WriteString(txt)
			between = sep
		}

	case PAGE:
		processElement(func(str string) {
			if str != "" {
				words := strings.FieldsFunc(str, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsDigit(c)
				})
				if len(words) > 0 {
					firstPage := words[0]
					if firstPage != "" {
						ok = true
						buffer.WriteString(between)
						buffer.WriteString(firstPage)
						between = sep
					}
				}
			}
		})

	case AUTH:
		processElement(func(str string) {
			if str != "" {
				ok = true
				// convert GenBank author to searchable form
				str = GenBankToMedlineAuthors(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case INITIALS:
		processElement(func(str string) {
			if str != "" {
				ok = true
				// convert given name to initials
				if len(str) != 2 || !unicode.IsUpper(rune(str[0])) || !unicode.IsUpper(rune(str[1])) {
					lft, rgt, found := strings.Cut(str, " ")
					if !found {
						lft, rgt, found = strings.Cut(str, "-")
					}
					if !found {
						lft, rgt, found = strings.Cut(str, ".")
					}
					if found && lft != "" && rgt != "" {
						str = lft[:1] + rgt[:1]
					} else {
						str = str[:1]
					}
				}
				str = strings.ToUpper(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case PROP:
		processElement(func(str string) {
			if str != "" {
				prop, fnd := propertyTable[str]
				if fnd {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(prop)
					between = sep
				} else {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString("Other")
					between = sep
				}
			}
		})

	case TRIM:
		processElement(func(str string) {
			if str != "" {
				str = strings.TrimPrefix(str, " ")
				str = strings.TrimSuffix(str, " ")
				str = strings.TrimSpace(str)
				if strings.HasPrefix(str, "0") {
					// also trim leading zeros
					str = strings.TrimPrefix(str, "0")
					if str == "" {
						// but leave one if was only zeros
						str = "0"
					}
				}
				if str != "" {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
			}
		})

	case WCT:
		count := 0

		processElement(func(str string) {
			if str != "" {

				words := strings.FieldsFunc(str, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsDigit(c)
				})
				for _, item := range words {
					item = strings.ToLower(item)
					if deStop {
						// exclude stop words from count
						if IsStopWord(item) {
							continue
						}
					}
					if doStem {
						item = porter2.Stem(item)
						item = strings.TrimSpace(item)
					}
					if item == "" {
						continue
					}
					count++
					ok = true
				}
			}
		})

		if ok {
			// total number of words
			val := strconv.Itoa(count)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case DOI:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = strings.TrimPrefix(str, "doi:")
				str = strings.TrimSpace(str)
				str = strings.TrimPrefix(str, "/")
				str = strings.TrimPrefix(str, "https://doi.org/")
				str = strings.TrimPrefix(str, "http://dx.doi.org/")
				str = url.QueryEscape(str)
				str = "https://doi.org/" + str
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case TRANSLATE:
		processElement(func(str string) {
			if str != "" {
				txt, found := transform[str]
				if found {
					// require successful mapping
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(txt)
					between = sep
				}
			}
		})

	case REPLACE:
		processElement(func(str string) {
			if str != "" {
				rlock.Lock()
				re, found := replx[str]
				if !found {
					re, found = replx[str]
					if !found {
						nw, err := regexp.Compile(reg)
						if err == nil {
							replx[str] = nw
							re = nw
						}
					}
				}
				rlock.Unlock()
				if re != nil {
					txt := re.ReplaceAllString(str, exp)
					if txt != "" {
						ok = true
						// wrp-directed EscapeString was delayed for REPLACE
						if wrp {
							txt = html.EscapeString(txt)
						}
						buffer.WriteString(between)
						buffer.WriteString(txt)
						between = sep
					}
				}
			}
		})

	case VALUE:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case NUM:
		// defer to "#" prefix processing to allow counting of container objects
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case LEN:
		// reverted defer to "%" prefix processing in processElement LEN, LENGTH case,
		// sacrifices getting same result with trivial doubly-specified -len "%object"
		// in order to restore more important use of -len "*" to get length of XML
		length := 0

		processElement(func(str string) {
			length += len(str)
			ok = true
		})

		if ok {
			// length of element strings
			val := strconv.Itoa(length)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case SUM:
		sum := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				sum += value
				ok = true
			}
		})

		if ok {
			// sum of element values
			val := strconv.Itoa(sum)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case ACC:
		sum := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				sum += value
				ok = true
				// running sum of element values
				val := strconv.Itoa(sum)
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
			}
		})

	case MIN:
		min := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				if !ok || value < min {
					min = value
				}
				ok = true
			}
		})

		if ok {
			// minimum of element values
			val := strconv.Itoa(min)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case MAX:
		max := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				if !ok || value > max {
					max = value
				}
				ok = true
			}
		})

		if ok {
			// maximum of element values
			val := strconv.Itoa(max)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case INC:
		processElement(func(str string) {
			if str != "" {
				num, err := strconv.Atoi(str)
				if err == nil {
					// increment value
					num++
					val := strconv.Itoa(num)
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(val)
					between = sep
				}
			}
		})

	case DEC:
		processElement(func(str string) {
			if str != "" {
				num, err := strconv.Atoi(str)
				if err == nil {
					// decrement value
					num--
					val := strconv.Itoa(num)
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(val)
					between = sep
				}
			}
		})

	case SUB:
		first := 0
		second := 0
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				count++
				if count == 1 {
					first = value
				} else if count == 2 {
					second = value
				}
			}
		})

		if count == 2 {
			// must have exactly 2 elements
			ok = true
			// difference of element values
			val := strconv.Itoa(first - second)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case AVG:
		sum := 0
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				sum += value
				count++
				ok = true
			}
		})

		if ok {
			// average of element values
			avg := int(float64(sum) / float64(count))
			val := strconv.Itoa(avg)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case DEV:
		count := 0
		mean := 0.0
		m2 := 0.0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				// Welford algorithm for one-pass standard deviation
				count++
				x := float64(value)
				delta := x - mean
				mean += delta / float64(count)
				m2 += delta * (x - mean)
			}
		})

		if count > 1 {
			// must have at least 2 elements
			ok = true
			// standard deviation of element values
			vrc := m2 / float64(count-1)
			dev := int(math.Sqrt(vrc))
			val := strconv.Itoa(dev)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case MED:
		var arry []int
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				arry = append(arry, value)
				count++
				ok = true
			}
		})

		if ok {
			// median of element values
			slices.Sort(arry)
			med := 0
			// convention for even-numbered lists adapted from https://github.com/SimonWaldherr/golibs/blob/v0.11.0/xmath/math.go#L181
			if count%2 == 1 {
				med = arry[count/2]
			} else {
				med = (arry[count/2] + arry[count/2-1]) / 2
			}
			val := strconv.Itoa(med)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case MUL:
		first := 0
		second := 0
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				count++
				if count == 1 {
					first = value
				} else if count == 2 {
					second = value
				}
			}
		})

		if count == 2 {
			// must have exactly 2 elements
			ok = true
			// product of element values
			val := strconv.Itoa(first * second)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case DIV:
		first := 0
		second := 0
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				count++
				if count == 1 {
					first = value
				} else if count == 2 {
					second = value
				}
			}
		})

		if count == 2 {
			// must have exactly 2 elements
			ok = true
			// quotient of element values
			val := strconv.Itoa(first / second)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case MOD:
		first := 0
		second := 0
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				count++
				if count == 1 {
					first = value
				} else if count == 2 {
					second = value
				}
			}
		})

		if count == 2 {
			// must have exactly 2 elements
			ok = true
			// modulus of element values
			val := strconv.Itoa(first % second)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case GEO:
		prod := float64(1)
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				prod *= float64(value)
				count++
				ok = true
			}
		})

		if ok {
			// adapted from https://github.com/SimonWaldherr/golibs/blob/v0.11.0/xmath/math.go#L246
			geo := int(float64(math.Pow(float64(prod), 1/float64(count))))
			val := strconv.Itoa(geo)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case HRM:
		sum := float64(0)
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				sum += 1 / float64(value)
				count++
				ok = true
			}
		})

		if ok {
			// adapted from https://github.com/SimonWaldherr/golibs/blob/v0.11.0/xmath/math.go#L229
			hrm := int(float64(count) * 1 / sum)
			val := strconv.Itoa(hrm)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case RMS:
		sum := float64(0)
		count := 0

		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil {
				sum += math.Pow(float64(value), 2)
				count++
				ok = true
			}
		})

		if ok {
			// adapted from https://github.com/SimonWaldherr/golibs/blob/v0.11.0/xmath/math.go#L216
			rms := int(math.Sqrt(sum / float64(count)))
			val := strconv.Itoa(rms)
			buffer.WriteString(between)
			buffer.WriteString(val)
			between = sep
		}

	case SQT:
		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil && value >= 0 {
				sqt := int(math.Sqrt(float64(value)))
				val := strconv.Itoa(sqt)
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
				ok = true
			}
		})

	case LG2, LGE, LOG:
		// return logarithm truncated to integer
		processElement(func(str string) {
			value, err := strconv.Atoi(str)
			if err == nil && value > 0 {
				lg := float64(0)
				if status == LG2 {
					lg = math.Log2(float64(value))
				} else if status == LGE {
					lg = math.Log(float64(value))
				} else if status == LOG {
					lg = math.Log10(float64(value))
				}
				dec, _ := math.Modf(lg)
				val := strconv.Itoa(int(dec))
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
				ok = true
			}
		})

	case BIN:
		processElement(func(str string) {
			num, err := strconv.Atoi(str)
			if err == nil {
				// convert to binary representation
				val := strconv.FormatInt(int64(num), 2)
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
				ok = true
			}
		})

	case OCT:
		processElement(func(str string) {
			num, err := strconv.Atoi(str)
			if err == nil {
				// convert to octal representation
				val := strconv.FormatInt(int64(num), 8)
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
				ok = true
			}
		})

	case HEX:
		processElement(func(str string) {
			num, err := strconv.Atoi(str)
			if err == nil {
				// convert to hexadecimal representation
				val := strconv.FormatInt(int64(num), 16)
				val = strings.ToUpper(val)
				// val := fmt.Sprintf("%X", num)
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
				ok = true
			}
		})

	case BIT:
		processElement(func(str string) {
			num, err := strconv.Atoi(str)
			if err == nil {
				// Kernighan algorithm for counting set bits
				count := 0
				for num != 0 {
					num &= num - 1
					count++
				}
				val := strconv.Itoa(count)
				buffer.WriteString(between)
				buffer.WriteString(val)
				between = sep
				ok = true
			}
		})

	case PAD:
		processElement(func(str string) {
			if str != "" {
				str = PadNumericID(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
				ok = true
			}
		})

	case RAW:
		// for development and debugging of common XML cleanup functions (undocumented)
		processElement(func(str string) {
			if str != "" {
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
				ok = true
			}
		})

	case ZEROBASED:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case ONEBASED:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case UCSCBASED:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case NUCLEIC:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case REVCOMP:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = ReverseComplement(str)
				buffer.WriteString(str)
				between = sep
			}
		})

	case FASTA:
		processElement(func(str string) {
			for str != "" {
				mx := len(str)
				if mx > 70 {
					mx = 70
				}
				item := str[:mx]
				str = str[mx:]
				ok = true
				item = strings.ToUpper(item)
				buffer.WriteString(between)
				buffer.WriteString(item)
				between = sep
			}
		})

	case NCBI2NA:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = Ncbi2naToIupac(str)
				buffer.WriteString(str)
				between = sep
			}
		})

	case NCBI4NA:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = Ncbi4naToIupac(str)
				buffer.WriteString(str)
				between = sep
			}
		})

	case CDS2PROT:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = TranslateCdRegion(str, cls.Gcd, cls.Frm, true, true, true, true, true, "")
				buffer.WriteString(str)
				between = sep
			}
		})

	case PEPTS:
		processElement(func(str string) {
			if str != "" {

				clauses := strings.FieldsFunc(str, func(c rune) bool {
					return c == '*' || c == '-' || c == 'X' || c == 'x' || !unicode.IsLetter(c) || c > 127
				})
				for _, item := range clauses {
					item = strings.ToLower(item)
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case MOLWTX, MOLWTM, MOLWTF:
		removeMet := false
		formylMet := false
		if status == MOLWTX {
			removeMet = true
		} else if status == MOLWTF {
			formylMet = true
		}
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = ProteinWeight(str, removeMet, formylMet)
				buffer.WriteString(str)
				between = sep
			}
		})

	case ACCESSION:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = strings.Replace(str, ".", "_", -1)
				buffer.WriteString(str)
				between = sep
			}
		})

	case NUMERIC:
		processElement(func(str string) {
			if str != "" {
				if IsAllDigits(str) {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
			}
		})

	case HGVS:
		processElement(func(str string) {
			if str != "" {
				ok = true
				buffer.WriteString(between)
				str = ParseHGVS(str)
				buffer.WriteString(str)
				between = sep
			}
		})

	case INDEXER:
		// build positional index with a choice of TITL, TIAB, ABST, TERM, TEXT, and STEM field names
		label := "TEXT"
		if indexerField != "" {
			label = indexerField
		}

		indices := make(map[string][]string)

		cumulative := 0

		var ilock sync.Mutex

		addItem := func(term string, position int) {

			// protect with mutex
			ilock.Lock()

			arry, found := indices[term]
			if !found {
				arry = make([]string, 0, 1)
			}
			arry = append(arry, strconv.Itoa(position))
			indices[term] = arry

			ilock.Unlock()
		}

		processElement(func(str string) {

			if str == "" {
				return
			}

			if str == "[Not Available]." {
				return
			}

			// remove parentheses to keep bracketed subscripts
			/*
				var (
					buffer []rune
					prev   rune
					inside bool
				)
				for _, ch := range str {
					if ch == '(' && prev != ' ' {
						inside = true
					} else if ch == ')' && inside {
						inside = false
					} else {
						buffer = append(buffer, ch)
					}
					prev = ch
				}
				str = string(buffer)
			*/

			if IsNotASCII(str) {
				str = FixMisusedLetters(str, true, false, true)
				str = TransformAccents(str, true, true)
				if HasUnicodeMarkup(str) {
					str = RepairUnicodeMarkup(str, SPACE)
				}
			}

			str = strings.ToLower(str)

			if HasBadSpace(str) {
				str = CleanupBadSpaces(str)
			}
			if HasAngleOrAmpersandEncoding(str) {
				str = RepairEncodedMarkup(str)
				str = RepairTableMarkup(str, SPACE)
				str = RepairScriptMarkup(str, SPACE)
				str = RepairMathMLMarkup(str, SPACE)
				// RemoveEmbeddedMarkup must be called before UnescapeString, which was suppressed in ExploreElements
				str = RemoveEmbeddedMarkup(str)
			}

			if HasAmpOrNotASCII(str) {
				str = html.UnescapeString(str)
				str = strings.ToLower(str)
			}

			if HasAdjacentSpaces(str) {
				str = CompressRunsOfSpaces(str)
			}

			str = strings.Replace(str, "(", " ", -1)
			str = strings.Replace(str, ")", " ", -1)

			str = strings.Replace(str, "_", " ", -1)

			if HasHyphenOrApostrophe(str) {
				str = FixSpecialCases(str)
			}

			str = strings.Replace(str, "-", " ", -1)

			// remove trailing punctuation from each word
			var arry []string

			terms := strings.Fields(str)
			for _, item := range terms {
				max := len(item)
				for max > 1 {
					ch := item[max-1]
					if ch != '.' && ch != ',' && ch != ':' && ch != ';' {
						break
					}
					// trim trailing period, comma, colon, and semicolon
					item = item[:max-1]
					// continue checking for runs of punctuation at end
					max--
				}
				if item == "" {
					continue
				}
				arry = append(arry, item)
			}

			// rejoin into string
			cleaned := strings.Join(arry, " ")

			// break clauses at punctuation other than space or underscore, and at non-ASCII characters
			clauses := strings.FieldsFunc(cleaned, func(c rune) bool {
				return (!unicode.IsLetter(c) && !unicode.IsDigit(c)) && c != ' ' && c != '_' || c > 127
			})

			// space replaces plus sign to separate runs of unpunctuated words
			phrases := strings.Join(clauses, " ")

			// break phrases into individual words
			words := strings.Fields(phrases)

			for _, item := range words {

				cumulative++

				// skip at site of punctuation break
				if item == "+" {
					continue
				}

				// skip if just a period, but allow terms that are all digits or period
				if item == "." {
					continue
				}

				// optional stop word removal
				if deStop && IsStopWord(item) {
					continue
				}

				if label == "STEM" {
					// optionally apply stemming algorithm
					item = porter2.Stem(item)
					item = strings.TrimSpace(item)
				}

				// index single normalized term with positions
				addItem(item, cumulative)
				ok = true
			}

			// pad to avoid false positive proximity match of words in adjacent paragraphs
			rounded := ((cumulative + 99) / 100) * 100
			if rounded-cumulative < 20 {
				rounded += 100
			}
			cumulative = rounded
		})

		prepareIndices := func() {

			if len(indices) < 1 {
				return
			}

			arry := slices.Sorted(maps.Keys(indices))

			last := ""
			for _, item := range arry {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				if item == last {
					// skip duplicate entry
					continue
				}
				buffer.WriteString("<")
				buffer.WriteString(label)
				if len(indices[item]) > 0 {
					// use attribute for position
					buffer.WriteString(" pos=\"")
					attr := strings.Join(indices[item], ",")
					buffer.WriteString(attr)
					buffer.WriteString("\"")
				}
				buffer.WriteString(">")
				buffer.WriteString(item)
				buffer.WriteString("</")
				buffer.WriteString(label)
				buffer.WriteString(">")
				last = item
			}
		}

		if ok {
			prepareIndices()
		}

	case TERMS:
		processElement(func(str string) {
			if str != "" {

				terms := strings.Fields(str)
				for _, item := range terms {
					max := len(item)
					for max > 1 {
						ch := item[max-1]
						if ch != '.' && ch != ',' && ch != ':' && ch != ';' {
							break
						}
						// trim trailing period, comma, colon, and semicolon
						item = item[:max-1]
						// continue checking for runs of punctuation at end
						max--
					}
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case WORDS:
		processElement(func(str string) {
			if str != "" {

				words := strings.FieldsFunc(str, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsDigit(c)
				})
				for _, item := range words {
					item = strings.ToLower(item)
					if deStop {
						if IsStopWord(item) {
							continue
						}
					}
					if doStem {
						item = porter2.Stem(item)
						item = strings.TrimSpace(item)
					}
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case PAIRS, PAIRX:
		processElement(func(str string) {
			if str != "" {

				doSingle := (status == PAIRX)

				if doSingle {
					str = PrepareForIndexing(str, true, false, true, true, true)
				}

				// break clauses at punctuation other than space, and at non-ASCII characters
				clauses := strings.FieldsFunc(str, func(c rune) bool {
					return (!unicode.IsLetter(c) && !unicode.IsDigit(c)) && c != ' ' || c > 127
				})

				// plus sign separates runs of unpunctuated words
				phrases := strings.Join(clauses, " + ")

				// break phrases into individual words
				words := strings.FieldsFunc(phrases, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsDigit(c)
				})

				// word pairs (or isolated singletons) separated by stop words
				if len(words) > 1 {
					past := ""
					run := 0
					for _, item := range words {
						if item == "+" {
							if doSingle && run == 1 && past != "" {
								ok = true
								buffer.WriteString(between)
								buffer.WriteString(past)
								between = sep
							}
							past = ""
							run = 0
							continue
						}
						item = strings.ToLower(item)
						if deStop {
							if IsStopWord(item) {
								if doSingle && run == 1 && past != "" {
									ok = true
									buffer.WriteString(between)
									buffer.WriteString(past)
									between = sep
								}
								past = ""
								run = 0
								continue
							}
						}
						if doStem {
							item = porter2.Stem(item)
							item = strings.TrimSpace(item)
						}
						if item == "" {
							past = ""
							continue
						}
						if past != "" {
							ok = true
							buffer.WriteString(between)
							buffer.WriteString(past + " " + item)
							between = sep
						}
						past = item
						run++
					}
					if doSingle && run == 1 && past != "" {
						ok = true
						buffer.WriteString(between)
						buffer.WriteString(past)
						between = sep
					}
				}
			}
		})

	case SPLIT:
		processElement(func(str string) {
			if str != "" && cls.Wth != "" {

				items := strings.Split(str, cls.Wth)
				for _, item := range items {
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case ORDER:
		processElement(func(str string) {
			if str != "" {
				ok = true
				str = SortStringByWords(str)
				buffer.WriteString(between)
				buffer.WriteString(str)
				between = sep
			}
		})

	case REVERSE:
		processElement(func(str string) {
			if str != "" {

				words := strings.FieldsFunc(str, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsDigit(c)
				})
				for lf, rt := 0, len(words)-1; lf < rt; lf, rt = lf+1, rt-1 {
					words[lf], words[rt] = words[rt], words[lf]
				}
				for _, item := range words {
					item = strings.ToLower(item)
					if deStop {
						if IsStopWord(item) {
							continue
						}
					}
					if doStem {
						item = porter2.Stem(item)
						item = strings.TrimSpace(item)
					}
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case LETTERS:
		processElement(func(str string) {
			if str != "" {
				for _, ch := range str {
					ok = true
					buffer.WriteString(between)
					buffer.WriteRune(ch)
					between = sep
				}
			}
		})

	case PENTAMERS:
		processElement(func(str string) {
			if str != "" {
				arry := SlidingSlices(str, 5)
				for _, item := range arry {
					item = strings.ToLower(item)
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case CLAUSES:
		processElement(func(str string) {
			if str != "" {

				clauses := strings.FieldsFunc(str, func(c rune) bool {
					return c == '.' || c == ',' || c == ';' || c == ':'
				})
				for _, item := range clauses {
					item = strings.ToLower(item)
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(item)
					between = sep
				}
			}
		})

	case MESHCODE:
		var code []string
		var tree []string

		processElement(func(str string) {
			if str != "" {
				txt, found := transform[str]
				str = strings.ToLower(str)
				code = append(code, str)
				ok = true

				if !found {
					return
				}
				txt = strings.ToLower(txt)
				txt = strings.Replace(txt, ".", "_", -1)
				codes := strings.FieldsFunc(txt, func(c rune) bool {
					return c == ','
				})
				for _, item := range codes {
					ch := item[0]
					if item == "" {
						continue
					}
					switch ch {
					case 'a', 'c', 'd', 'e', 'f', 'g', 'z':
						tree = append(tree, item)
					default:
					}
				}
			}
		})

		if len(code) > 1 {
			slices.Sort(code)
		}
		if len(tree) > 1 {
			slices.Sort(tree)
		}

		last := ""
		for _, item := range code {
			if item == last {
				// skip duplicate entry
				continue
			}
			buffer.WriteString("<CODE>")
			buffer.WriteString(item)
			buffer.WriteString("</CODE>")
			last = item
		}

		last = ""
		for _, item := range tree {
			if item == last {
				// skip duplicate entry
				continue
			}
			buffer.WriteString("<TREE>")
			buffer.WriteString(item)
			buffer.WriteString("</TREE>")
			last = item
		}

	case MATRIX:
		var arry []string

		processElement(func(str string) {
			if str != "" {
				txt, found := transform[str]
				if found {
					str = txt
				}
				arry = append(arry, str)
				ok = true
			}
		})

		if len(arry) > 1 {
			slices.Sort(arry)

			for i, frst := range arry {
				for j, scnd := range arry {
					if i == j {
						continue
					}
					buffer.WriteString(between)
					buffer.WriteString(frst)
					buffer.WriteString("\t")
					buffer.WriteString(scnd)
					between = "\n"
				}
			}
		}

	case CLASSIFY:
		processElement(func(str string) {
			if str != "" {
				kywds := make(map[string]bool)

				// search for whole word or whole phrase substrings
				srchr.Search(str[:],
					func(str, pat string, pos int) bool {
						mtch := strings.TrimSpace(pat)
						rslt := transform[mtch]
						if rslt != "" {
							items := strings.Split(rslt, ",")
							for _, itm := range items {
								tag, val := SplitInTwoRight(itm, ":")
								txt := val
								if tag != "" {
									txt = "<" + tag + ">" + val + "</" + tag + ">"
								}
								kywds[txt] = true
							}
						}
						return true
					})

				keys := slices.Sorted(maps.Keys(kywds))

				// record single copy of each match, in alphabetical order
				for _, key := range keys {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(key)
					between = sep
				}
			}
		})

	case HISTOGRAM:
		processElement(func(str string) {
			if str != "" {
				ok = true

				hlock.Lock()

				val := histogram[str]
				val++
				histogram[str] = val
				/*
					if strings.Contains(str, "&lt;sub&gt;") || strings.Contains(str, "<sub>") {
						val := histogram["sub"]
						val++
						histogram["sub"] = val
					}
					if strings.Contains(str, "&lt;sup&gt;") || strings.Contains(str, "<sup>") {
						val := histogram["sup"]
						val++
						histogram["sup"] = val
					}
					for _, ch := range str {
						if IsUnicodeSubsc(ch) {
							val := histogram["usb"]
							val++
							histogram["usb"] = val
							break
						}
					}
					for _, ch := range str {
						if IsUnicodeSuper(ch) {
							val := histogram["usp"]
							val++
							histogram["usp"] = val
							break
						}
					}
				*/
				/*
					for _, ch := range str {
						num := strconv.Itoa(int(ch))
						val := histogram[num]
						val++
						histogram[num] = val
					}
				*/

				hlock.Unlock()
			}
		})

	case FREQUENCY:
		processElement(func(str string) {
			if str != "" {
				ok = true

				hlock.Lock()

				for _, ch := range str {
					if ch < 128 {
						continue
					}
					key := fmt.Sprintf("0x%04X", int(ch))
					val := histogram[key]
					val++
					histogram[key] = val
				}

				hlock.Unlock()
			}
		})

	case ACCENTED:
		processElement(func(str string) {
			if str != "" {
				found := false
				for _, ch := range str {
					if ch > 127 {
						found = true
						break
					}
				}
				if found {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString(str)
					between = sep
				}
			}
		})
		/*
			processElement(func(str string) {
				if str != "" {
					found := false
					for _, ch := range str {
						if ch > 127 {
							found = true
							break
						}
					}
					if found {
						ok = true
						buffer.WriteString(between)
						buffer.WriteString(str)
						between = sep
					}
				}
			})
		*/
		/*
			processElement(func(str string) {
				if str != "" {
					terms := strings.Fields(str)
					for _, item := range terms {
						max := len(item)
						for max > 1 {
							ch := item[max-1]
							if ch != '.' && ch != ',' && ch != ':' && ch != ';' {
								break
							}
							// trim trailing period, comma, colon, and semicolon
							item = item[:max-1]
							// continue checking for runs of punctuation at end
							max--
						}
						if item == "" {
							continue
						}
						found := false
						for _, ch := range item {
							if ch > 127 {
								found = true
								break
							}
						}
						if found {
							ok = true
							after := TransformAccents(item, true, false)
							for _, c := range item {
								if c > 127 {
									tg := fmt.Sprintf("%d\t%U\t%s\t%s\t%s\n", c, c, string(c), item, after)
									buffer.WriteString(tg)
								}
							}
						}
					}
				}
			})
		*/

	case TEST:
		suffix := ""
		if reg == "" && exp != "" {
			suffix = " in " + exp
		}

		processElement(func(str string) {
			if str != "" {
				if HasCombiningAccent(str[:]) {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString("Combining Accent" + suffix)
					between = sep
				}
				if HasInvisibleUnicode(str[:]) {
					ok = true
					buffer.WriteString(between)
					buffer.WriteString("Invisible Unicode" + suffix)
					between = sep
				}
			}
		})

	case SCAN:
		// for identification of records with current data issues of interest (undocumented)
		processElement(func(str string) {
			if str != "" {
				for _, ch := range str {
					if ch < 128 {
						continue
					}
					if ch == 223 {
						// sharp s 0x00DF
						ok = true
						buffer.WriteString(between)
						buffer.WriteString("0x00DF")
						between = sep
						break
					}
					if ch == 946 {
						// beta 0x03B2
						ok = true
						buffer.WriteString(between)
						buffer.WriteString("0x03B2")
						between = sep
						break
					}
				}
				/*
					for _, ch := range str {
						if ch < 128 {
							continue
						}
						if ch == 181 {
							// micro 0x00B5
							ok = true
							buffer.WriteString(between)
							buffer.WriteString("0x00B5")
							between = sep
							break
						}
						if ch == 956 {
							// mu 0x03BC
							ok = true
							buffer.WriteString(between)
							buffer.WriteString("0x03BC")
							between = sep
							break
						}
					}
				*/
				/*
					terms := strings.Fields(str)
					for _, item := range terms {
						hasmicro := false
						hasmu := false
						for _, ch := range item {
							if ch < 128 {
								continue
							}
							if ch == 181 {
								// micro 0x00B5
								hasmicro = true
							}
							if ch == 956 {
								// mu 0x03BC
								hasmu = true
							}
						}
						if hasmicro || hasmu {
							ok = true
							buffer.WriteString(between)
							if hasmicro {
								buffer.WriteString("r")
							}
							if hasmu {
								buffer.WriteString("u")
							}
							buffer.WriteString("\t")
							buffer.WriteString(item)
							between = sep
						}
					}
				*/
			}
		})

	default:
	}

	// use default value if nothing written
	if !ok && cls.Def != "" {
		ok = true
		buffer.WriteString(cls.Def)
	}

	buffer.WriteString(sfx)

	if !ok {
		return "", false
	}

	txt := buffer.String()

	return txt, true
}

// processInstructions performs extraction commands on a subset of XML
func processInstructions(
	commands []*Operation,
	curr *XMLNode,
	mask string,
	tab string,
	ret string,
	index int,
	level int,
	variables map[string]string,
	transform map[string]string,
	srchr *FSMSearcher,
	histogram map[string]int,
	accum func(string),
) (string, string) {

	if accum == nil {
		return tab, ret
	}

	sep := "\t"
	pfx := ""
	sfx := ""
	plg := ""
	elg := ""
	lst := ""

	def := ""
	msk := ""

	reg := ""
	exp := ""

	wth := ""

	gcd := 0
	frm := 0

	col := "\t"
	lin := "\n"

	varname := ""
	isAccum := false

	wrp := false

	plain := true
	var currColor *color.Color

	// handles color, e.g., -color "red,bold", reset to plain by -color "-" (undocumented)
	printInColor := func(str string) {
		if plain || currColor == nil {
			accum(str)
		} else {
			tx := currColor.SprintFunc()
			tmp := fmt.Sprintf("%s", tx(str))
			accum(tmp)
		}
	}

	// process commands
	for _, op := range commands {

		str := op.Value

		switch op.Type {
		case ELEMENT:
			cls := Clause{Prev: tab, Pfx: pfx, Sfx: sfx, Plg: plg, Sep: sep, Def: def, Msk: msk, Reg: reg, Exp: exp, Wth: wth, Gcd: gcd, Frm: frm, Wrp: wrp}
			txt, ok := processClause(curr, op.Stages, mask, cls, op.Type, index, level, variables, transform, srchr, histogram)
			if ok {
				if varname != "" {
					// CONVERTER value may have preceding tab, change to space and trim spaces
					txt = strings.Replace(txt, "\t", " ", -1)
					txt = strings.TrimSpace(txt)
					variables[varname] = txt
					varname = ""
				} else {
					plg = ""
					lst = elg
					tab = col
					ret = lin
					if plain {
						accum(txt)
					} else {
						printInColor(txt)
					}
				}
			}
		case HISTOGRAM:
			cls := Clause{Prev: "", Pfx: "", Sfx: "", Plg: "", Sep: "", Def: "", Msk: "", Reg: "", Exp: "", Wth: "", Gcd: gcd, Frm: frm, Wrp: wrp}
			txt, ok := processClause(curr, op.Stages, mask, cls, op.Type, index, level, variables, transform, srchr, histogram)
			if ok {
				accum(txt)
			}
		case TAB:
			col = str
		case RET:
			lin = str
		case PFX:
			pfx = str
		case SFX:
			sfx = str
		case SEP:
			sep = str
		case TAG:
			wrp = true
			fallthrough
		case LBL:
			lbl := str
			accum(tab)
			accum(plg)
			accum(pfx)
			if plain {
				accum(lbl)
			} else {
				printInColor(lbl)
			}
			accum(sfx)
			plg = ""
			lst = elg
			tab = col
			ret = lin
		case PFC:
			// preface clears previous tab and sets prefix in one command
			pfx = str
			fallthrough
		case CLR:
			// clear previous tab after the fact
			tab = ""
		case DEQ:
			// set queued tab after the fact
			tab = str
		case PLG:
			plg = str
		case ELG:
			elg = str
		case WRP:
			// shortcut to wrap elements in XML tags
			if str == "" || str == "-" {
				sep = "\t"
				pfx = ""
				sfx = ""
				plg = ""
				elg = ""
				wrp = false
				break
			}
			if strings.Index(str, ",") >= 0 {
				// -wrp with comma-separated arguments is deprecated, but supported for backward compatibility
				lft, rgt := SplitInTwoRight(str, ",")
				if lft != "" {
					plg = "<" + lft + ">"
					elg = "</" + lft + ">"
				}
				if rgt != "" && rgt != "-" {
					pfx = "<" + rgt + ">"
					sfx = "</" + rgt + ">"
					sep = sfx + pfx
				}
				wrp = true
				break
			}
			if strings.Index(str, "/") >= 0 {
				// supports slash-separated components
				pfx = ""
				sfx = ""
				sep = ""
				items := strings.Split(str, "/")
				for i := range len(items) {
					tmp := items[i]
					// replace spaces with underscore
					tmp = strings.Replace(tmp, " ", "_", -1)
					pfx += "<" + tmp + ">"
				}
				for i := len(items) - 1; i >= 0; i-- {
					tmp := items[i]
					tmp = strings.Replace(tmp, " ", "_", -1)
					sfx += "</" + tmp + ">"
				}
				sep = sfx + pfx
				wrp = true
				break
			}
			// shortcut for strings.HasPrefix(str, "&") and strings.TrimPrefix(str, "&")
			if len(str) > 1 && str[0] == '&' {
				str = str[1:]
				// expand variable to get actual tag
				str = variables[str]
			}
			// single object name, no comma or slash
			tmp := strings.Replace(str, " ", "_", -1)
			pfx = "<" + tmp + ">"
			sfx = "</" + tmp + ">"
			sep = sfx + pfx
			wrp = true
		case BKT:
			// shortcut to wrap elements in bracketed fields
			if str == "" || str == "-" {
				sep = "\t"
				pfx = ""
				sfx = ""
				plg = ""
				elg = ""
				wrp = false
				break
			}
			// shortcut for strings.HasPrefix(str, "&") and strings.TrimPrefix(str, "&")
			if len(str) > 1 && str[0] == '&' {
				str = str[1:]
				// expand variable to get actual tag
				str = variables[str]
			}
			// single object name, no comma or slash
			tmp := strings.Replace(str, " ", "_", -1)
			pfx = "[" + tmp + "="
			sfx = "]"
			sep = ","
			wrp = true
		case ENC:
			// shortcut to mark unexpanded instances with XML tags
			plg = ""
			elg = ""
			// shortcut for strings.HasPrefix(str, "&") and strings.TrimPrefix(str, "&")
			if len(str) > 1 && str[0] == '&' {
				str = str[1:]
				// expand variable to get actual tag
				str = variables[str]
			}
			if str != "" && str != "-" {
				items := strings.Split(str, "/")
				for i := range len(items) {
					tmp := items[i]
					// replace spaces with underscore
					tmp = strings.Replace(tmp, " ", "_", -1)
					plg += "<" + tmp + ">"
				}
				for i := len(items) - 1; i >= 0; i-- {
					tmp := items[i]
					tmp = strings.Replace(tmp, " ", "_", -1)
					elg += "</" + tmp + ">"
				}
			}
		case RST:
			pfx = ""
			sfx = ""
			plg = ""
			elg = ""
			sep = "\t"
			def = ""
			msk = ""
			wrp = false
		case DEF:
			def = str
		case MSK:
			msk = str
		case REG:
			reg = str
		case EXP:
			exp = str
		case WITH:
			wth = str
		case GCODE:
			gcd = 0
			if len(str) > 1 && str[0] == '&' {
				str = str[1:]
				// expand variable to get actual tag
				str = variables[str]
			}
			if IsAllDigits(str) {
				val, err := strconv.Atoi(str)
				if err == nil {
					gcd = val
				}
			}
		case FRAME0, FRAME1:
			frm = 0
			if len(str) > 1 && str[0] == '&' {
				str = str[1:]
				// expand variable to get actual tag
				str = variables[str]
			}
			if IsAllDigits(str) {
				val, err := strconv.Atoi(str)
				if err == nil {
					if op.Type == FRAME0 && val >= 0 {
						// already 0-based offset (0-2)
						frm = val
					} else if op.Type == FRAME1 && val > 0 {
						// convert from frame (1-3) to offset (0-2)
						frm = val - 1
					}
				}
			}
		case COLOR:
			currColor = color.New()
			if str == "-" || str == "reset" || str == "clear" {
				plain = true
				break
			}
			plain = false
			items := strings.Split(str, ",")
			for _, itm := range items {
				switch itm {
				case "red":
					currColor.Add(color.FgRed)
				case "grn", "green":
					currColor.Add(color.FgGreen)
				case "blu", "blue":
					currColor.Add(color.FgBlue)
				case "blk", "black":
					currColor.Add(color.FgBlack)
				case "bld", "bold":
					currColor.Add(color.Bold)
				case "ital", "italic", "italics":
					currColor.Add(color.Italic)
				case "blink", "flash":
					currColor.Add(color.BlinkSlow)
				default:
					DisplayError("Unrecognized color argument '%s'", itm)
					os.Exit(1)
				}
			}
		case VARIABLE:
			isAccum = false
			varname = str
		case ACCUMULATOR:
			isAccum = true
			varname = str
		case CONVERTER:
			varname = str
		case VALUE:
			length := len(str)
			if length > 1 && str[0] == '(' && str[length-1] == ')' {
				// set variable from literal text inside parentheses, e.g., -COM "(, )"
				variables[varname] = str[1 : length-1]
				// -if "&VARIABLE" will succeed if set to blank with empty parentheses "()"
			} else if str == "" {
				// -if "&VARIABLE" will fail if initialized with empty string ""
				delete(variables, varname)
			} else {
				cls := Clause{Prev: "", Pfx: pfx, Sfx: sfx, Plg: plg, Sep: sep, Def: def, Msk: msk, Reg: reg, Exp: exp, Wth: wth, Gcd: gcd, Frm: frm, Wrp: wrp}
				txt, ok := processClause(curr, op.Stages, mask, cls, op.Type, index, level, variables, transform, srchr, histogram)
				if ok {
					plg = ""
					lst = elg
					if isAccum {
						if variables[varname] == "" {
							variables[varname] = txt
						} else {
							variables[varname] += sep + txt
						}
					} else {
						variables[varname] = txt
					}
				}
			}
			varname = ""
			isAccum = false
		default:
			cls := Clause{Prev: tab, Pfx: pfx, Sfx: sfx, Plg: plg, Sep: sep, Def: def, Msk: msk, Reg: reg, Exp: exp, Wth: wth, Gcd: gcd, Frm: frm, Wrp: wrp}
			txt, ok := processClause(curr, op.Stages, mask, cls, op.Type, index, level, variables, transform, srchr, histogram)
			if ok {
				if varname != "" {
					// CONVERTER value may have preceding tab, change to space and trim spaces
					txt = strings.Replace(txt, "\t", " ", -1)
					txt = strings.TrimSpace(txt)
					variables[varname] = txt
					varname = ""
				} else {
					plg = ""
					lst = elg
					tab = col
					ret = lin
					if plain {
						accum(txt)
					} else {
						printInColor(txt)
					}
				}
			}
		}
	}

	if plain {
		accum(lst)
	} else {
		printInColor(lst)
	}

	return tab, ret
}

// CONDITIONAL EXECUTION USES -if AND -unless STATEMENT, WITH SUPPORT FOR DEPRECATED -match AND -avoid STATEMENTS

// conditionsAreSatisfied tests a set of conditions to determine if extraction should proceed
func conditionsAreSatisfied(conditions []*Operation, curr *XMLNode, mask string, index, level int, variables map[string]string) bool {

	if curr == nil {
		return false
	}

	required := 0
	observed := 0
	forbidden := 0
	isMatch := false
	isAvoid := false

	// matchFound tests individual conditions
	matchFound := func(stages []*Step) bool {

		if stages == nil || len(stages) < 1 {
			return false
		}

		stage := stages[0]

		var constraint *Step

		if len(stages) > 1 {
			constraint = stages[1]
		}

		status := stage.Type
		prnt := stage.Parent
		match := stage.Match
		attrib := stage.Attrib
		typL := stage.TypL
		strL := stage.StrL
		intL := stage.IntL
		typR := stage.TypR
		strR := stage.StrR
		intR := stage.IntR
		norm := stage.Norm
		wildcard := stage.Wild
		unescape := true

		found := false
		number := ""

		// exploreElements is a wrapper for ExploreElements, obtaining most arguments as closures
		exploreElements := func(proc func(string, int)) {
			ExploreElements(curr, mask, prnt, match, attrib, wildcard, unescape, level, proc)
		}

		// test string or numeric constraints
		testConstraint := func(str string) bool {

			if str == "" || constraint == nil {
				return false
			}

			val := constraint.Value
			stat := constraint.Type

			switch stat {
			case EQUALS, CONTAINS, INCLUDES, EXCLUDES, ISWITHIN, STARTSWITH, ENDSWITH, ISNOT, ISBEFORE, ISAFTER, MATCHES, RESEMBLES:
				// substring test on element values
				str = strings.ToUpper(str)
				val = strings.ToUpper(val)

				switch stat {
				case EQUALS:
					if str == val {
						return true
					}
				case CONTAINS:
					if strings.Contains(str, val) {
						return true
					}
				case INCLUDES:
					str = strings.TrimSpace(str)
					val = strings.TrimSpace(val)
					if strings.Contains(" "+str+" ", " "+val+" ") {
						return true
					}
				case EXCLUDES:
					if !strings.Contains(str, val) {
						return true
					}
				case ISWITHIN:
					if strings.Contains(val, str) {
						return true
					}
				case STARTSWITH:
					if strings.HasPrefix(str, val) {
						return true
					}
				case ENDSWITH:
					if strings.HasSuffix(str, val) {
						return true
					}
				case ISNOT:
					if str != val {
						return true
					}
				case ISBEFORE:
					if str < val {
						return true
					}
				case ISAFTER:
					if str > val {
						return true
					}
				case MATCHES:
					if RemoveCommaOrSemicolon(str) == strings.ToLower(val) {
						return true
					}
				case RESEMBLES:
					if SortStringByWords(str) == strings.ToLower(val) {
						return true
					}
				default:
				}
			case ISEQUALTO, DIFFERSFROM:
				// conditional argument is element specifier
				if constraint.Parent != "" || constraint.Match != "" || constraint.Attrib != "" {
					ch := val[0]
					// pound, percent, and caret prefixes supported (undocumented)
					switch ch {
					case '#':
						count := 0
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							count++
						})
						val = strconv.Itoa(count)
					case '%':
						length := 0
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							if stn != "" {
								length += len(stn)
							}
						})
						val = strconv.Itoa(length)
					case '^':
						depth := 0
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							depth = lvl
						})
						val = strconv.Itoa(depth)
					default:
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							if stn != "" {
								val = stn
							}
						})
					}
				}
				str = strings.ToUpper(str)
				val = strings.ToUpper(val)

				switch stat {
				case ISEQUALTO:
					if str == val {
						return true
					}
				case DIFFERSFROM:
					if str != val {
						return true
					}
				default:
				}
			case GT, GE, LT, LE, EQ, NE:
				// second argument of numeric test can be element specifier
				if constraint.Parent != "" || constraint.Match != "" || constraint.Attrib != "" {
					ch := val[0]
					// pound, percent, and caret prefixes supported as potentially useful for data QA (undocumented)
					switch ch {
					case '#':
						count := 0
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							count++
						})
						val = strconv.Itoa(count)
					case '%':
						length := 0
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							if stn != "" {
								length += len(stn)
							}
						})
						val = strconv.Itoa(length)
					case '^':
						depth := 0
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							depth = lvl
						})
						val = strconv.Itoa(depth)
					case '&':
						if len(val) > 1 {
							val = val[1:]
							// expand variable to get actual tag
							val = variables[val]
						}
					default:
						ExploreElements(curr, mask, constraint.Parent, constraint.Match, constraint.Attrib, constraint.Wild, true, level, func(stn string, lvl int) {
							if stn != "" {
								_, errz := strconv.Atoi(stn)
								if errz == nil {
									val = stn
								}
							}
						})
					}
				}

				// numeric tests on element values
				x, errx := strconv.Atoi(str)
				y, erry := strconv.Atoi(val)

				// both arguments must resolve to integers
				if errx != nil || erry != nil {
					return false
				}

				switch stat {
				case GT:
					if x > y {
						return true
					}
				case GE:
					if x >= y {
						return true
					}
				case LT:
					if x < y {
						return true
					}
				case LE:
					if x <= y {
						return true
					}
				case EQ:
					if x == y {
						return true
					}
				case NE:
					if x != y {
						return true
					}
				default:
				}
			default:
			}

			return false
		}

		// checkConstraint applies optional [min:max] range restriction and sends result to testConstraint
		checkConstraint := func(str string) bool {

			// handle usual situation with no range first
			if norm {
				return testConstraint(str)
			}

			// check for [after|before] variant
			if typL == STRINGRANGE || typR == STRINGRANGE {
				if strL != "" {
					// use case-insensitive test
					strL = strings.ToUpper(strL)
					idx := strings.Index(strings.ToUpper(str), strL)
					if idx < 0 {
						// specified substring must be present in original string
						return false
					}
					ln := len(strL)
					// remove leading text
					str = str[idx+ln:]
				}
				if strR != "" {
					strR = strings.ToUpper(strR)
					idx := strings.Index(strings.ToUpper(str), strR)
					if idx < 0 {
						// specified substring must be present in remaining string
						return false
					}
					// remove trailing text
					str = str[:idx]
				}
				if str != "" {
					return testConstraint(str)
				}
				return false
			}

			min := 0
			max := 0

			// slice arguments use variable value +- adjustment or integer constant
			if typL == VARIABLERANGE {
				if strL == "" {
					return false
				}
				lft, ok := variables[strL]
				if !ok {
					return false
				}
				val, err := strconv.Atoi(lft)
				if err != nil {
					return false
				}
				// range argument values are inclusive and 1-based, decrement variable start +- offset to use in slice
				min = val + intL - 1
			} else if typL == INTEGERRANGE {
				// range argument values are inclusive and 1-based, decrement literal start to use in slice
				min = intL - 1
			}
			if typR == VARIABLERANGE {
				if strR == "" {
					return false
				}
				rgt, ok := variables[strR]
				if !ok {
					return false
				}
				val, err := strconv.Atoi(rgt)
				if err != nil {
					return false
				}
				if val+intR < 0 {
					// negative value is 1-based inset from end of string (undocumented)
					max = len(str) + val + intR + 1
				} else {
					max = val + intR
				}
			} else if typR == INTEGERRANGE {
				if intR < 0 {
					// negative max is inset from end of string (undocumented)
					max = len(str) + intR + 1
				} else {
					max = intR
				}
			}

			// numeric range now calculated, apply slice to string
			if min == 0 && max == 0 {
				return testConstraint(str)
			} else if max == 0 {
				if min > 0 && min < len(str) {
					str = str[min:]
					if str != "" {
						return testConstraint(str)
					}
				}
			} else if min == 0 {
				if max > 0 && max <= len(str) {
					str = str[:max]
					if str != "" {
						return testConstraint(str)
					}
				}
			} else {
				if min < max && min > 0 && max <= len(str) {
					str = str[min:max]
					if str != "" {
						return testConstraint(str)
					}
				}
			}

			return false
		}

		switch status {
		case ELEMENT:
			exploreElements(func(str string, lvl int) {
				// match to XML container object sends empty string, so do not check for str != "" here
				// test every selected element individually if value is specified
				if constraint == nil || checkConstraint(str) {
					found = true
				}
			})
		case VARIABLE:
			// use value of stored variable
			str, ok := variables[match]
			if ok {
				//  -if &VARIABLE -equals VALUE is the supported construct
				if constraint == nil || checkConstraint(str) {
					found = true
				}
			}
		case COUNT:
			count := 0

			exploreElements(func(str string, lvl int) {
				count++
				found = true
			})

			// number of element objects
			number = strconv.Itoa(count)
		case LENGTH:
			length := 0

			exploreElements(func(str string, lvl int) {
				length += len(str)
				found = true
			})

			// length of element strings
			number = strconv.Itoa(length)
		case DEPTH:
			depth := 0

			exploreElements(func(str string, lvl int) {
				depth = lvl
				found = true
			})

			// depth of last element in scope
			number = strconv.Itoa(depth)
		case INDEX:
			// index of explored parent object
			number = strconv.Itoa(index)
			found = true
		default:
		}

		if number == "" {
			return found
		}

		if constraint == nil || checkConstraint(number) {
			return true
		}

		return false
	}

	// test conditional arguments
	for _, op := range conditions {

		switch op.Type {
		// -if tests for presence of element (deprecated -match can test element:value)
		case SELECT, IF, MATCH:
			// checking for failure here allows for multiple -if [ -and / -or ] clauses
			if isMatch && observed < required {
				return false
			}
			if isAvoid && forbidden > 0 {
				return false
			}
			required = 0
			observed = 0
			forbidden = 0
			isMatch = true
			isAvoid = false
			// continue on to next two cases
			fallthrough
		case AND:
			required++
			// continue on to next case
			fallthrough
		case OR:
			if matchFound(op.Stages) {
				observed++
				// record presence of forbidden element if in -unless clause
				forbidden++
			}
		// -unless tests for absence of element, or presence but with failure of subsequent value test (deprecated -avoid can test element:value)
		case UNLESS, AVOID:
			if isMatch && observed < required {
				return false
			}
			if isAvoid && forbidden > 0 {
				return false
			}
			required = 0
			observed = 0
			forbidden = 0
			isMatch = false
			isAvoid = true
			if matchFound(op.Stages) {
				forbidden++
			}
		default:
		}
	}

	if isMatch && observed < required {
		return false
	}
	if isAvoid && forbidden > 0 {
		return false
	}

	return true
}

// RECURSIVELY PROCESS EXPLORATION COMMANDS AND XML DATA STRUCTURE

// processCommands visits XML nodes, performs conditional tests, and executes data extraction instructions
func processCommands(
	cmds *Block,
	curr *XMLNode,
	tab string,
	ret string,
	index int,
	level int,
	variables map[string]string,
	transform map[string]string,
	srchr *FSMSearcher,
	histogram map[string]int,
	accum func(string),
) (string, string) {

	if accum == nil {
		return tab, ret
	}

	prnt := cmds.Parent
	match := cmds.Match

	// closure passes local variables to callback, which can modify caller tab and ret values
	processNode := func(node *XMLNode, idx, lvl int) {

		// apply -if or -unless tests
		if conditionsAreSatisfied(cmds.Conditions, node, match, idx, lvl, variables) {

			// execute data extraction commands
			if len(cmds.Commands) > 0 {
				tab, ret = processInstructions(cmds.Commands, node, match, tab, ret, idx, lvl, variables, transform, srchr, histogram, accum)
			}

			// process sub commands on child node
			for _, sub := range cmds.Subtasks {
				tab, ret = processCommands(sub, node, tab, ret, 1, lvl, variables, transform, srchr, histogram, accum)
			}

		} else {

			// execute commands after -else statement
			if len(cmds.Failure) > 0 {
				tab, ret = processInstructions(cmds.Failure, node, match, tab, ret, idx, lvl, variables, transform, srchr, histogram, accum)
			}
		}
	}

	// explorePath recursive definition
	var explorePath func(*XMLNode, []string, int, int, func(*XMLNode, int, int)) int

	// explorePath visits child nodes and matches against next entry in path
	explorePath = func(curr *XMLNode, path []string, indx, levl int, proc func(*XMLNode, int, int)) int {

		if curr == nil || proc == nil {
			return indx
		}

		if len(path) < 1 {
			proc(curr, indx, levl)
			indx++
			return indx
		}

		name := path[0]
		rest := path[1:]

		// explore next level of child nodes
		for chld := curr.Children; chld != nil; chld = chld.Next {
			if chld.Name == name {
				// recurse only if child matches next component in path
				indx = explorePath(chld, rest, indx, levl+1, proc)
			}
		}

		return indx
	}

	if cmds.Foreword != "" {
		accum(cmds.Foreword)
	}

	// apply -position test

	if cmds.Position == "" || cmds.Position == "all" {

		ExploreNodes(curr, prnt, match, index, level, processNode)

	} else if cmds.Position == "path" {

		ExploreNodes(curr, prnt, match, index, level,
			func(node *XMLNode, idx, lvl int) {
				// exploreNodes callback has matched first path component, now explore remainder one level and component at a time
				explorePath(node, cmds.Path, idx, lvl, processNode)
			})

	} else {

		var single *XMLNode
		lev := 0
		ind := 0

		if cmds.Position == "first" {

			ExploreNodes(curr, prnt, match, index, level,
				func(node *XMLNode, idx, lvl int) {
					if single == nil {
						single = node
						ind = idx
						lev = lvl
					}
				})

		} else if cmds.Position == "last" {

			ExploreNodes(curr, prnt, match, index, level,
				func(node *XMLNode, idx, lvl int) {
					single = node
					ind = idx
					lev = lvl
				})

		} else if cmds.Position == "outer" {

			// print only first and last nodes
			var beg *Limiter
			var end *Limiter

			ExploreNodes(curr, prnt, match, index, level,
				func(node *XMLNode, idx, lvl int) {
					if beg == nil {
						beg = &Limiter{node, idx, lvl}
					} else {
						end = &Limiter{node, idx, lvl}
					}
				})

			if beg != nil {
				processNode(beg.Obj, beg.Idx, beg.Lvl)
			}
			if end != nil {
				processNode(end.Obj, end.Idx, end.Lvl)
			}

		} else if cmds.Position == "inner" {

			// print all but first and last nodes
			var prev *Limiter
			var next *Limiter
			first := true

			ExploreNodes(curr, prnt, match, index, level,
				func(node *XMLNode, idx, lvl int) {
					if first {
						first = false
						return
					}

					prev = next
					next = &Limiter{node, idx, lvl}

					if prev != nil {
						processNode(prev.Obj, prev.Idx, prev.Lvl)
					}
				})

		} else if cmds.Position == "even" {

			even := false

			ExploreNodes(curr, prnt, match, index, level,
				func(node *XMLNode, idx, lvl int) {
					if even {
						processNode(node, idx, lvl)
					}
					even = !even
				})

		} else if cmds.Position == "odd" {

			odd := true

			ExploreNodes(curr, prnt, match, index, level,
				func(node *XMLNode, idx, lvl int) {
					if odd {
						processNode(node, idx, lvl)
					}
					odd = !odd
				})

		} else {

			// use numeric position
			number, err := strconv.Atoi(cmds.Position)
			if err == nil {

				pos := 0

				ExploreNodes(curr, prnt, match, index, level,
					func(node *XMLNode, idx, lvl int) {
						pos++
						if pos == number {
							single = node
							ind = idx
							lev = lvl
						}
					})

			} else {

				DisplayError("Unrecognized position '%s'", cmds.Position)
				os.Exit(1)
			}
		}

		if single != nil {
			processNode(single, ind, lev)
		}
	}

	if cmds.Afterword != "" {
		accum(cmds.Afterword)
	}

	return tab, ret
}

// PROCESS ONE XML COMPONENT RECORD

// ProcessExtract perform data extraction driven by command-line arguments
func ProcessExtract(text, parent string, index int, hd, tl string, transform map[string]string, srchr *FSMSearcher, histogram map[string]int, cmds *Block) string {

	if text == "" || cmds == nil {
		return ""
	}

	// exit from function will collect garbage of node structure for current XML object
	pat := ParseRecord(text, parent)

	if pat == nil {
		return ""
	}

	// exit from function will also free map of recorded variables for current -pattern
	variables := make(map[string]string)

	var buffer strings.Builder

	ok := false

	if hd != "" {
		buffer.WriteString(hd)
	}

	ret := ""

	if cmds.Position == "select" {

		if conditionsAreSatisfied(cmds.Conditions, pat, cmds.Match, index, 1, variables) {
			ok = true
			buffer.WriteString(text)
			ret = "\n"
		}

	} else {

		// start processing at top of command tree and top of XML subregion selected by -pattern
		_, ret = processCommands(cmds, pat, "", "", index, 1, variables, transform, srchr, histogram,
			func(str string) {
				if str != "" {
					ok = true
					buffer.WriteString(str)
				}
			})
	}

	if tl != "" {
		buffer.WriteString(tl)
	}

	if ret != "" {
		ok = true
		buffer.WriteString(ret)
	}

	txt := buffer.String()

	// remove leading newline (-insd -pfx artifact)
	if txt != "" && txt[0] == '\n' {
		txt = txt[1:]
	}

	if !ok {
		return ""
	}

	// return consolidated result string
	return txt
}

// XMLtoData applies xtract logic to XML string
func XMLtoData(xml string, args []string) string {

	if xml == "" {
		return ""
	}

	if len(args) < 1 {
		DisplayError("Insufficient command-line arguments supplied to xtract")
		os.Exit(1)
	}

	head := ""
	tail := ""

	hd := ""
	tl := ""

	inSwitch := false

	for {

		inSwitch = true

		switch args[0] {
		case "-head":
			if len(args) < 2 {
				DisplayError("Pattern missing after -head command")
				os.Exit(1)
			}
			head = ConvertSlash(args[1])
			// allow splitting of -head argument, keep appending until next command (undocumented)
			ofs, nxt := 0, args[2:]
			for {
				if len(nxt) < 1 {
					break
				}
				tmp := nxt[0]
				if strings.HasPrefix(tmp, "-") {
					break
				}
				ofs++
				txt := ConvertSlash(tmp)
				if head != "" && !strings.HasSuffix(head, "\t") {
					head += "\t"
				}
				head += txt
				nxt = nxt[1:]
			}
			if ofs > 0 {
				args = args[ofs:]
			}
		case "-tail":
			if len(args) < 2 {
				DisplayError("Pattern missing after -tail command")
				os.Exit(1)
			}
			tail = ConvertSlash(args[1])
		case "-hd":
			if len(args) < 2 {
				DisplayError("Pattern missing after -hd command")
				os.Exit(1)
			}
			hd = ConvertSlash(args[1])
		case "-tl":
			if len(args) < 2 {
				DisplayError("Pattern missing after -tl command")
				os.Exit(1)
			}
			tl = ConvertSlash(args[1])
		case "-wrp":
			// shortcut to wrap records in XML tags
			if len(args) < 2 {
				DisplayError("Pattern missing after -wrp command")
				os.Exit(1)
			}
			tmp := ConvertSlash(args[1])
			lft, rgt := SplitInTwoLeft(tmp, ",")
			if lft != "" {
				head = "<" + lft + ">"
				tail = "</" + lft + ">"
			}
			if rgt != "" {
				hd = "<" + rgt + ">"
				tl = "</" + rgt + ">"
			}
		case "-set":
			if len(args) < 2 {
				DisplayError("Pattern missing after -set command")
				os.Exit(1)
			}
			tmp := ConvertSlash(args[1])
			if tmp != "" {
				head = "<" + tmp + ">"
				tail = "</" + tmp + ">"
			}
		case "-rec":
			if len(args) < 2 {
				DisplayError("Pattern missing after -rec command")
				os.Exit(1)
			}
			tmp := ConvertSlash(args[1])
			if tmp != "" {
				hd = "<" + tmp + ">"
				tl = "</" + tmp + ">"
			}
		default:
			// if not any of the controls, set flag to break out of for loop
			inSwitch = false
		}

		if !inSwitch {
			break
		}

		// skip past arguments
		args = args[2:]

		if len(args) < 1 {
			DisplayError("Insufficient command-line arguments supplied to xtract")
			os.Exit(1)
		}
	}

	// allow -record as synonym of -pattern (undocumented)
	if args[0] == "-record" || args[0] == "-Record" {
		args[0] = "-pattern"
	}

	// make sure top-level -pattern command is next
	if args[0] != "-pattern" && args[0] != "-Pattern" {
		DisplayError("No -pattern in command-line arguments")
		os.Exit(1)
	}
	if len(args) < 2 {
		DisplayError("Item missing after -pattern command")
		os.Exit(1)
	}

	topPat := args[1]
	if topPat == "" {
		DisplayError("Item missing after -pattern command")
		os.Exit(1)
	}
	if strings.HasPrefix(topPat, "-") {
		DisplayError("Misplaced %s command", topPat)
		os.Exit(1)
	}

	// look for -pattern Parent/* construct for heterogeneous data, e.g., -pattern PubmedArticleSet/*
	topPattern, star := SplitInTwoLeft(topPat, "/")
	if topPattern == "" {
		return ""
	}

	parent := ""
	if star == "*" {
		parent = topPattern
	} else if star != "" {
		DisplayError("-pattern Parent/Child construct is not supported")
		os.Exit(1)
	}

	turbo := false

	transform := make(map[string]string)

	// parse nested exploration instruction from command-line arguments
	cmds := ParseArguments(args, topPattern)
	if cmds == nil {
		DisplayError("Problem parsing command-line arguments")
		os.Exit(1)
	}

	// GLOBAL MAP FOR SORT-UNIQ-COUNT HISTOGRAM ARGUMENT

	histogram := make(map[string]int)

	// LAUNCH PRODUCER, CONSUMER, AND UNSHUFFLER GOROUTINES

	// launch producer goroutine to partition XML by pattern
	xmlq := CreateXMLProducer(topPattern, star, turbo, CreateXMLStreamer(strings.NewReader(xml), nil))

	// launch consumer goroutines to parse and explore partitioned XML objects
	tblq := CreateXMLConsumers(cmds, parent, hd, tl, transform, false, histogram, xmlq)

	// launch unshuffler goroutine to restore order of results
	unsq := CreateXMLUnshuffler(tblq)

	if xmlq == nil || tblq == nil || unsq == nil {
		DisplayError("Unable to create servers")
		os.Exit(1)
	}

	// DRAIN OUTPUT CHANNEL TO EXECUTE EXTRACTION COMMANDS, RESTORE OUTPUT ORDER WITH HEAP

	var buffer strings.Builder
	okay := false
	last := ""

	if head != "" {
		buffer.WriteString(head[:])
		buffer.WriteString("\n")
	}

	for curr := range unsq {

		str := curr.Text

		if str == "" {
			continue
		}

		last = str
		buffer.WriteString(str)

		okay = true
	}

	// print -histogram results, if populated
	keys := slices.SortedFunc(maps.Keys(histogram), CompareAlphaOrNumericKeys)

	for _, str := range keys {

		count := histogram[str]

		val := strconv.Itoa(count)
		buffer.WriteString(val)
		buffer.WriteString("\t")
		buffer.WriteString(str)
		buffer.WriteString("\n")

		last = "\n"
		okay = true
	}

	if !strings.HasSuffix(last, "\n") {
		buffer.WriteString("\n")
	}

	if tail != "" {
		buffer.WriteString(tail[:])
		buffer.WriteString("\n")
	}

	txt := ""
	if okay {
		txt = buffer.String()
	}
	buffer.Reset()

	return txt
}
