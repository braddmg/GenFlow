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
// File Name:  phrase.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"fmt"
	"github.com/surgebase/porter2"
	"html"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// GetLocalArchivePaths returns Archive and Working paths taken from environment variable(s)
func GetLocalArchivePaths(db string) (string, string) {

	db = strings.TrimSpace(db)
	db = strings.TrimSuffix(db, "/")

	if db == "" {
		// if empty string, default to pubmed database
		db = "pubmed"
	}
	db = strings.ToLower(db)

	if strings.Index(db, " ") >= 0 {
		DisplayError("Database '%s' must not contain spaces", db)
		return "", ""
	}

	master, working, ok := "", "", false

	getEnvPaths := func(mstr, wrkg string) (string, string, bool) {

		ms := os.Getenv(mstr)
		if ms == "" {
			return "", "", false
		}

		wk := os.Getenv(wrkg)
		if wk == "" {
			wk = ms
		}

		if !strings.HasSuffix(ms, "/") {
			ms += "/"
		}
		if !strings.HasSuffix(wk, "/") {
			wk += "/"
		}

		return ms, wk, true
	}

	getConfigPaths := func(cnfg string) (string, string, bool) {

		/*
		   [pubmed]
		   ARCHIVE=/Volumes/archive/pubmed
		   WORKING=/Volumes/working/pubmed

		   [pmc]
		   ARCHIVE=/Volumes/archive/pmc
		   WORKING=/Volumes/working/pmc

		   [taxonomy]
		   ARCHIVE=/Volumes/archive/taxonomy
		   WORKING=/Volumes/working/taxonomy
		*/

		/*
		   <ConfigFile>
		     <pubmed>
		       <ARCHIVE>/Volumes/archive/pubmed</ARCHIVE>
		       <WORKING>/Volumes/working/pubmed</WORKING>
		     </pubmed>
		     <pmc>
		       <ARCHIVE>/Volumes/archive/pmc</ARCHIVE>
		       <WORKING>/Volumes/working/pmc</WORKING>
		     </pmc>
		     <taxonomy>
		       <ARCHIVE>/Volumes/archive/taxonomy</ARCHIVE>
		       <WORKING>/Volumes/working/taxonomy</WORKING>
		     </taxonomy>
		   </ConfigFile>
		*/

		cf := os.Getenv(cnfg)
		if cf == "" {
			return "", "", false
		}

		getIniFile := func(fileName string) string {

			if fileName == "" {
				return ""
			}

			inFile, err := os.Open(fileName)
			if err != nil {
				DisplayError("Unable to open configuration file '%s'", fileName)
				return ""
			}
			defer inFile.Close()

			inis := INIConverter(inFile)
			if inis == nil {
				DisplayError("Unable to create INI to XML converter")
				return ""
			}

			text := ChanToString(inis)

			text = strings.TrimSpace(text)

			return text
		}

		ms, wk := "", ""

		text := getIniFile(cf)
		if text != "" {

			pat := ParseRecord(text[:], "ConfigFile")

			VisitNodes(pat, db, func(sect *XMLNode) {

				VisitElements(sect, "ARCHIVE", "", func(str string) { ms = str })
				VisitElements(sect, "WORKING", "", func(str string) { wk = str })
			})
		}

		if ms == "" {
			return "", "", false
		}

		if wk == "" {
			wk = ms
		}

		if !strings.HasSuffix(ms, "/") {
			ms += "/"
		}
		if !strings.HasSuffix(wk, "/") {
			wk += "/"
		}

		return ms, wk, true
	}

	// first try environment variable pointing to configuration file
	master, working, ok = getConfigPaths("EDIRECT_LOCAL_CONFIG")
	if ok {
		return master, working
	}

	// new convention points to volume, which will have a subfolder for each specific database
	master, working, ok = getEnvPaths("EDIRECT_LOCAL_ARCHIVE", "EDIRECT_LOCAL_WORKING")

	// try older name of environment variable for primary path
	if !ok {
		master, working, ok = getEnvPaths("EDIRECT_LOCAL_MASTER", "EDIRECT_LOCAL_WORKING")
	}

	if ok {
		// append database name to volume
		return master + db + "/", working + db + "/"
	}

	// old convention has individual database-specific environment variables
	switch db {
	case "pubmed":
		master, working, ok = getEnvPaths("EDIRECT_PUBMED_MASTER", "EDIRECT_PUBMED_WORKING")
	case "pmc":
		master, working, ok = getEnvPaths("EDIRECT_PMC_MASTER", "EDIRECT_PMC_WORKING")
	case "taxonomy":
		master, working, ok = getEnvPaths("EDIRECT_TAXONOMY_MASTER", "EDIRECT_TAXONOMY_WORKING")
	}

	if ok {
		return master, working
	}

	return "", ""
}

type alias struct {
	table    map[string]string
	lock     sync.Mutex
	fpath    string
	isLoaded bool
}

// loadAliasTable should be called within a lock on the alias.lock mutex
func (a *alias) loadAliasTable(reverse, commas bool) {

	if a == nil || a.fpath == "" {
		return
	}

	if a.isLoaded {
		return
	}

	file, ferr := os.Open(a.fpath)

	if file != nil && ferr == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			str := scanner.Text()
			if str == "" {
				continue
			}
			cols := strings.SplitN(str, "\t", 2)
			if len(cols) != 2 {
				continue
			}

			cleanTerm := func(str string) string {
				str = CleanupQuery(str, false, true)
				parts := strings.FieldsFunc(str, func(c rune) bool {
					return (!unicode.IsLetter(c) && !unicode.IsDigit(c) && c != ',') || c > 127
				})
				str = strings.Join(parts, " ")
				if commas {
					str = strings.Replace(str, ",", " ", -1)
				}
				str = strings.ToLower(str)
				str = strings.TrimSpace(str)
				str = CompressRunsOfSpaces(str)
				return str
			}

			one := cleanTerm(cols[0])
			two := cleanTerm(cols[1])
			if reverse {
				a.table[two] = one
			} else {
				a.table[one] = two
			}
		}
	}

	file.Close()

	// set even if loading failed to prevent multiple attempts
	a.isLoaded = true
}

var journalAliases = map[string]string{
	"pnas":                  "proc natl acad sci u s a",
	"journal of immunology": "journal of immunology baltimore md 1950",
	"biorxiv":               "biorxiv the preprint server for biology",
	"biorxivorg":            "biorxiv the preprint server for biology",
}

var ptypAliases = map[string]string{
	"clinical trial phase 1":     "clinical trial phase i",
	"clinical trial phase 2":     "clinical trial phase ii",
	"clinical trial phase 3":     "clinical trial phase iii",
	"clinical trial phase 4":     "clinical trial phase iv",
	"clinical trial phase one":   "clinical trial phase i",
	"clinical trial phase two":   "clinical trial phase ii",
	"clinical trial phase three": "clinical trial phase iii",
	"clinical trial phase four":  "clinical trial phase iv",
}

var (
	meshName alias
	meshTree alias
)

func printTermCount(base, term, field string) int {

	data, _ := getPostingIDs(base, term, field, true, false)
	size := len(data)
	fmt.Fprintf(os.Stdout, "%d\t%s\n", size, term)

	return size
}

