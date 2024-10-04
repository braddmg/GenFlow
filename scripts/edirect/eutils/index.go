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
// File Name:  index.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// PUBMED INDEXED AND INVERTED FILE FORMATS

// Local archive indexing reads PubmedArticle XML records and produces IdxDocument records.
// Title and Title/Abstract fields include term positions as XML attributes:

/*

  ...
  <IdxDocument>
    <IdxUid>2539356</IdxUid>
    <IdxSearchFields>
      <UID>02539356</UID>
      <SIZE>13727</SIZE>
      <YEAR>1989</YEAR>
      <DATE>1989 04</DATE>
      <RDAT>2019 05 08</RDAT>
      <JOUR>J Bacteriol</JOUR>
      <JOUR>2985120R</JOUR>
      <JOUR>0021-9193</JOUR>
      <JOUR>Journal of Bacteriology</JOUR>
      <JOUR>J Bacteriol</JOUR>
      <JOUR>0021-9193</JOUR>
      <VOL>171</VOL>
      <ISS>4</ISS>
      <PAGE>1904</PAGE>
      <LANG>eng</LANG>
      <ANUM>2</ANUM>
      <FAUT>Kans JA</FAUT>
      <LAUT>Casadaban MJ</LAUT>
      <AUTH>Kans JA</AUTH>
      <AUTH>Casadaban MJ</AUTH>
      <TITL pos="7">immunity</TITL>
      <TITL pos="1">nucleotide</TITL>
      <TITL pos="3">required</TITL>
      <TITL pos="2">sequences</TITL>
      <TITL pos="5">tn3</TITL>
      <TITL pos="6">transposition</TITL>
      <TIAB pos="145">38</TIAB>
      <TIAB pos="126">acting</TIAB>
      <TIAB pos="188">additional</TIAB>
      <TIAB pos="146">base</TIAB>
      <TIAB pos="125">cis</TIAB>
      <TIAB pos="172,178,187">conferred</TIAB>
      ...
      <PAIR>nucleotide sequences</PAIR>
      <PAIR>sequences required</PAIR>
      <PAIR>tn3 transposition</PAIR>
      <PAIR>transposition immunity</PAIR>
      <PTYP>Journal Article</PTYP>
      <PTYP>Research Support, U.S. Gov&#39;t, P.H.S.</PTYP>
      <PROP>Published In Print</PROP>
      <PROP>Has Abstract</PROP>
      <DOI>9891 4191 4091 4 171 bj 8211 01</DOI>
      <PMCID>209839</PMCID>
      <CODE>d001483</CODE>
      <CODE>d002874</CODE>
      ...
      <MESH>Plasmids</MESH>
      <MESH>Recombination, Genetic</MESH>
    </IdxSearchFields>
  </IdxDocument>
  ...

*/

// Inversion reads a set of indexed documents and generated InvDocument records:

/*

  ...
  <InvDocument>
    <InvKey>tn3</InvKey>
    <InvIDs>
      <TIAB pos="5,102,117,129,157,194">2539356</TIAB>
      <TITL pos="5">2539356</TITL>
    </InvIDs>
  </InvDocument>
  <InvDocument>
    <InvKey>tn3 transposition</InvKey>
    <InvIDs>
      <PAIR>2539356</PAIR>
    </InvIDs>
  </InvDocument>
  <InvDocument>
    <InvKey>transposition</InvKey>
    <InvIDs>
      <TIAB pos="6,122">2539356</TIAB>
      <TITL pos="6">2539356</TITL>
    </InvIDs>
  </InvDocument>
  <InvDocument>
    <InvKey>transposition immunity</InvKey>
    <InvIDs>
      <PAIR>2539356</PAIR>
    </InvIDs>
  </InvDocument>
  ...

*/

// Separate inversion runs are merged and used to produce term lists and postings file.
// These can then be searched by passing commands to EDirect's "phrase-search" script.

// ENTREZ2INDEX COMMAND GENERATOR

// MakeE2Commands generates extraction commands to create input for Entrez2Index
func MakeE2Commands(tform, idxargs string) []string {

	var acc []string

	// idxargs file contains one command or argument per line
	if idxargs == "" {
		return acc
	}

	inFile, err := os.Open(idxargs)
	if err != nil {
		DisplayError("Unable to open index argument array file: %s\n", err.Error())
		return acc
	}
	defer inFile.Close()

	scanr := bufio.NewScanner(inFile)
	if scanr == nil {
		DisplayError("Unable to create NewScanner")
		return acc
	}

	for scanr.Scan() {

		line := scanr.Text()

		// do NOT skip empty line or trim spaces, to allow "" or " " arguments

		acc = append(acc, line)
	}

	return acc
}