func printTermCounts(base, term, field string) int {

	pdlen := len(PostingDir(term))

	if len(term) < pdlen {
		DisplayError("Term count argument must be at least %d characters", pdlen)
		os.Exit(1)
	}

	if strings.Contains(term[:pdlen], "*") {
		DisplayError("Wildcard asterisk must not be in first %d characters", pdlen)
		os.Exit(1)
	}

	dpath, key := PostingPath(base, field, term, false)
	if dpath == "" {
		return 0
	}

	// schedule asynchronous fetching
	mi := readMasterIndexFuture(dpath, key, field)

	tl := readTermListFuture(dpath, key, field)

	// fetch master index and term list
	indx := <-mi

	trms := <-tl

	if indx == nil || len(indx) < 1 {
		return 0
	}

	if trms == nil || len(trms) < 1 {
		return 0
	}

	// master index is padded with phantom term and postings position
	numTerms := len(indx) - 1

	strs := make([]string, numTerms)
	if strs == nil || len(strs) < 1 {
		return 0
	}

	retlength := int32(len("\n"))

	// populate array of strings from term list
	for i, j := 0, 1; i < numTerms; i++ {
		from := indx[i].TermOffset
		to := indx[j].TermOffset - retlength
		j++
		txt := string(trms[from:to])
		strs[i] = txt
	}

	// change protecting underscore to space
	term = strings.Replace(term, "_", " ", -1)

	// flank pattern with start-of-string and end-of-string symbols
	pat := "^" + term + "$"

	// change asterisk in query to dot + star for regular expression
	pat = strings.Replace(pat, "*", ".*", -1)

	re, err := regexp.Compile(pat)

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return 0
	}

	count := 0

	for R, str := range strs {
		if re.MatchString(str) {
			offset := indx[R].PostOffset
			size := indx[R+1].PostOffset - offset
			fmt.Fprintf(os.Stdout, "%d\t%s\n", size/4, str)
			count++
		}
	}

	return count
}

func printTermPositions(base, term, field string) int {

	data, ofst := getPostingIDs(base, term, field, false, false)
	size := len(data)
	fmt.Fprintf(os.Stdout, "\n%d\t%s\n\n", size, term)

	for i := range len(data) {
		fmt.Fprintf(os.Stdout, "%12d\t", data[i])
		pos := ofst[i]
		sep := ""
		for j := range len(pos) {
			fmt.Fprintf(os.Stdout, "%s%d", sep, pos[j])
			sep = ","
		}
		fmt.Fprintf(os.Stdout, "\n")
	}

	return size
}

func parseField(db, str string) (string, string) {

	field := "TIAB"
	if db == "pmc" {
		field = "TEXT"
	}

	if strings.HasSuffix(str, "]") {
		pos := strings.Index(str, "[")
		if pos >= 0 {
			field = str[pos:]
			field = strings.TrimPrefix(field, "[")
			field = strings.TrimSuffix(field, "]")
			str = str[:pos]
			str = strings.TrimSpace(str)
		}
		switch field {
		case "NORM":
			field = "TIAB"
		case "STEM", "TIAB", "TITL", "ABST", "TEXT", "TERM":
		case "PIPE":
		default:
			str = strings.Replace(str, " ", "_", -1)
		}
	}

	return field, str
}

// QUERY EVALUATION FUNCTION

func evaluateQuery(base, db, phrase string, clauses []string, noStdout, isLink bool) (int, []int32) {

	if clauses == nil || clauses[0] == "" {
		return 0, nil
	}

	count := 0

	// flag set if no tildes, indicates no proximity tests in query
	noProx := true
	for _, tkn := range clauses {
		if strings.HasPrefix(tkn, "~") {
			noProx = false
		}
	}

	phrasePositions := func(pn, pm []uint16, dlt uint16) []uint16 {

		var arry []uint16

		ln, lm := len(pn), len(pm)

		q, r := 0, 0

		vn, vm := pn[q], pm[r]
		vnd := vn + dlt

		for {
			if vnd > vm {
				r++
				if r == lm {
					break
				}
				vm = pm[r]
			} else if vnd < vm {
				q++
				if q == ln {
					break
				}
				vn = pn[q]
				vnd = vn + dlt
			} else {
				// store position of first word in current growing phrase
				arry = append(arry, vn)
				q++
				r++
				if q == ln || r == lm {
					break
				}
				vn = pn[q]
				vm = pm[r]
				vnd = vn + dlt
			}
		}

		return arry
	}

	proximityPositions := func(pn, pm []uint16, dlt uint16) []uint16 {

		var arry []uint16

		ln, lm := len(pn), len(pm)

		q, r := 0, 0

		vn, vm := pn[q], pm[r]
		vnd := vn + dlt

		for {
			if vnd < vm {
				q++
				if q == ln {
					break
				}
				vn = pn[q]
				vnd = vn + dlt
			} else if vn < vm {
				// store position of first word in downstream phrase that passes proximity test
				arry = append(arry, vm)
				q++
				r++
				if q == ln || r == lm {
					break
				}
				vn = pn[q]
				vm = pm[r]
				vnd = vn + dlt
			} else {
				r++
				if r == lm {
					break
				}
				vm = pm[r]
			}
		}

		return arry
	}

	eval := func(str string) ([]int32, [][]uint16, int) {

		// extract optional [FIELD] qualifier
		field, str := parseField(db, str)

		if field == "PIPE" {
			// esearch -db pubmed -query "complement system proteins [MESH]" -pub clinical |
			// efetch -format uid | phrase-search -query "[PIPE] AND coagulation [TITL]"
			var data []int32
			// read UIDs from stdin
			uidq := CreateUIDReader(os.Stdin)
			for ext := range uidq {

				val, err := strconv.Atoi(ext.Text)
				if err != nil {
					DisplayError("Unrecognized UID %s", ext.Text)
					os.Exit(1)
				}

				data = append(data, int32(val))
			}
			// sort UIDs before returning
			slices.Sort(data)
			return data, nil, 0
		}

		words := strings.Fields(str)

		if words == nil || len(words) < 1 {
			return nil, nil, 0
		}

		// if no tilde proximity tests, and not building up phrase from multiple words,
		// no need to use more expensive position tests when calculating intersection
		if noProx && len(words) == 1 {
			term := words[0]
			if strings.HasPrefix(term, "+") {
				return nil, nil, 0
			}
			term = strings.Replace(term, "_", " ", -1)
			data, _ := getPostingIDs(base, term, field, true, isLink)
			count++
			return data, nil, 1
		}

		dist := 0

		var intersect []Arrays

		var futures []<-chan Arrays

		// schedule asynchronous fetching
		for _, term := range words {

			term = strings.Replace(term, "_", " ", -1)

			if strings.HasPrefix(term, "+") {
				dist += strings.Count(term, "+")
				// run of stop words or explicit plus signs skip past one or more words in phrase
				continue
			}

			fetch := postingIDsFuture(base, term, field, dist, isLink)

			futures = append(futures, fetch)

			dist++
		}

		runtime.Gosched()

		for _, chn := range futures {

			// fetch postings data
			fut := <-chn

			if len(fut.Data) < 1 {
				// bail if word not present
				return nil, nil, 0
			}

			// append posting and positions
			intersect = append(intersect, fut)

			runtime.Gosched()
		}

		if len(intersect) < 1 {
			return nil, nil, 0
		}

		// start phrase with first word
		data, ofst, dist := intersect[0].Data, intersect[0].Ofst, intersect[0].Dist+1

		if len(intersect) == 1 {
			return data, ofst, dist
		}

		for i := 1; i < len(intersect); i++ {

			// add subsequent words, keep starting positions of phrases that contain all words in proper position
			data, ofst = extendPositionalIDs(data, ofst, intersect[i].Data, intersect[i].Ofst, intersect[i].Dist, phrasePositions)
			if len(data) < 1 {
				// bail if phrase not present
				return nil, nil, 0
			}
			dist = intersect[i].Dist + 1
		}

		count += len(intersect)

		// return UIDs and all positions of current phrase
		return data, ofst, dist
	}

	prevTkn := ""

	nextToken := func() string {

		if len(clauses) < 1 {
			return ""
		}

		// remove next token from slice
		tkn := clauses[0]
		clauses = clauses[1:]

		if tkn == "(" && prevTkn != "" && prevTkn != "&" && prevTkn != "|" && prevTkn != "!" {
			DisplayError("Tokens '%s' and '%s' should be separated by AND, OR, or NOT", prevTkn, tkn)
			os.Exit(1)
		}

		if prevTkn == ")" && tkn != "" && tkn != "&" && tkn != "|" && tkn != "!" && tkn != ")" {
			DisplayError("Tokens '%s' and '%s' should be separated by AND, OR, or NOT", prevTkn, tkn)
			os.Exit(1)
		}

		prevTkn = tkn

		return tkn
	}

	// recursive definitions
	var fact func() ([]int32, [][]uint16, int, string)
	var prox func() ([]int32, string)
	var excl func() ([]int32, string)
	var term func() ([]int32, string)
	var expr func() ([]int32, string)

	fact = func() ([]int32, [][]uint16, int, string) {

		var (
			data  []int32
			ofst  [][]uint16
			delta int
			tkn   string
		)

		tkn = nextToken()

		if tkn == "(" {
			// recursively process expression in parentheses
			data, tkn = expr()
			if tkn == ")" {
				tkn = nextToken()
			} else {
				DisplayError("Expected ')' but received '%s'", tkn)
				os.Exit(1)
			}
		} else if tkn == ")" {
			DisplayError("Unexpected ')' token")
			os.Exit(1)
		} else if tkn == "&" || tkn == "|" || tkn == "!" {
			DisplayError("Unexpected operator '%s' in expression", tkn)
			os.Exit(1)
		} else if tkn == "" {
			DisplayError("Unexpected end of expression in '%s'", phrase)
			os.Exit(1)
		} else {
			// evaluate current phrase
			data, ofst, delta = eval(tkn)
			tkn = nextToken()
		}

		return data, ofst, delta, tkn
	}

	prox = func() ([]int32, string) {

		var (
			next []int32
			noff [][]uint16
			ndlt int
		)

		data, ofst, delta, tkn := fact()
		if len(data) < 1 {
			return nil, tkn
		}

		for strings.HasPrefix(tkn, "~") {
			dist := strings.Count(tkn, "~")
			next, noff, ndlt, tkn = fact()
			if len(next) < 1 {
				return nil, tkn
			}
			// next phrase must be within specified distance after the previous phrase
			data, ofst = extendPositionalIDs(data, ofst, next, noff, delta+dist, proximityPositions)
			if len(data) < 1 {
				return nil, tkn
			}
			delta = ndlt
		}

		return data, tkn
	}

	excl = func() ([]int32, string) {

		var next []int32

		data, tkn := prox()
		for tkn == "!" {
			next, tkn = prox()
			data = excludeIDs(data, next)
		}

		return data, tkn
	}

	term = func() ([]int32, string) {

		var next []int32

		data, tkn := excl()
		for tkn == "&" {
			next, tkn = excl()
			data = intersectIDs(data, next)
		}

		return data, tkn
	}

	expr = func() ([]int32, string) {

		var next []int32

		data, tkn := term()
		for tkn == "|" {
			next, tkn = term()
			data = combineIDs(data, next)
		}

		return data, tkn
	}

	// enter recursive descent parser
	result, tkn := expr()

	if tkn != "" {
		DisplayError("Unexpected token '%s' at end of expression", tkn)
		os.Exit(1)
	}

	// sort final result
	slices.Sort(result)

	if noStdout {
		return count, result
	}

	// use buffers to speed up uid printing
	var buffer strings.Builder

	wrtr := bufio.NewWriter(os.Stdout)

	for _, pmid := range result {
		val := strconv.Itoa(int(pmid))
		buffer.WriteString(val[:])
		buffer.WriteString("\n")
	}

	txt := buffer.String()
	if txt != "" {
		// print buffer
		wrtr.WriteString(txt[:])
	}

	wrtr.Flush()

	runtime.Gosched()

	return count, nil
}

// QUERY PARSING FUNCTIONS

func prepareQuery(str string) string {

	if str == "" {
		return ""
	}

	if strings.HasPrefix(str, "[PIPE]") {
		str = "stdin " + str
	}

	// dash before AUTH indicates no truncation, double asterisk survives until setFieldQualifiers
	if strings.HasSuffix(str, "- [AUTH]") || strings.HasSuffix(str, "-[AUTH]") {
		str = strings.Replace(str, "-", "**", -1)
	}
	// also support trailing underscore (undocumented)
	if strings.HasSuffix(str, "_ [AUTH]") || strings.HasSuffix(str, "_[AUTH]") {
		str = strings.Replace(str, "_", "**", -1)
	}

	removeInternalParentheses := func(str string) string {

		if len(str) < 3 {
			return str
		}

		var (
			buffer strings.Builder
		)

		saveOneRune := func(prev, curr, next rune) {

			if curr == '(' || curr == ')' {
				if prev != ' ' && prev != 0 && next != ' ' && next != 0 {
					curr = ' '
				}
			}

			buffer.WriteRune(curr)
		}

		arry := SlidingSlices(str, 3)

		first := true
		last := rune(0)
		for _, item := range arry {

			if len(item) < 3 {
				continue
			}
			prev, curr, next := rune(item[0]), rune(item[1]), rune(item[2])

			if first {
				saveOneRune(0, prev, 0)
				first = false
			}

			saveOneRune(prev, curr, next)

			last = next
		}
		saveOneRune(0, last, 0)

		return buffer.String()
	}

	isBoolean := false
	words := strings.Split(str, " ")
	for _, word := range words {
		switch word {
		case "AND", "OR", "NOT":
			isBoolean = true
		default:
		}
	}

	if isBoolean {
		// more care with parentheses in chemical names if query string also has Boolean operators
		str = removeInternalParentheses(str)
	} else {
		str = strings.Replace(str, "(", " ", -1)
		str = strings.Replace(str, ")", " ", -1)
	}

	str = html.UnescapeString(str)

	str = CleanupQuery(str, false, true)

	str = strings.Replace(str, "~ ~", "~~", -1)
	str = strings.Replace(str, "~ ~", "~~", -1)

	str = strings.TrimSpace(str)

	// temporarily flank with spaces to detect misplaced operators at ends
	str = " " + str + " "

	str = strings.Replace(str, " AND ", " & ", -1)
	str = strings.Replace(str, " OR ", " | ", -1)
	str = strings.Replace(str, " NOT ", " ! ", -1)

	str = strings.Replace(str, "(", " ( ", -1)
	str = strings.Replace(str, ")", " ) ", -1)
	str = strings.Replace(str, "&", " & ", -1)
	str = strings.Replace(str, "|", " | ", -1)
	str = strings.Replace(str, "!", " ! ", -1)

	// ensure that bracketed fields are flanked by spaces
	str = strings.Replace(str, "[", " [", -1)
	str = strings.Replace(str, "]", "] ", -1)

	// remove temporary flanking spaces
	str = strings.TrimSpace(str)

	str = strings.ToLower(str)

	str = strings.Replace(str, "_", " ", -1)

	if HasHyphenOrApostrophe(str) {
		str = FixSpecialCases(str)
	}

	str = strings.Replace(str, "-", " ", -1)

	str = strings.Replace(str, "'", "", -1)

	// allow links like pubmed_cited and pubmed_cites
	str = strings.Replace(str, "[pubmed ", "[pubmed_", -1)

	// break terms at punctuation, and at non-ASCII characters, allowing brackets for field names,
	// along with Boolean control symbols, underscore for protected terms, asterisk to indicate
	// truncation wildcard, tilde for maximum proximity, and plus sign for exactly one wildcard word
	terms := strings.FieldsFunc(str, func(c rune) bool {
		return (!unicode.IsLetter(c) && !unicode.IsDigit(c) &&
			c != '_' && c != '*' && c != '~' && c != '+' &&
			c != '$' && c != '&' && c != '|' && c != '!' &&
			c != '(' && c != ')' && c != '[' && c != ']') || c > 127
	})

	// rejoin into processed sentence
	tmp := strings.Join(terms, " ")

	tmp = CompressRunsOfSpaces(tmp)
	tmp = strings.TrimSpace(tmp)

	return tmp
}