// UPDATE CACHED INDEXED AND INVERTED-INDEX FILES FROM LOCAL ARCHIVE FOLDERS

// examineFolder collects two-digit subdirectories, xml files, e2x files, and inv files
func examineFolder(base, path string) ([]string, []string, []string, []string) {

	dir := filepath.Join(base, path)

	contents, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil, nil
	}

	isTwoDigits := func(str string) bool {

		if len(str) != 2 {
			return false
		}

		ch := str[0]
		if ch < '0' || ch > '9' {
			return false
		}

		ch = str[1]
		if ch < '0' || ch > '9' {
			return false
		}

		return true
	}

	var dirs []string
	var xmls []string
	var e2xs []string
	var invs []string

	for _, item := range contents {
		name := item.Name()
		if name == "" {
			continue
		}
		if item.IsDir() {
			if isTwoDigits(name) {
				dirs = append(dirs, name)
			}
		} else if strings.HasSuffix(name, ".xml.gz") {
			xmls = append(xmls, name)
		} else if strings.HasSuffix(name, ".e2x.gz") {
			e2xs = append(e2xs, name)
		} else if strings.HasSuffix(name, ".inv.gz") {
			invs = append(invs, name)
		}
	}

	return dirs, xmls, e2xs, invs
}

// gzFileToString reads selected gzipped file, uncompressing and saving contents as string
func gzFileToString(fpath string) string {

	file, err := os.Open(fpath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var rdr io.Reader

	gz, err := gzip.NewReader(file)
	if err != nil {
		return ""
	}
	defer gz.Close()
	rdr = gz

	byt, err := io.ReadAll(rdr)
	if err != nil {
		return ""
	}

	str := string(byt)
	if str == "" {
		return ""
	}

	if !strings.HasSuffix(str, "\n") {
		str += "\n"
	}

	return str
}

func stringToGzFile(base, path, file, str string) {

	if str == "" {
		return
	}

	dpath := filepath.Join(base, path)
	if dpath == "" {
		return
	}
	err := os.MkdirAll(dpath, os.ModePerm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return
	}
	fpath := filepath.Join(dpath, file)
	if fpath == "" {
		return
	}

	// overwrites and truncates existing file
	fl, err := os.Create(fpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return
	}

	// for small files, use regular gzip
	zpr, err := gzip.NewWriterLevel(fl, gzip.BestSpeed)
	if err != nil {
		DisplayError("Unable to create compressor")
		os.Exit(1)
	}

	wrtr := bufio.NewWriter(zpr)

	// write contents
	wrtr.WriteString(str)
	if !strings.HasSuffix(str, "\n") {
		wrtr.WriteString("\n")
	}

	err = wrtr.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return
	}

	err = zpr.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return
	}

	// fl.Sync()

	err = fl.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return
	}
}

// e2IndexConsumer callbacks have access to application-specific data as closures
type e2IndexConsumer func(inp <-chan XMLRecord) <-chan XMLRecord