func prepareExact(str, sfx string, deStop bool) string {

	if str == "" {
		return ""
	}

	if str == "[Not Available]." || str == "Health." {
		return ""
	}

	str = CleanupQuery(str, true, true)

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
		return (!unicode.IsLetter(c) && !unicode.IsDigit(c) && c != ' ' && c != '_') || c > 127
	})

	// space replaces plus sign to separate runs of unpunctuated words
	phrases := strings.Join(clauses, " ")

	var chain []string

	// break phrases into individual words
	words := strings.Fields(phrases)

	for _, item := range words {

		// skip at site of punctuation break
		if item == "+" {
			chain = append(chain, "+")
			continue
		}

		// skip if just a period, but allow terms that are all digits or period
		if item == "." {
			chain = append(chain, "+")
			continue
		}

		// optional stop word removal
		if deStop && IsStopWord(item) {
			chain = append(chain, "+")
			continue
		}

		// index single normalized term
		chain = append(chain, item)
	}

	// rejoin into processed sentence
	tmp := strings.Join(chain, " ")

	tmp = strings.Replace(tmp, "+ +", "++", -1)
	tmp = strings.Replace(tmp, "+ +", "++", -1)

	tmp = CompressRunsOfSpaces(tmp)
	tmp = strings.TrimSpace(tmp)

	if tmp != "" && !strings.HasSuffix(tmp, "]") {
		tmp += " " + sfx
	}

	return tmp
}

func processStopWords(str string, deStop bool) string {

	if str == "" {
		return ""
	}

	var chain []string

	terms := strings.Fields(str)

	nextField := func(terms []string) (string, int) {

		for j, item := range terms {
			if strings.HasPrefix(item, "[") && strings.HasSuffix(item, "]") {
				return strings.ToUpper(item), j + 1
			}
		}

		return "", 0
	}

	// replace unwanted and stop words with plus sign
	for len(terms) > 0 {

		item := terms[0]
		terms = terms[1:]

		fld, j := nextField(terms)

		stps := false
		rlxd := false
		switch fld {
		case "[NORM]":
			fld = "[TIAB]"
			fallthrough
		case "[TIAB]", "[TITL]", "[ABST]", "[TEXT]", "[TERM]":
			stps = true
		case "[STEM]":
			stps = true
			rlxd = true
		case "":
			stps = true
		default:
		}

		addOneTerm := func(itm string) {

			if stps {
				if itm == "." {
					// skip if just a period, but allow terms that are all digits or period
					chain = append(chain, "+")
				} else if deStop && IsStopWord(itm) {
					// skip if stop word, breaking phrase chain
					chain = append(chain, "+")
				} else if rlxd {
					isWildCard := strings.HasSuffix(itm, "*")
					if isWildCard {
						// temporarily remove trailing asterisk
						itm = strings.TrimSuffix(itm, "*")
					}

					itm = porter2.Stem(itm)
					itm = strings.TrimSpace(itm)

					if isWildCard {
						// do wildcard search in stemmed term list
						itm += "*"
					}
					chain = append(chain, itm)
				} else {
					// record single unmodified term
					chain = append(chain, itm)
				}
			} else {
				// do not treat non-TIAB terms as stop words
				chain = append(chain, itm)
			}
		}

		if j == 0 {
			// index single normalized term
			addOneTerm(item)
			continue
		}

		for j > 0 {

			addOneTerm(item)

			j--
			item = terms[0]
			terms = terms[1:]
		}

		if fld != "" {
			chain = append(chain, fld)
		}
	}

	// rejoin into processed sentence
	tmp := strings.Join(chain, " ")

	tmp = strings.Replace(tmp, "+ +", "++", -1)
	tmp = strings.Replace(tmp, "+ +", "++", -1)

	tmp = strings.Replace(tmp, "~ +", "~+", -1)
	tmp = strings.Replace(tmp, "+ ~", "+~", -1)

	for strings.Contains(tmp, "~+") {
		tmp = strings.Replace(tmp, "~+", "~~", -1)
	}
	for strings.Contains(tmp, "+~") {
		tmp = strings.Replace(tmp, "+~", "~~", -1)
	}

	tmp = CompressRunsOfSpaces(tmp)
	tmp = strings.TrimSpace(tmp)

	return tmp
}

func partitionQuery(str string) []string {

	if str == "" {
		return nil
	}

	str = CompressRunsOfSpaces(str)
	str = strings.TrimSpace(str)

	str = " " + str + " "

	// flank all operators with caret
	str = strings.Replace(str, " ( ", " ^ ( ^ ", -1)
	str = strings.Replace(str, " ) ", " ^ ) ^ ", -1)
	str = strings.Replace(str, " & ", " ^ & ^ ", -1)
	str = strings.Replace(str, " | ", " ^ | ^ ", -1)
	str = strings.Replace(str, " ! ", " ^ ! ^ ", -1)
	str = strings.Replace(str, " ~", " ^ ~", -1)
	str = strings.Replace(str, "~ ", "~ ^ ", -1)

	str = CompressRunsOfSpaces(str)
	str = strings.TrimSpace(str)

	str = strings.Replace(str, "^ ^", "^", -1)

	if strings.HasPrefix(str, "^ ") {
		str = str[2:]
	}
	if strings.HasSuffix(str, " ^") {
		max := len(str)
		str = str[:max-2]
	}

	str = strings.Replace(str, "~ ^ +", "~+", -1)
	str = strings.Replace(str, "+ ^ ~", "+~", -1)

	str = strings.Replace(str, "~ +", "~+", -1)
	str = strings.Replace(str, "+ ~", "+~", -1)

	for strings.Contains(str, "~+") {
		str = strings.Replace(str, "~+", "~~", -1)
	}
	for strings.Contains(str, "+~") {
		str = strings.Replace(str, "+~", "~~", -1)
	}

	// split into non-broken phrase segments or operator symbols
	tmp := strings.Split(str, " ^ ")

	return tmp
}

func setFieldQualifiers(db string, clauses []string) []string {

	var res []string

	if clauses == nil {
		return nil
	}

	for _, str := range clauses {

		// pass control symbols unchanged
		if str == "(" || str == ")" || str == "&" || str == "|" || str == "!" || strings.HasPrefix(str, "~") {
			res = append(res, str)
			continue
		}

		// pass angle bracket content delimiters (for -phrase, -require, -exclude)
		if str == "<" || str == ">" {
			res = append(res, str)
			continue
		}

		if strings.HasSuffix(str, " [YEAR]") {

			slen := len(str)
			str = str[:slen-7]

			// regular 4-digit year
			if len(str) == 4 && IsAllDigitsOrPeriod(str) {
				res = append(res, str+" [YEAR]")
				continue
			}

			// check for year wildcard
			if len(str) == 4 && str[3] == '*' && IsAllDigitsOrPeriod(str[:3]) {

				DisplayError("Wildcards not supported - use ####:#### range instead")
				os.Exit(1)
			}

			// allow year month day to look for unexpected annotation
			if len(str) > 9 {
				res = append(res, str+" [YEAR]")
				continue
			}

			// check for year range
			if len(str) == 9 && str[4] == ' ' && IsAllDigitsOrPeriod(str[:4]) && IsAllDigitsOrPeriod(str[5:]) {
				start, err := strconv.Atoi(str[:4])
				if err != nil {
					DisplayError("Unable to recognize first year '%s'", str[:4])
					os.Exit(1)
				}
				stop, err := strconv.Atoi(str[5:])
				if err != nil {
					DisplayError("Unable to recognize final year '%s'", str[5:])
					os.Exit(1)
				}
				if start > stop {
					continue
				}
				// expand year range into individual year-by-year queries
				pfx := "("
				sfx := ")"
				for start <= stop {
					res = append(res, pfx)
					pfx = "|"
					yr := strconv.Itoa(start)
					res = append(res, yr+" [year]")
					start++
				}
				res = append(res, sfx)
				continue
			}

			DisplayError("Unable to recognize year expression '%s'", str)
			os.Exit(1)

		} else if strings.HasSuffix(str, " [AUTH]") ||
			strings.HasSuffix(str, " [FAUT]") ||
			strings.HasSuffix(str, " [LAUT]") {

			slen := len(str)
			fld := str[slen-7:]
			str = str[:slen-7]

			str = strings.TrimSpace(str)
			if str == "" {
				continue
			}
			fld = strings.TrimSpace(fld)

			// double asterisk was converted from dash to indicate exact match
			if strings.HasSuffix(str, "**") {
				str = strings.TrimSuffix(str, "**")
				str = strings.TrimSpace(str)
				res = append(res, str+" "+fld)
				continue
			}

			// if already has wildcard, leave in place
			if strings.Index(str, "*") >= 0 {
				res = append(res, str+" "+fld)
				continue
			}

			// if just last name, space plus asterisk to wildcard on any initials
			if strings.Index(str, " ") < 0 {
				res = append(res, str+" * "+fld)
				continue
			}

			// otherwise, if space between last name and initials, immediate asterisk for all subsequent initials
			res = append(res, str+"* "+fld)
			continue

		} else if strings.HasSuffix(str, " [ANUM]") ||
			strings.HasSuffix(str, " [INUM]") ||
			strings.HasSuffix(str, " [FNUM]") ||
			strings.HasSuffix(str, " [TLEN]") ||
			strings.HasSuffix(str, " [TNUM]") {

			slen := len(str)
			bdy := str[:slen-7]
			fld := str[slen-7:]
			bdy = strings.TrimSpace(bdy)
			fld = strings.TrimSpace(fld)

			// look for remnant of colon separating two integers
			lft, rgt := SplitInTwoLeft(bdy, " ")
			lft = strings.TrimSpace(lft)
			rgt = strings.TrimSpace(rgt)

			if lft == "" && rgt == "" {
				DisplayError("Unable to recognize expression '%s'", str)
				os.Exit(1)
			}

			// regular integer
			if rgt == "" {
				// check for wildcard
				if strings.HasSuffix(lft, "*") {

					DisplayError("Wildcards not supported - use #:# range instead")
					os.Exit(1)
				}
				if IsAllDigits(lft) {
					res = append(res, str)
					continue
				}
				DisplayError("Field %s must be an integer", fld)
				os.Exit(1)
			}

			// check for integer range
			if !IsAllDigits(lft) || !IsAllDigits(rgt) {
				DisplayError("Unable to recognize expression '%s'", str)
				os.Exit(1)
			}

			start, err := strconv.Atoi(lft)
			if err != nil {
				DisplayError("Unable to recognize starting number '%s'", lft)
				os.Exit(1)
			}
			stop, err := strconv.Atoi(rgt)
			if err != nil {
				DisplayError("Unable to recognize ending number '%s'", rgt)
				os.Exit(1)
			}
			if start > stop {
				// put into proper order
				start, stop = stop, start
			}
			// expand range into individual number-by-number queries
			fld = strings.ToLower(fld)
			pfx := "("
			sfx := ")"
			for start <= stop {
				res = append(res, pfx)
				pfx = "|"
				yr := strconv.Itoa(start)
				res = append(res, yr+" "+fld)
				start++
			}
			res = append(res, sfx)
			continue

		} else if strings.HasSuffix(str, " [TREE]") {

			slen := len(str)
			str = str[:slen-7]

			if db == "pubmed" {

				// pad if top-level mesh tree wildcard uses four character trie
				if len(str) == 4 && str[3] == '*' {
					key := str[:2]
					num, ok := TrieLen[key]
					if ok && num > 3 {
						str = str[0:3] + " " + "*"
					}
				}

				str = strings.Replace(str, " ", ".", -1)
				tmp := str
				tmp = strings.TrimSuffix(tmp, "*")
				if len(tmp) > 2 && unicode.IsLower(rune(tmp[0])) && IsAllDigitsOrPeriod(tmp[1:]) {
					str = strings.Replace(str, ".", " ", -1)
					res = append(res, str+" [TREE]")
					continue
				}

				DisplayError("Unable to recognize mesh code expression '%s'", str)
				os.Exit(1)

			} else if db == "taxonomy" {

				// e.g., "Eukaryota Metazoa Chordata Mammalia Primates * [TREE]"

				res = append(res, str+" [TREE]")
				continue
			}

		} else if strings.HasSuffix(str, " [JOUR]") {

			slen := len(str)
			str = str[:slen-7]

			// check hard-coded journal alias map (would be better to use map from Data/jourindx.txt)
			alias, ok := journalAliases[str]
			if ok {
				res = append(res, alias+" [JOUR]")
				continue
			}

			// no alias found, use as is
			res = append(res, str+" [JOUR]")
			continue

		} else if strings.HasSuffix(str, " [PTYP]") {

			slen := len(str)
			str = str[:slen-7]

			// convert clinical trial phase with arabic numeral or english word to roman numeral
			alias, ok := ptypAliases[str]
			if ok {
				res = append(res, alias+" [PTYP]")
				continue
			}

			// no alias found, use as is
			res = append(res, str+" [PTYP]")
			continue

		} else if strings.HasSuffix(str, " [DOI]") {

			slen := len(str)
			str = str[:slen-6]

			rev := ReverseString(str)
			res = append(res, rev+" [DOI]")
			continue

		} else if strings.HasSuffix(str, " [MESH]") {

			slen := len(str)
			str = str[:slen-7]

			if meshName.fpath == "" || meshTree.fpath == "" {
				base, _ := GetLocalArchivePaths("pubmed")
				if base != "" {
					if meshName.fpath == "" {
						meshName.fpath = filepath.Join(base, "Data", "meshname.txt")
					}
					if meshTree.fpath == "" {
						meshTree.fpath = filepath.Join(base, "Data", "meshtree.txt")
					}
				}
			}

			// load mesh tables within mutexes
			meshName.lock.Lock()
			if !meshName.isLoaded {
				meshName.loadAliasTable(true, true)
			}
			meshName.lock.Unlock()

			meshTree.lock.Lock()
			if !meshTree.isLoaded {
				meshTree.loadAliasTable(false, false)
			}
			meshTree.lock.Unlock()

			// check mesh alias tables
			if meshName.isLoaded && meshTree.isLoaded {
				code, ok := meshName.table[str]
				if ok {
					cluster, ok := meshTree.table[code]
					if ok {
						if strings.Index(cluster, ",") < 0 {
							res = append(res, cluster+"* [TREE]")
							continue
						}
						trees := strings.Split(cluster, ",")
						// expand multiple trees in OR group
						pfx := "("
						sfx := ")"
						for _, tr := range trees {
							res = append(res, pfx)
							pfx = "|"
							tr = strings.TrimSpace(tr)
							res = append(res, tr+"* [TREE]")
						}
						res = append(res, sfx)
						continue
					} else {
						res = append(res, code+" [CODE]")
						continue
					}
				}
			}

			// skip if MeSH term not yet indexed in tree
			continue
		}

		// remove leading and trailing plus signs and spaces
		for strings.HasPrefix(str, "+") || strings.HasPrefix(str, " ") {
			str = str[1:]
		}
		for strings.HasSuffix(str, "+") || strings.HasSuffix(str, " ") {
			slen := len(str)
			str = str[:slen-1]
		}

		res = append(res, str)
	}

	return res
}