// IncrementalIndex creates or updates missing cached .e2x.gz indexed files,
// e.g., /Index/02/53/025393.e2x.gz for /Archive/02/53/93/*.xml.gz
func IncrementalIndex(db, pfx, ptrn string, dotMax int, csmr e2IndexConsumer) <-chan string {

	if csmr == nil {
		return nil
	}

	if db == "" {
		db = "pubmed"
	}

	// obtain paths from environment variable(s)
	master, working := GetLocalArchivePaths(db)

	if master == "" || working == "" {

		DisplayError("Unable to get local archive paths")
		os.Exit(1)
	}

	archiveBase := master + "Archive"
	indexBase := working + "Index"

	// check to make sure local archive is mounted
	_, err := os.Stat(archiveBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local archive and search index is not mounted")
		os.Exit(1)
	}

	// check to make sure local index is mounted
	_, err = os.Stat(indexBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local incremental index is not mounted")
		os.Exit(1)
	}

	// visitArchiveFolders sends an Archive leaf folder path plus the file base names
	// contained in it down a channel, e.g., [ "02/53/93", "2539300", "2539301", ..., ] for
	// Archive/02/53/93/*.xml.gz
	visitArchiveFolders := func(archiveBase string) <-chan []string {

		out := make(chan []string, chanDepth)
		if out == nil {
			DisplayError("Unable to create archive explorer channel")
			os.Exit(1)
		}

		// recursive definition
		var visitSubFolders func(base, path string, out chan<- []string)

		// visitSubFolders recurses to leaf directories
		visitSubFolders = func(base, path string, out chan<- []string) {

			dirs, xmls, _, _ := examineFolder(base, path)

			// recursively explore subdirectories
			if dirs != nil {
				for _, dr := range dirs {
					sub := filepath.Join(path, dr)
					visitSubFolders(base, sub, out)
				}
				return
			}

			// looking for leaf Archive directory with at least one *.xml.gz file
			if xmls == nil {
				return
			}

			// remove ".xml.gz" suffixes, leaving unpadded PMID
			for i, file := range xmls {
				pos := strings.Index(file, ".")
				if pos >= 0 {
					file = file[:pos]
				}
				xmls[i] = file
			}

			if len(xmls) > 1 {
				// sort fields in alphabetical or numeric order
				slices.SortFunc(xmls, CompareAlphaOrNumericKeys)
			}

			var res []string
			res = append(res, path)
			res = append(res, xmls...)

			out <- res
		}

		visitArchiveSubset := func(base string, out chan<- []string) {

			defer close(out)

			dirs, _, _, _ := examineFolder(base, "")

			// iterate through top directories
			for _, top := range dirs {
				// skip Sentinels folder
				if IsAllDigits(top) && len(top) == 2 {
					visitSubFolders(base, top, out)
				}

				// force garbage collection
				runtime.GC()

				runtime.Gosched()
			}
		}

		// launch single archive visitor goroutine
		go visitArchiveSubset(archiveBase, out)

		return out
	}

	// filterIndexFolders checks for presence of an Index file for an archive folder,
	// only passing those files that need to be (re)indexed
	filterIndexFolders := func(indexBase string, inp <-chan []string) <-chan XMLRecord {

		out := make(chan XMLRecord, chanDepth)
		if out == nil {
			DisplayError("Unable to create index filter channel")
			os.Exit(1)
		}

		filterIndexSubset := func(indBase string, inp <-chan []string, out chan<- XMLRecord) {

			defer close(out)

			idx := 0

			for data := range inp {

				// path is first element in slice
				path := data[0]
				// "02/53/93/"

				// followed by xml file base names (PMIDs)
				pmids := data[1:]

				indPath := path[:6]
				// "02/53/"

				indFile := strings.Replace(path, "/", "", -1)
				// "025393"

				target := filepath.Join(indBase, indPath, indFile+".e2x.gz")

				_, err := os.Stat(target)
				if err == nil {
					// skip if first-level incremental Entrez index file exists for current set of 100 archive records
					continue
				}

				for _, pmid := range pmids {
					// increment index so unshuffler can restore order of results
					idx++

					// send PMID (unindexed file base name) down channel
					out <- XMLRecord{Index: idx, Ident: indFile, Text: pmid}
				}
			}
		}

		// launch single index filter goroutine
		go filterIndexSubset(indexBase, inp, out)

		return out
	}

	cleanIndexFiles := func(inp <-chan XMLRecord) <-chan XMLRecord {

		out := make(chan XMLRecord, chanDepth)
		if out == nil {
			DisplayError("Unable to create index cleaner channel")
			os.Exit(1)
		}

		indexCleaner := func(wg *sync.WaitGroup, indBase string, inp <-chan XMLRecord, out chan<- XMLRecord) {

			defer wg.Done()

			re := regexp.MustCompile(">[ \n\r\t]*<")

			for curr := range inp {

				str := curr.Text

				if str == "" {
					continue
				}

				// clean up white space between stop tag and next start tag, replacing with a single newline
				str = re.ReplaceAllString(str, ">\n<")

				if !strings.HasSuffix(str, "\n") {
					str += "\n"
				}

				out <- XMLRecord{Index: curr.Index, Ident: curr.Ident, Text: str}
			}
		}

		var wg sync.WaitGroup

		// launch multiple index cleaner goroutines
		for range numProcs {
			wg.Add(1)
			go indexCleaner(&wg, indexBase, inp, out)
		}

		// launch separate anonymous goroutine to wait until all index cleaners are done
		go func() {
			wg.Wait()
			close(out)
		}()

		return out
	}

	combineIndexFiles := func(indexBase string, dotMax int, inp <-chan XMLRecord) <-chan string {

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create index combiner channel")
			os.Exit(1)
		}

		// mutex to protect access to rollingCount and rollingColumn variables
		var vlock sync.Mutex

		rollingCount := 0
		rollingColumn := 0

		countSuccess := func() {

			vlock.Lock()

			rollingCount++
			if rollingCount >= dotMax {
				rollingCount = 0
				// print dot (progress monitor)
				fmt.Fprintf(os.Stderr, ".")
				rollingColumn++
				if rollingColumn > 49 {
					// print newline after 50 dots
					fmt.Fprintf(os.Stderr, "\n")
					rollingColumn = 0
				}
			}

			vlock.Unlock()
		}

		indexCombiner := func(indBase string, inp <-chan XMLRecord, out chan<- string) {

			defer close(out)

			currentIdent := ""

			var buffer strings.Builder

			verbose := false
			// set verbose flag from environment variable
			env := os.Getenv("EDIRECT_LOCAL_VERBOSE")
			if env == "Y" || env == "y" || env == "true" {
				verbose = true
			}

			for curr := range inp {

				str := curr.Text

				if str == "" {
					continue
				}

				ident := curr.Ident

				if ident != currentIdent && currentIdent != "" {
					txt := buffer.String()
					indPath, _ := IndexTrie(currentIdent + "00")
					stringToGzFile(indBase, indPath, currentIdent+".e2x.gz", txt)
					buffer.Reset()

					if verbose {
						fmt.Fprintf(os.Stderr, "IDX %s/%s%s.e2x.gz\n", indBase, indPath, currentIdent)
					} else {
						// progress monitor
						countSuccess()
					}

					out <- currentIdent
				}

				currentIdent = ident

				buffer.WriteString(str)

			}

			if currentIdent != "" {
				txt := buffer.String()
				indPath, _ := IndexTrie(currentIdent + "00")
				stringToGzFile(indBase, indPath, currentIdent+".e2x.gz", txt)
				buffer.Reset()

				if verbose {
					fmt.Fprintf(os.Stderr, "IDX %s/%s%s.e2x.gz\n", indBase, indPath, currentIdent)
				}
			}

			if rollingColumn > 0 {
				vlock.Lock()
				fmt.Fprintf(os.Stderr, "\n")
				vlock.Unlock()
			}
		}

		// launch single index combiner goroutine
		go indexCombiner(indexBase, inp, out)

		return out
	}

	if dotMax == 0 {
		// show a dot every 200 .e2x files, generated from up to 20000 .xml files
		dotMax = 200
		if db == "pmc" {
			dotMax = 50
		} else if db == "taxonomy" {
			dotMax = 500
		}
	}

	vrfq := visitArchiveFolders(archiveBase)
	vifq := filterIndexFolders(indexBase, vrfq)
	strq := CreateFetchers(archiveBase, db, pfx, ".xml", ptrn, true, vifq)
	// callback passes cmds and transform values as closures to xtract createConsumers
	tblq := csmr(strq)
	// clean up XML (no measured benefit to adding next record size prefix)
	sifq := cleanIndexFiles(tblq)
	// restore original order, so indexed results are grouped by archive folder
	unsq := CreateXMLUnshuffler(sifq)
	cifq := combineIndexFiles(indexBase, dotMax, unsq)

	if vrfq == nil || vifq == nil || strq == nil || tblq == nil || sifq == nil || unsq == nil || cifq == nil {
		return nil
	}

	return cifq
}