// SEARCH TERM LISTS FOR PHRASES OR NORMALIZED TERMS, OR MATCH BY PATTERN

// ProcessSearch evaluates query, returns list of PMIDs to stdout
func ProcessSearch(db, phrase string, xact, titl, isLink, deStop bool) int {

	if phrase == "" {
		return 0
	}

	if db == "" {
		db = "pubmed"
	}
	db = strings.ToLower(db)

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	postingsBase := base + "Postings"

	if titl {
		phrase = prepareExact(phrase, "[titl]", deStop)
	} else if xact {
		if db == "pmc" {
			phrase = prepareExact(phrase, "[text]", deStop)
		} else {
			phrase = prepareExact(phrase, "[tiab]", deStop)
		}
	} else {
		phrase = prepareQuery(phrase)
	}

	phrase = processStopWords(phrase, deStop)

	clauses := partitionQuery(phrase)

	clauses = setFieldQualifiers(db, clauses)

	count, _ := evaluateQuery(postingsBase, db, phrase, clauses, false, isLink)

	return count
}

// ProcessQuery evaluates query, returns list of PMIDs in array
func ProcessQuery(db, phrase string, xact, titl, isLink, deStop bool) []int32 {

	if phrase == "" {
		return nil
	}

	if db == "" {
		db = "pubmed"
	}
	db = strings.ToLower(db)

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	postingsBase := base + "Postings"

	if titl {
		phrase = prepareExact(phrase, "[titl]", deStop)
	} else if xact {
		if db == "pmc" {
			phrase = prepareExact(phrase, "[text]", deStop)
		} else {
			phrase = prepareExact(phrase, "[tiab]", deStop)
		}
	} else {
		phrase = prepareQuery(phrase)
	}

	phrase = processStopWords(phrase, deStop)

	clauses := partitionQuery(phrase)

	clauses = setFieldQualifiers(db, clauses)

	_, arry := evaluateQuery(postingsBase, db, phrase, clauses, true, isLink)

	return arry
}

// ProcessMock shows individual steps in processing query for evaluation
func ProcessMock(db, phrase string, xact, titl, deStop bool) int {

	if phrase == "" {
		return 0
	}

	if db == "" {
		db = "pubmed"
	}
	db = strings.ToLower(db)

	fmt.Fprintf(os.Stdout, "processSearch:\n\n%s\n\n", phrase)

	if titl {
		phrase = prepareExact(phrase, "[titl]", deStop)

		fmt.Fprintf(os.Stdout, "prepareExact:\n\n%s\n\n", phrase)
	} else if xact {
		if db == "pmc" {
			phrase = prepareExact(phrase, "[text]", deStop)
		} else {
			phrase = prepareExact(phrase, "[tiab]", deStop)
		}

		fmt.Fprintf(os.Stdout, "prepareExact:\n\n%s\n\n", phrase)
	} else {
		phrase = prepareQuery(phrase)

		fmt.Fprintf(os.Stdout, "prepareQuery:\n\n%s\n\n", phrase)
	}

	phrase = processStopWords(phrase, deStop)

	fmt.Fprintf(os.Stdout, "processStopWords:\n\n%s\n\n", phrase)

	clauses := partitionQuery(phrase)

	fmt.Fprintf(os.Stdout, "partitionQuery:\n\n")
	for _, tkn := range clauses {
		fmt.Fprintf(os.Stdout, "%s\n", tkn)
	}
	fmt.Fprintf(os.Stdout, "\n")

	clauses = setFieldQualifiers(db, clauses)

	fmt.Fprintf(os.Stdout, "setFieldQualifiers:\n\n")
	for _, tkn := range clauses {
		fmt.Fprintf(os.Stdout, "%s\n", tkn)
	}
	fmt.Fprintf(os.Stdout, "\n")

	return 0
}

// ProcessCount prints document count for each term, also supports terminal wildcards
func ProcessCount(db, phrase string, plrl, psns, deStop bool) int {

	if phrase == "" {
		return 0
	}

	if db == "" {
		db = "pubmed"
	}
	db = strings.ToLower(db)

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	postingsBase := base + "Postings"

	phrase = prepareQuery(phrase)

	phrase = processStopWords(phrase, deStop)

	clauses := partitionQuery(phrase)

	clauses = setFieldQualifiers(db, clauses)

	if clauses == nil {
		return 0
	}

	count := 0

	splitIntoWords := func(str string) []string {

		if str == "" {
			return nil
		}

		var arry []string

		parts := strings.Split(str, "+")

		for _, segment := range parts {

			segment = strings.TrimSpace(segment)

			if segment == "" {
				continue
			}

			words := strings.Fields(segment)

			for _, item := range words {
				if strings.HasPrefix(item, "~") {
					continue
				}
				arry = append(arry, item)
			}
		}

		return arry
	}

	checkTermCounts := func(txt string) {

		field, str := parseField(db, txt)

		var words []string

		words = splitIntoWords(str)

		if words == nil || len(words) < 1 {
			return
		}

		for _, term := range words {

			term = strings.Replace(term, "_", " ", -1)

			if psns {
				count += printTermPositions(postingsBase, term, field)
			} else if plrl {
				count += printTermCounts(postingsBase, term, field)
			} else {
				count += printTermCount(postingsBase, term, field)
			}
		}
	}

	for _, item := range clauses {

		// skip control symbols
		if item == "(" || item == ")" || item == "&" || item == "|" || item == "!" {
			continue
		}

		checkTermCounts(item)
	}

	runtime.Gosched()

	return count
}

// TermCounts prints document counts for terms by subdirectory
func TermCounts(db, field, key, ttls string) int {

	if field == "" || key == "" || ttls == "" {
		return 0
	}

	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local postings path")
		os.Exit(1)
	}

	dpath := filepath.Join(base, "Postings", field, ttls)

	// schedule asynchronous fetching
	mi := readMasterIndexFuture(dpath, key, field)

	tl := readTermListFuture(dpath, key, field)

	// fetch master index and term list
	indx := <-mi

	trms := <-tl

	if indx == nil || len(indx) < 1 {
		return 0
	}

	if trms == nil || len(trms) < 1 {
		return 0
	}

	// master index is padded with phantom term and postings position
	numTerms := len(indx) - 1

	strs := make([]string, numTerms)
	if strs == nil || len(strs) < 1 {
		return 0
	}

	retlength := int32(len("\n"))

	// populate array of strings from term list
	for i, j := 0, 1; i < numTerms; i++ {
		from := indx[i].TermOffset
		to := indx[j].TermOffset - retlength
		j++
		txt := string(trms[from:to])
		strs[i] = txt
	}

	count := 0

	for R, str := range strs {
		offset := indx[R].PostOffset
		size := indx[R+1].PostOffset - offset
		fmt.Fprintf(os.Stdout, "%d\t%s\n", size/4, str)
		count++
	}

	return count
}