// InvertIndexedFile reads IdxDocument XML strings and writes a combined InvDocument XML record
func InvertIndexedFile(inp <-chan string) <-chan string {

	if inp == nil {
		return nil
	}

	indexDispenser := func(inp <-chan string) <-chan []string {

		if inp == nil {
			return nil
		}

		out := make(chan []string, chanDepth)
		if out == nil {
			DisplayError("Unable to create dispenser channel")
			os.Exit(1)
		}

		type Inverter struct {
			ilock sync.Mutex
			// map for inverted index
			inverted map[string][]string
		}

		inverters := make(map[rune]*Inverter)

		prefixes := "01234567890abcdefghijklmnopqrstuvwxyz"

		for _, ch := range prefixes {
			inverters[ch] = &Inverter{inverted: make(map[string][]string)}
		}

		// add single posting
		addPost := func(fld, term, pos, uid string) {

			ch := rune(term[0])
			inv := inverters[ch]

			// protect map with mutex
			inv.ilock.Lock()

			data, ok := inv.inverted[term]
			if !ok {
				data = make([]string, 0, 4)
				// first entry on new slice is term
				data = append(data, term)
			}
			data = append(data, fld)
			data = append(data, uid)
			data = append(data, pos)
			// always need to update inverted, since data may be reallocated
			inv.inverted[term] = data

			inv.ilock.Unlock()
		}

		// xmlDispenser prepares UID, term, and position strings for inversion
		xmlDispenser := func(wg *sync.WaitGroup, inp <-chan string, out chan<- []string) {

			defer wg.Done()

			currUID := ""

			doDispense := func(tag, attr, content string) {

				if tag == "IdxUid" {
					currUID = content
				} else {

					content = html.UnescapeString(content)

					// expand Greek letters, anglicize characters in other alphabets
					if IsNotASCII(content) {

						content = TransformAccents(content, true, true)

						if HasAdjacentSpacesOrNewline(content) {
							content = CompressRunsOfSpaces(content)
						}

						content = UnicodeToASCII(content)

						if HasFlankingSpace(content) {
							content = strings.TrimSpace(content)
						}
					}

					content = strings.ToLower(content)

					// remove punctuation from term
					content = strings.Map(func(c rune) rune {
						if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != ' ' && c != '-' && c != '_' {
							return -1
						}
						return c
					}, content)

					content = strings.Replace(content, "_", " ", -1)
					content = strings.Replace(content, "-", " ", -1)

					if HasAdjacentSpacesOrNewline(content) {
						content = CompressRunsOfSpaces(content)
					}

					if HasFlankingSpace(content) {
						content = strings.TrimSpace(content)
					}

					if content != "" && currUID != "" {
						addPost(tag, content, attr, currUID)
					}
				}
			}

			// read partitioned XML from producer channel
			for str := range inp {
				StreamValues(str[:], "IdxDocument", doDispense)
			}
		}

		var wg sync.WaitGroup

		// launch multiple dispenser goroutines
		for range numProcs {
			wg.Add(1)
			go xmlDispenser(&wg, inp, out)
		}

		// launch separate anonymous goroutine to wait until all dispensers are done
		go func() {
			wg.Wait()

			// send results to inverters
			for _, ch := range prefixes {
				inv := inverters[ch]
				for _, data := range inv.inverted {
					out <- data

					runtime.Gosched()
				}
			}

			close(out)
		}()

		return out
	}

	indexInverter := func(inp <-chan []string) <-chan XMLRecord {

		if inp == nil {
			return nil
		}

		out := make(chan XMLRecord, chanDepth)
		if out == nil {
			DisplayError("Unable to create inverter channel")
			os.Exit(1)
		}

		// xmlInverter sorts and prints one posting list
		xmlInverter := func(wg *sync.WaitGroup, inp <-chan []string, out chan<- XMLRecord) {

			defer wg.Done()

			var buffer strings.Builder

			printPosting := func(key string, data []string) string {

				fields := make(map[string]map[string]string)

				for len(data) > 1 {
					fld := data[0]
					uid := data[1]
					att := data[2]
					positions, ok := fields[fld]
					if !ok {
						positions = make(map[string]string)
						fields[fld] = positions
					}
					// store position attribute string by uid
					positions[uid] = att
					// skip to next position
					data = data[3:]
				}

				buffer.Reset()

				buffer.WriteString("<InvDocument>\n")
				buffer.WriteString("<InvKey>")
				buffer.WriteString(key)
				buffer.WriteString("</InvKey>\n")
				buffer.WriteString("<InvIDs>\n")

				// sort fields in alphabetical order
				keys := slices.Sorted(maps.Keys(fields))

				for _, fld := range keys {

					positions := fields[fld]

					arry := slices.SortedFunc(maps.Keys(positions), CompareAlphaOrNumericKeys)

					// print list of UIDs, skipping duplicates
					prev := ""
					for _, uid := range arry {
						if uid == prev {
							continue
						}

						buffer.WriteString("<")
						buffer.WriteString(fld)
						atr := positions[uid]
						if atr != "" {
							buffer.WriteString(" ")
							buffer.WriteString(atr)
						}
						buffer.WriteString(">")
						buffer.WriteString(uid)
						buffer.WriteString("</")
						buffer.WriteString(fld)
						buffer.WriteString(">\n")

						prev = uid
					}
				}

				buffer.WriteString("</InvIDs>\n")
				buffer.WriteString("</InvDocument>\n")

				str := buffer.String()

				return str
			}

			for inv := range inp {

				key := inv[0]
				data := inv[1:]

				str := printPosting(key, data)

				out <- XMLRecord{Ident: key, Text: str}

				runtime.Gosched()
			}
		}

		var wg sync.WaitGroup

		// launch multiple inverter goroutines
		for range numProcs {
			wg.Add(1)
			go xmlInverter(&wg, inp, out)
		}

		// launch separate anonymous goroutine to wait until all inverters are done
		go func() {
			wg.Wait()
			close(out)
		}()

		return out
	}

	indexResolver := func(inp <-chan XMLRecord) <-chan string {

		if inp == nil {
			return nil
		}

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create resolver channel")
			os.Exit(1)
		}

		// xmlResolver prints inverted postings alphabetized by identifier prefix
		xmlResolver := func(inp <-chan XMLRecord, out chan<- string) {

			// close channel when all records have been processed
			defer close(out)

			// map for inverted index
			inverted := make(map[string]string)

			// drain channel, populate map for alphabetizing
			for curr := range inp {

				inverted[curr.Ident] = curr.Text
			}

			ordered := slices.Sorted(maps.Keys(inverted))

			// iterate through alphabetized results
			for _, curr := range ordered {

				txt := inverted[curr]

				// send result to output
				out <- txt

				runtime.Gosched()
			}
		}

		// launch single resolver goroutine
		go xmlResolver(inp, out)

		return out
	}

	idsq := indexDispenser(inp)
	invq := indexInverter(idsq)
	idrq := indexResolver(invq)

	if idsq == nil || invq == nil || idrq == nil {
		return nil
	}

	return idrq
}