// ProcessLinks reads a list of PMIDs, merges resulting links
func ProcessLinks(db, fld string) {

	if fld == "" {
		return
	}

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	postingsBase := base + "Postings"

	// createLinkGrouper reads from UID reader and groups PMIDs under the same LinksTrie
	createLinkGrouper := func(base, fld string, inp <-chan XMLRecord) <-chan []string {

		if base == "" || fld == "" || inp == nil {
			return nil
		}

		out := make(chan []string, chanDepth)
		if out == nil {
			DisplayError("Unable to create link grouper channel")
			os.Exit(1)
		}

		linkGrouper := func(base, fld string, inp <-chan XMLRecord, out chan<- []string) {

			// report when grouper has no more records to process
			defer close(out)

			var arry []string

			currPfx := ""

			for ext := range inp {

				uid := ext.Text
				_, pfx := LinksTrie(uid, true)

				if pfx != currPfx && currPfx != "" {

					if arry != nil {
						// send group of PMIDs with the same line trie down the channel
						out <- arry
					}

					// empty the slice
					arry = nil
				}

				arry = append(arry, uid)

				currPfx = pfx
			}

			// send final results
			if arry != nil {
				// send group of PMIDs with the same line trie down the channel
				out <- arry
			}
		}

		// launch single link grouper goroutine
		go linkGrouper(base, fld, inp, out)

		return out
	}

	// mutex for link results
	var llock sync.RWMutex

	// map for combining link results
	combinedLinks := make(map[int]bool)

	createLinkMergers := func(prom, field string, inp <-chan []string) <-chan string {

		if prom == "" || field == "" || inp == nil {
			return nil
		}

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create link merger channel")
			os.Exit(1)
		}

		// linkMerge processes a set of terms from the same master index area
		linkMerge := func(wg *sync.WaitGroup, prom, field string, inp <-chan []string, out chan<- string) {

			// report when this matcher has no more records to process
			defer wg.Done()

			if inp == nil || out == nil {
				return
			}

			for terms := range inp {

				key := terms[0]

				dir, ky := LinksTrie(key, true)
				if dir == "" {
					continue
				}
				dpath := filepath.Join(prom, field, dir)
				if dpath == "" {
					continue
				}

				// schedule asynchronous fetching
				mi := readMasterIndexFuture(dpath, ky, field)

				tl := readTermListFuture(dpath, ky, field)

				// fetch master index and term list
				indx := <-mi

				trms := <-tl

				if indx == nil || len(indx) < 1 {
					continue
				}

				if trms == nil || len(trms) < 1 {
					continue
				}

				// master index is padded with phantom term and postings position
				numTerms := len(indx) - 1

				strs := make([]string, numTerms)
				if strs == nil || len(strs) < 1 {
					continue
				}

				retlength := int32(len("\n"))

				// populate array of strings from term list
				for i, j := 0, 1; i < numTerms; i++ {
					from := indx[i].TermOffset
					to := indx[j].TermOffset - retlength
					j++
					txt := string(trms[from:to])
					strs[i] = txt
				}

				postingsLoop := func(dpath, ky, field string) {

					inFile, _ := commonOpenFile(dpath, ky+"."+field+".pst")
					if inFile == nil {
						return
					}

					defer inFile.Close()

					for _, term := range terms {

						term = PadNumericID(term)

						// binary search in term list
						L, R := 0, numTerms-1
						for L < R {
							mid := (L + R) / 2
							if strs[mid] < term {
								L = mid + 1
							} else {
								R = mid
							}
						}

						linkLoop := func(offset, size int32) {

							data := make([]int32, size/4)
							if data == nil || len(data) < 1 {
								return
							}

							_, err := inFile.Seek(int64(offset), io.SeekStart)
							if err != nil {
								fmt.Fprintf(os.Stderr, "%s\n", err.Error())
								return
							}

							// read relevant postings list section
							err = binary.Read(inFile, binary.LittleEndian, data)
							if err != nil {
								fmt.Fprintf(os.Stderr, "%s\n", err.Error())
								return
							}

							if data == nil || len(data) < 1 {
								return
							}

							llock.Lock()

							for _, uid := range data {
								combinedLinks[int(uid)] = true
							}

							llock.Unlock()
						}

						// regular search requires exact match from binary search
						if R < numTerms && strs[R] == term {

							offset := indx[R].PostOffset
							size := indx[R+1].PostOffset - offset

							linkLoop(offset, size)
						}
					}
				}

				postingsLoop(dpath, ky, field)

				out <- ky
			}
		}

		var wg sync.WaitGroup

		// launch multiple link merger goroutines
		for range numServe {
			wg.Add(1)
			go linkMerge(&wg, prom, field, inp, out)
		}

		// launch separate anonymous goroutine to wait until all mergers are done
		go func() {
			wg.Wait()
			close(out)
		}()

		return out
	}

	// read text PMIDs from stdin
	uidq := CreateUIDReader(os.Stdin)

	grpq := createLinkGrouper(postingsBase, fld, uidq)

	lnkq := createLinkMergers(postingsBase, fld, grpq)

	// drain channel
	for range lnkq {
	}

	// sort id keys in alphabetical order
	keys := slices.Sorted(maps.Keys(combinedLinks))

	// use buffers to speed up PMID printing
	var buffer strings.Builder

	wrtr := bufio.NewWriter(os.Stdout)

	for _, uid := range keys {
		pmid := strconv.Itoa(uid)
		buffer.WriteString(pmid)
		buffer.WriteString("\n")
	}

	txt := buffer.String()
	if txt != "" {
		// print buffer
		wrtr.WriteString(txt[:])
	}

	wrtr.Flush()

	runtime.Gosched()
}

// ProcessMatch evaluates query, returns lines with term match count and UID
func ProcessMatch(db, phrase string, deStop bool) {

	if phrase == "" {
		return
	}

	if db == "" {
		db = "pubmed"
	}
	db = strings.ToLower(db)

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	phrase = prepareQuery(phrase)

	phrase = processStopWords(phrase, deStop)

	clauses := partitionQuery(phrase)

	clauses = setFieldQualifiers(db, clauses)

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

	nextToken := func() string {

		if len(clauses) < 1 {
			return ""
		}

		// remove next token from slice
		tkn := clauses[0]
		clauses = clauses[1:]

		return tkn
	}

	// histogram of counts for UIDs matching one or more terms
	counts := make(map[int32]int)

	for {
		tkn := nextToken()
		if tkn == "" {
			break
		}
		if tkn == "&" || tkn == "|" || tkn == "^" {
			continue
		}

		var terms []string
		field := ""

		pos := strings.Index(tkn, "[")
		if pos >= 0 {
			field = tkn[pos:]
			tkn = tkn[:pos]
			field = strings.TrimSpace(field)
			tkn = strings.TrimSpace(tkn)
		}
		if field == "[PAIR]" {
			terms = wordPairs(tkn)
		} else {
			terms = singleWords(tkn)
		}

		if len(terms) < 1 {
			break
		}

		for _, item := range terms {
			arry := ProcessQuery(db, item+" "+field, false, false, false, deStop)
			for _, uid := range arry {
				val := counts[uid]
				val++
				counts[uid] = val
			}
		}
	}

	// data reorganized by number of matches
	matches := make(map[int][]int32)

	for id, ct := range counts {
		mtch := matches[ct]
		if mtch == nil {
			mtch = make([]int32, 0, 1)
		}
		mtch = append(mtch, id)
		matches[ct] = mtch
	}

	// sort highest count to lowest count
	keys := slices.SortedFunc(maps.Keys(matches),
		func(i, j int) int {
			// sends arguments in reverse order
			return cmp.Compare(j, i)
		})

	var buffer strings.Builder

	wrtr := bufio.NewWriter(os.Stdout)

	has := 0

	for _, key := range keys {

		if key == 1 && has > 0 {
			break
		}
		cts := matches[key]
		kys := strconv.Itoa(key)

		// within the same count, sort lowest UID to highest UID
		slices.Sort(cts)

		for _, uid := range cts {
			uids := strconv.Itoa(int(uid))
			buffer.WriteString(kys + "\t" + uids + "\n")
		}

		has++
		// do not break if has > 2, now allows any count > 1,
		// use just-top-hits {n} to filter for top n counts
	}

	txt := buffer.String()
	if txt != "" {
		// print buffer
		wrtr.WriteString(txt[:])
	}

	wrtr.Flush()

	runtime.Gosched()
}

func streamTermsOrTotals(db, fld string, justTerms bool) <-chan string {

	if db == "" {
		db = "pubmed"
	}
	if fld == "" {
		DisplayError("Field missing for visiting terms")
		os.Exit(1)
	}

	// obtain paths from environment variable(s)
	master, _ := GetLocalArchivePaths(db)

	if master == "" {

		DisplayError("Unable to get local archive paths")
		os.Exit(1)
	}

	postingsBase := master + "Postings"

	// check to make sure local postings are mounted
	_, err := os.Stat(postingsBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local postings files are not mounted")
		os.Exit(1)
	}

	findTopDirs := func(db, fld string) <-chan string {

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create term path channel")
			os.Exit(1)
		}

		findTopDirs := func(base, fld string, out chan<- string) {

			defer close(out)

			fieldBase := filepath.Join(base, fld)

			topDirs, err := os.ReadDir(fieldBase)
			if err != nil {
				return
			}

			for _, item := range topDirs {
				name := item.Name()
				if name != "" && item.IsDir() {
					sub := filepath.Join(fieldBase, name)
					out <- sub
				}
			}
		}

		go findTopDirs(postingsBase, fld, out)

		return out
	}

	findSubDirs := func(inp <-chan string) <-chan []string {

		if inp == nil {
			return nil
		}

		out := make(chan []string, chanDepth)
		if out == nil {
			DisplayError("Unable to create term exploration channel")
			os.Exit(1)
		}

		getSubDirs := func(inp <-chan string, out chan<- []string) {

			defer close(out)

			visitOneTop := func(top string) {

				var paths []string

				// recursive definition
				var visitSubFolders func(path string)

				// visitSubFolders recurses to leaf directories
				visitSubFolders = func(path string) {

					contents, err := os.ReadDir(path)
					if err != nil {
						return
					}

					for _, item := range contents {
						name := item.Name()
						if name == "" {
							continue
						}
						if item.IsDir() {
							continue
						}
						if strings.HasSuffix(name, ".trm") {
							name = strings.TrimSuffix(name, ".trm")
							tl := filepath.Join(path, name)
							paths = append(paths, tl)
						}
					}
					for _, item := range contents {
						name := item.Name()
						if name == "" {
							continue
						}
						if item.IsDir() {
							sub := filepath.Join(path, name)
							visitSubFolders(sub)
						}
					}
				}

				visitSubFolders(top)

				out <- paths
			}

			for top := range inp {
				visitOneTop(top)
			}
		}

		go getSubDirs(inp, out)

		return out
	}

	readBinaryFileToMaster := func(fileName string) []Master {

		f, err := os.Open(fileName)
		if err != nil {
			DisplayError("Unable to open input file '%s'", fileName)
			return nil
		}

		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return nil
		}

		size := fi.Size()

		data := make([]Master, size/8)
		if data == nil || len(data) < 1 {
			return nil
		}

		err = binary.Read(f, binary.LittleEndian, &data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return nil
		}

		return data
	}

	fetchTermSet := func(inp <-chan []string) <-chan []string {

		if inp == nil {
			return nil
		}

		out := make(chan []string, chanDepth)
		if out == nil {
			DisplayError("Unable to create term set channel")
			os.Exit(1)
		}

		fetchTerms := func(inp <-chan string) <-chan string {

			out := make(chan string, chanDepth)
			if out == nil {
				DisplayError("Unable to create term fetch channel")
				os.Exit(1)
			}

			termFetcher := func(wg *sync.WaitGroup, inp <-chan string, out chan<- string) {

				// report when more records to process
				defer wg.Done()

				getTermCounts := func(pth string) {

					trms := ReadTextFileToString(pth + ".trm")
					if trms == "" || len(trms) < 1 {
						return
					}

					if justTerms {
						out <- trms
						return
					}

					indx := readBinaryFileToMaster(pth + ".mst")
					if indx == nil || len(indx) < 1 {
						return
					}

					// master index is padded with phantom term and postings position
					numTerms := len(indx) - 1

					strs := make([]string, numTerms)
					if strs == nil || len(strs) < 1 {
						return
					}

					retlength := int32(len("\n"))

					// populate array of strings from term list
					for i, j := 0, 1; i < numTerms; i++ {
						from := indx[i].TermOffset
						to := indx[j].TermOffset - retlength
						j++
						txt := string(trms[from:to])
						strs[i] = txt
					}

					var arry []string

					for R, str := range strs {
						offset := indx[R].PostOffset
						size := indx[R+1].PostOffset - offset
						line := fmt.Sprintf("%d\t%s\n", size/4, str)
						arry = append(arry, line)
					}

					lst := strings.Join(arry, "")

					out <- lst
				}

				for pth := range inp {
					getTermCounts(pth)
				}
			}

			var wg sync.WaitGroup

			// launch multiple fetcher goroutines
			for range numServe {
				wg.Add(1)
				go termFetcher(&wg, inp, out)
			}

			// launch separate anonymous goroutine to wait until all fetchers are done
			go func() {
				wg.Wait()
				close(out)
			}()

			return out
		}

		fetchByGroup := func(inp <-chan []string, out chan<- []string) {

			defer close(out)

			for paths := range inp {
				slc := SliceToChan(paths)
				tms := fetchTerms(slc)
				var tlst []string
				for trm := range tms {
					tlst = append(tlst, trm)
				}
				out <- tlst
			}
		}

		go fetchByGroup(inp, out)

		return out
	}

	sortTermSet := func(inp <-chan []string) <-chan string {

		if inp == nil {
			return nil
		}

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create term sorter channel")
			os.Exit(1)
		}

		sortGroup := func(inp <-chan []string, out chan<- string) {

			defer close(out)

			sortOneSet := func(tlst []string) {

				lists := make(map[string]string)
				if lists == nil {
					DisplayError("Unable to create term list map")
					os.Exit(1)
				}

				for _, terms := range tlst {

					lft, _, foundl := strings.Cut(terms, "\n")
					if foundl {
						lft = strings.TrimSpace(lft)
						_, rgt, foundr := strings.Cut(lft, "\t")
						if foundr {
							lists[rgt] = terms
						} else {
							lists[lft] = terms
						}
					}
				}

				// sort terms in alphabetical order
				keys := slices.Sorted(maps.Keys(lists))

				for _, key := range keys {
					str := lists[key]
					out <- str
				}

				runtime.Gosched()
			}

			for tlst := range inp {
				sortOneSet(tlst)
			}
		}

		go sortGroup(inp, out)

		return out
	}

	trms := findTopDirs(db, fld)
	subs := findSubDirs(trms)
	tlst := fetchTermSet(subs)
	srts := sortTermSet(tlst)

	if trms == nil || subs == nil || tlst == nil || srts == nil {
		DisplayError("Unable to create term generator")
		os.Exit(1)
	}

	return srts
}

// StreamTerms prints all postings terms for a given field
func StreamTerms(db, fld string) <-chan string {

	return streamTermsOrTotals(db, fld, true)
}

// StreamTotals prints all postings terms and counts for a given field
func StreamTotals(db, fld string) <-chan string {

	return streamTermsOrTotals(db, fld, false)
}

// initialize empty journal and MeSH maps before non-init functions are called
func init() {

	meshName.table = make(map[string]string)
	meshTree.table = make(map[string]string)
}