// IncrementalInvert creates or updates missing cached .inv.gz inverted index files
func IncrementalInvert(db string, dotMax int) <-chan string {

	if db == "" {
		db = "pubmed"
	}

	// obtain paths from environment variable(s)
	_, working := GetLocalArchivePaths(db)

	if working == "" {

		DisplayError("Unable to get local index paths")
		os.Exit(1)
	}

	indexBase := working + "Index"
	invertBase := working + "Invert"

	// check to make sure local index directory is mounted
	_, err := os.Stat(indexBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local incremental index is not mounted")
		os.Exit(1)
	}

	// check to make sure local invert directory is mounted
	_, err = os.Stat(invertBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local invert directory is not mounted")
		os.Exit(1)
	}

	indexFetchers := func(inp <-chan string) <-chan string {

		if inp == nil {
			return nil
		}

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create index fetcher channel")
			os.Exit(1)
		}

		// e2xFetcher reads indexed XML from file
		e2xFetcher := func(wg *sync.WaitGroup, inp <-chan string, out chan<- string) {

			defer wg.Done()

			for file := range inp {

				txt := gzFileToString(file)

				out <- txt
			}
		}

		var wg sync.WaitGroup

		// launch multiple fetcher goroutines
		for range numProcs {
			wg.Add(1)
			go e2xFetcher(&wg, inp, out)
		}

		// launch separate anonymous goroutine to wait until all fetchers are done
		go func() {
			wg.Wait()
			close(out)
		}()

		return out
	}

	invertIndexFiles := func(invBase string, inp <-chan []string) <-chan string {

		out := make(chan string, chanDepth)
		if out == nil {
			DisplayError("Unable to create index inverter channel")
			os.Exit(1)
		}

		invertIndexSubset := func(wg *sync.WaitGroup, invBase string, inp <-chan []string, out chan<- string) {

			defer wg.Done()

			for data := range inp {

				// path is first element in slice
				path := data[0]
				// "02/53"

				// followed by e2x file paths
				filenames := data[1:]

				invPath := path[:3]
				// "02/"

				invFile := strings.Replace(path, "/", "", -1)
				// "0253"

				s2cq := SliceToChan(filenames)
				idfq := indexFetchers(s2cq)
				// indexDispenser | indexInverter | indexResolver
				iifq := InvertIndexedFile(idfq)

				var buffer strings.Builder

				for str := range iifq {
					buffer.WriteString(str)
				}

				txt := buffer.String()
				if txt == "" {
					fmt.Fprintf(os.Stderr, "Empty %s\n", invPath+invFile)
					return
				}

				// save to target file
				stringToGzFile(invertBase, invPath, invFile+".inv.gz", txt)

				out <- invFile + ".inv.gz"
			}
		}

		var wg sync.WaitGroup

		// launch inverter goroutines (use 3 to numProcs: with numServe, PMC will eventually overflow memory)
		for range numProcs {
			wg.Add(1)
			go invertIndexSubset(&wg, invBase, inp, out)
		}

		// launch separate anonymous goroutine to wait until all inverters are done
		go func() {
			wg.Wait()
			close(out)
		}()

		return out
	}

	// visitIndexFolders sends an Index leaf folder name, e.g., "02/53",
	// plus full paths to all component index files, down a channel, but
	// only if there is no existing Invert file, e.g., Invert/02/0253.inv.gz
	visitIndexFolders := func(indexBase, invertBase string) <-chan []string {

		out := make(chan []string, chanDepth)
		if out == nil {
			DisplayError("Unable to create index explorer channel")
			os.Exit(1)
		}

		// mutex to protect access to rollingCount and rollingColumn variables
		var vlock sync.Mutex

		if dotMax == 0 {
			// show a dot every 10 .inv files that were deleted for regeneration,
			// representing from up to 1000 .e2x files, some also just regenerated
			dotMax = 10
		}

		rollingCount := 0
		rollingColumn := 0

		countSuccess := func() {

			vlock.Lock()

			rollingCount++
			if rollingCount >= dotMax {
				rollingCount = 0
				// print dot (progress monitor)
				fmt.Fprintf(os.Stderr, ".")
				rollingColumn++
				if rollingColumn > 49 {
					// print newline after 50 dots
					fmt.Fprintf(os.Stderr, "\n")
					rollingColumn = 0
				}
			}

			vlock.Unlock()
		}

		verbose := false
		// set verbose flag from environment variable
		env := os.Getenv("EDIRECT_LOCAL_VERBOSE")
		if env == "Y" || env == "y" || env == "true" {
			verbose = true
		}

		// recursive definition
		var visitSubFolders func(idxBase, invBase, path string, out chan<- []string)

		// visitSubFolders recurses to leaf directories
		visitSubFolders = func(idxBase, invBase, path string, out chan<- []string) {

			dirs, _, e2xs, _ := examineFolder(idxBase, path)

			// recursively explore subdirectories
			if dirs != nil {
				for _, dr := range dirs {
					sub := filepath.Join(path, dr)
					visitSubFolders(idxBase, invBase, sub, out)
				}
				return
			}

			// looking for leaf Index directory with at least one *.e2x.gz file
			if e2xs == nil {
				return
			}

			// "02/53"

			invPath := path[:2]
			// "02"

			invFile := strings.Replace(path, "/", "", -1)
			// "0253"

			target := filepath.Join(invBase, invPath, invFile+".inv.gz")

			_, err := os.Stat(target)
			if err == nil {
				// if inverted index file exists for the indexed folder, no need to recreate
				return
			}

			var res []string

			// first string is indexed folder path, to become inverted file name
			res = append(res, path)

			for _, file := range e2xs {

				// path for indexed file within current leaf folder
				fpath := filepath.Join(idxBase, path, file)

				res = append(res, fpath)
			}

			if verbose {
				fmt.Fprintf(os.Stderr, "INV %s\n", invFile)
			} else {
				// progress monitor
				countSuccess()
			}

			out <- res
		}

		visitIndexSubset := func(idxBase, invBase string, out chan<- []string) {

			defer close(out)

			dirs, _, _, _ := examineFolder(idxBase, "")

			// iterate through top directories
			for _, top := range dirs {
				if IsAllDigits(top) && len(top) == 2 {
					visitSubFolders(idxBase, invBase, top, out)
				}

				// force garbage collection
				runtime.GC()

				runtime.Gosched()
			}

			if rollingColumn > 0 {
				vlock.Lock()
				fmt.Fprintf(os.Stderr, "\n")
				vlock.Unlock()
			}
		}

		// launch single index visitor goroutine
		go visitIndexSubset(indexBase, invertBase, out)

		return out
	}

	vafq := visitIndexFolders(indexBase, invertBase)
	out := invertIndexFiles(invertBase, vafq)
	if vafq == nil || out == nil {
		DisplayError("Unable to create visitIndexFolders channel")
		os.Exit(1)
	}

	return out
}
