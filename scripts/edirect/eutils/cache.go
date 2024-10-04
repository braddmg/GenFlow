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
// File Name:  cache.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// fetchOneXMLRecord is an internal function for fetching an XML record from the archive
func fetchOneXMLRecord(id, base, pfx, sfx string, zipp bool, buf bytes.Buffer) string {

	if id == "" {
		return ""
	}

	id = strings.TrimPrefix(id, "PMC")

	dir, file := ArchiveTrie(id)

	if dir == "" || file == "" {
		return ""
	}

	if zipp {
		sfx += ".gz"
	}

	fpath := filepath.Join(base, dir, pfx+file+sfx)
	if fpath == "" {
		return ""
	}

	iszip := zipp

	inFile, err := os.Open(fpath)

	// if failed to find ".xml" or ".e2x" file, try ".xml.gz" or ".e2x.gz" without requiring -gzip
	if err != nil && os.IsNotExist(err) && !zipp {
		iszip = true
		fpath := filepath.Join(base, dir, pfx+file+sfx+".gz")
		if fpath == "" {
			return ""
		}
		inFile, err = os.Open(fpath)
	}
	if err != nil {
		msg := err.Error()
		if !strings.HasSuffix(msg, "no such file or directory") && !strings.HasSuffix(msg, "cannot find the path specified.") {
			fmt.Fprintf(os.Stderr, "%s\n", msg)
		}
		return ""
	}

	defer inFile.Close()

	brd := bufio.NewReader(inFile)

	if iszip {

		zpr, err := gzip.NewReader(brd)

		defer zpr.Close()

		if err == nil {
			// copy and decompress cached file contents
			buf.ReadFrom(zpr)
		}

	} else {

		// copy cached file contents
		buf.ReadFrom(brd)
	}

	str := buf.String()

	return str
}

// FetchLocalRecord returns the XML for a single UID
func FetchLocalRecord(id, db, pfx, ptrn string) string {

	if id == "" || db == "" {
		return ""
	}

	if pfx != "" {
		id = strings.TrimPrefix(id, pfx)
	}

	var buf bytes.Buffer

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	archiveBase := base + "Archive"

	// check to make sure local archive is mounted
	_, err := os.Stat(archiveBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local archive and search index is not mounted")
		os.Exit(1)
	}

	str := fetchOneXMLRecord(id, archiveBase, pfx, ".xml", true, buf)

	// trim any header included in archive XML file
	if str != "" && ptrn != "" {
		pos := strings.Index(str, "<"+ptrn+">")
		if pos > 0 {
			// remove any leading xml and DOCTYPE lines
			str = str[pos:]
		}
	}

	return str
}

// GzipString allows separate compression of xml + DOCTYPE header and PubmedArticle record
func GzipString(str string) []byte {

	if str == "" {
		return nil
	}

	var buf bytes.Buffer

	// for small files, use regular gzip
	gzWriter, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	gzWriter.Write([]byte(str))
	gzWriter.Close()

	return buf.Bytes()
}

// CreateUIDReader sends PMIDs and their numeric orders down a channel.
// This allows detection of updated records that appear shortly after
// earlier versions, preventing the wrong version from being saved.
func CreateUIDReader(in io.Reader) <-chan XMLRecord {

	if in == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create uid reader channel")
		os.Exit(1)
	}

	// uidReader reads uids from input stream and sends through channel
	uidReader := func(in io.Reader, out chan<- XMLRecord) {

		// close channel when all records have been processed
		defer close(out)

		scanr := bufio.NewScanner(in)

		idx := 0
		for scanr.Scan() {

			// read lines of identifiers
			file := scanr.Text()
			idx++

			pos := strings.Index(file, ".")
			if pos >= 0 {
				// remove version suffix
				file = file[:pos]
			}

			out <- XMLRecord{Index: idx, Text: file}
		}
	}

	// launch single uid reader goroutine
	go uidReader(in, out)

	return out
}

// ReadsUIDsFromString reads a comma-separated string of uids and sends through channel
func ReadsUIDsFromString(uids string) <-chan XMLRecord {

	if uids == "" {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create uid string reader channel")
		os.Exit(1)
	}

	// uidStringReader reads uids from input stream and sends through channel
	uidStringReader := func(uids string, out chan<- XMLRecord) {

		// close channel when all records have been processed
		defer close(out)

		items := strings.Split(uids, ",")

		idx := 0
		for _, item := range items {

			idx++

			pos := strings.Index(item, ".")
			if pos >= 0 {
				// remove version suffix
				item = item[:pos]
			}

			out <- XMLRecord{Index: idx, Text: item}
		}
	}

	// launch single uid string reader goroutine
	go uidStringReader(uids, out)

	return out
}

func mapXMLtoASN(node *XMLNode, proc func(string)) {

	if node == nil || proc == nil {
		return
	}

	// array to speed up indentation
	indentSpaces := []string{
		"",
		"  ",
		"    ",
		"      ",
		"        ",
		"          ",
		"            ",
		"              ",
		"                ",
		"                  ",
	}

	// indent a specified number of spaces
	doIndent := func(indt int) {
		i := indt
		for i > 9 {
			proc("                    ")
			i -= 10
		}
		if i < 0 {
			return
		}
		proc(indentSpaces[i])
	}

	afix := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
		"&apos;", "'",
		"&#39;", "'",
		"&quot;", "'",
		"&#34;", "'",
		"\"", "'",
	)

	// doASNtree recursive definition
	var doASNtree func(*XMLNode, int, bool)

	doASNtree = func(curr *XMLNode, depth int, comma bool) {

		// suppress if it would be an empty self-closing tag
		if !IsNotJustWhitespace(curr.Attributes) && curr.Contents == "" && curr.Children == nil {
			return
		}

		name := curr.Name
		if name == "" {
			name = "_"
		}

		// leading hyphen  - unnamed braces (element of SEQUENCE OF or SET OF structured objects)
		// trailing hyphen - unquoted value
		// internal hyphen - convert to space
		show := true
		quot := true
		if strings.HasPrefix(name, "_") {
			show = false
		} else if strings.HasSuffix(name, "_") {
			name = strings.TrimSuffix(name, "_")
			quot = false
		}
		name = strings.Replace(name, "_", " ", -1)

		if curr.Contents != "" {

			doIndent(depth)
			proc(name)
			proc(" ")
			if quot {
				proc("\"")
			}

			str := curr.Contents[:]
			if HasBadSpace(str) {
				str = CleanupBadSpaces(str)
			}
			if IsNotASCII(str) {
				str = TransformAccents(str, false, false)
			}
			if HasAdjacentSpaces(str) {
				str = CompressRunsOfSpaces(str)
			}
			str = afix.Replace(str)
			proc(str)

			if quot {
				proc("\"")
			}

		} else {

			doIndent(depth)
			if show {
				proc(name)
				proc(" ")
			}
			if depth == 0 {
				proc("::= ")
			}
			proc("{\n")

			for chld := curr.Children; chld != nil; chld = chld.Next {
				// do not print comma after last child object in chain
				doASNtree(chld, depth+1, (chld.Next != nil))
			}

			doIndent(depth)
			proc("}")
		}

		if comma {
			proc(",")
		}
		proc("\n")
	}

	doASNtree(node, 0, false)
}

// XMLDoctypeGzipLen is length of gzipped xml + DOCTYPE header, added as separate gzip packet in front of each compressed record
const XMLDoctypeGzipLen = 183

// CreateStashers saves records to archive, multithreaded for performance, use of UID
// position index allows it to prevent earlier version from overwriting later version
func CreateStashers(stsh, parent, indx, pfx, sfx, db, xmlString string, transform map[string]string, hash, zipp, asn bool, report int, inp <-chan XMLRecord) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create stasher channel")
		os.Exit(1)
	}

	find := ParseIndex(indx)

	if zipp {
		sfx += ".gz"
	}

	type StasherType int

	const (
		OKAY StasherType = iota
		WAIT
		BAIL
	)

	// mutex to protect access to inUse map
	var wlock sync.Mutex

	// map to track files currently being written
	inUse := make(map[string]int)

	// lockFile function prevents colliding writes
	lockFile := func(id string, index int) StasherType {

		// map is non-reentrant, protect with mutex
		wlock.Lock()

		// multiple return paths, schedule the unlock command up front
		defer wlock.Unlock()

		idx, ok := inUse[id]

		if ok {
			if index < idx {
				// later version is being written by another goroutine, skip this
				return BAIL
			}
			// earlier version is being written by another goroutine, wait
			return WAIT
		}

		// okay to write file, mark in use to prevent collision
		inUse[id] = index
		return OKAY
	}

	// freeFile function removes entry from inUse map
	freeFile := func(id string) {

		wlock.Lock()

		// free entry in map, later versions of same record can now be written
		delete(inUse, id)

		wlock.Unlock()
	}

	// mutex to protect access to rollingCount variable
	var tlock sync.Mutex

	rollingCount := 0

	countSuccess := func() {

		tlock.Lock()

		rollingCount++
		if rollingCount >= report {
			rollingCount = 0
			// print dot (progress monitor)
			fmt.Fprintf(os.Stderr, ".")
		}

		tlock.Unlock()
	}

	var xmlDoctype []byte

	// make gzip-compressed byte array of xml + DOCTYPE header
	if xmlString != "" {
		xmlDoctype = GzipString(xmlString)
	}

	if db == "pubmed" {

		// reality check on expected length
		pmaHeadLen := len(xmlDoctype)
		// if it changes due to a new release date, or new gzip software,
		// delete the Archive, remove Pubmed sentinel files, update constant
		// in source code, and reindex
		if pmaHeadLen != XMLDoctypeGzipLen {
			fmt.Fprintf(os.Stdout, "Unexpected xmlDoctype length %d\n\n", pmaHeadLen)
		}
	}

	// stashRecord saves individual XML record to archive file accessed by trie
	stashRecord := func(str, id string, index int) string {

		pos := strings.Index(id, ".")
		if pos >= 0 {
			// remove version from UID
			id = id[:pos]
		}

		dir, file := ArchiveTrie(id)

		if dir == "" || file == "" {
			return ""
		}

		if asn {
			// convert customized XML to ASN.1 before saving
			curr := ParseRecord(str, parent)

			var buffer strings.Builder

			mapXMLtoASN(curr,
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
			}
			str = txt
		} else if db == "pmc" {
			// for XML, add trailing newline, needed for streaming without decompressing
			if !strings.HasSuffix(str, "\n") {
				str += "\n"
			}
		} else {
			// for XML, add trailing newline, needed for streaming without decompressing
			if !strings.HasSuffix(str, "\n") {
				str += "\n"
			}
		}

		attempts := 5
		keepChecking := true

		for keepChecking {
			// check if file is not being written by another goroutine
			switch lockFile(id, index) {
			case OKAY:
				// okay to save this record now
				keepChecking = false
			case WAIT:
				// earlier version is being saved, wait one second and try again
				time.Sleep(time.Second)
				attempts--
				if attempts < 1 {
					// could not get lock after several attempts
					DisplayError("Unable to save '%s'", id)
					return ""
				}
			case BAIL:
				// later version is being saved, skip this one
				return ""
			default:
			}
		}

		// delete lock after writing file
		defer freeFile(id)

		dpath := filepath.Join(stsh, dir)
		if dpath == "" {
			return ""
		}
		err := os.MkdirAll(dpath, os.ModePerm)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return ""
		}
		fpath := filepath.Join(dpath, pfx+file+sfx)
		if fpath == "" {
			return ""
		}

		// overwrites and truncates existing file
		fl, err := os.Create(fpath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return ""
		}

		res := id

		if hash {
			// calculate hash code for verification table
			hsh := crc32.NewIEEE()
			hsh.Write([]byte(str))
			val := hsh.Sum32()
			res = strconv.FormatUint(uint64(val), 10)
		}

		if zipp {

			if !asn && db == "pubmed" {
				fl.Write(xmlDoctype)
			}

			zp := GzipString(str)
			fl.Write(zp)

		} else {

			// copy uncompressed record to file
			fl.WriteString(str)
			if !strings.HasSuffix(str, "\n") {
				fl.WriteString("\n")
			}
		}

		// fl.Sync()

		err = fl.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return ""
		}

		// progress monitor prints dot every 1000 (.xml or .asn) or 50000 (.e2x) records
		countSuccess()

		return res
	}

	// xmlStasher reads from channel and calls stashRecord
	xmlStasher := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- string) {

		defer wg.Done()

		for ext := range inp {

			ident := FindIdentifier(ext.Text, parent, find)

			// optional lookup of integer identifier from accession.version string
			txt, found := transform[ident]
			if found {
				ident = txt
			}

			ext.Ident = ident

			// skip BioC records with explicit 'unknown' identifier
			if ext.Ident == "unknown" {
				continue
			}

			hsh := stashRecord(ext.Text, ext.Ident, ext.Index)

			res := ext.Ident
			if hash {
				res += "\t" + hsh
			}
			res += "\n"

			runtime.Gosched()

			out <- res
		}
	}

	var wg sync.WaitGroup

	// launch multiple stasher goroutines
	for range numServe {
		wg.Add(1)
		go xmlStasher(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all stashers are done
	go func() {
		wg.Wait()
		close(out)
		// print newline after rows of dots (progress monitor)
		fmt.Fprintf(os.Stderr, "\n")
	}()

	return out
}

// CreateDeleter reads PMIDs, deletes them in the archive, and sends them
// down a channel to have the affected inverted index cache files removed.
func CreateDeleter(stsh, db string, in io.Reader) <-chan string {

	if stsh == "" || in == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create deleter channel")
		os.Exit(1)
	}

	recordDeleter := func(in io.Reader, out chan<- string) {

		// close channel when all records have been processed
		defer close(out)

		scanr := bufio.NewScanner(in)

		verbose := false
		// set verbose flag from environment variable
		env := os.Getenv("EDIRECT_LOCAL_VERBOSE")
		if env == "Y" || env == "y" || env == "true" {
			verbose = true
		}

		for scanr.Scan() {

			// read lines of identifiers
			id := scanr.Text()

			id = strings.TrimPrefix(id, "PMC")
			id = strings.TrimSuffix(id, "\n")

			pos := strings.Index(id, ".")
			if pos >= 0 {
				// remove version suffix
				id = id[:pos]
			}

			dir, file := ArchiveTrie(id)

			if dir == "" || file == "" {
				continue
			}

			dpath := filepath.Join(stsh, dir, file+".xml.gz")
			if dpath == "" {
				continue
			}

			os.Remove(dpath)
			if verbose {
				fmt.Fprintf(os.Stderr, "DEL PMD %s\n", dpath)
			}

			out <- id
		}
	}

	// launch single deleter goroutine
	go recordDeleter(in, out)

	return out
}

// CreateClearer clears incremental index files whose underlying records have changed
func CreateClearer(indexBase, invertBase, db string, inp <-chan string) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create clearer channel")
		os.Exit(1)
	}

	// maps to track index and inverted files that were deleted
	deletedIdx := make(map[string]bool)

	deletedInv := make(map[string]bool)

	clearStaleIndices := func(inp <-chan string, out chan<- string) {

		// close channel when all records have been processed
		defer close(out)

		verbose := false
		// set verbose flag from environment variable
		env := os.Getenv("EDIRECT_LOCAL_VERBOSE")
		if env == "Y" || env == "y" || env == "true" {
			verbose = true
		}

		for id := range inp {

			id = strings.TrimPrefix(id, "PMC")
			id = strings.TrimSuffix(id, "\n")

			pos := strings.Index(id, ".")
			if pos >= 0 {
				// remove version from UID
				id = id[:pos]
			}

			xdir, idx := IndexTrie(id)
			if xdir == "" || idx == "" {
				continue
			}

			xpath := filepath.Join(indexBase, xdir, idx+".e2x.gz")
			deletedIdx[xpath] = true

			vdir, inv := InvertTrie(id)
			if vdir == "" || inv == "" {
				continue
			}

			vpath := filepath.Join(invertBase, vdir, inv+".inv.gz")
			deletedInv[vpath] = true

			out <- id
		}

		// read (uniqued) maps and delete stale index and inverted files
		for str := range deletedIdx {

			os.Remove(str)
			if verbose {
				fmt.Fprintf(os.Stderr, "DEL IDX %s\n", str)
			}
		}

		for str := range deletedInv {

			os.Remove(str)
			if verbose {
				fmt.Fprintf(os.Stderr, "DEL INV %s\n", str)
			}
		}
	}

	// launch single clearer goroutine
	go clearStaleIndices(inp, out)

	return out
}

// CreateFetchers returns uncompressed records from archive, multithreaded for speed
func CreateFetchers(stsh, db, pfx, sfx, ptrn string, zipp bool, inp <-chan XMLRecord) <-chan XMLRecord {

	if inp == nil || stsh == "" {
		return nil
	}

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	archiveBase := base + "Archive"

	// check to make sure local archive is mounted
	_, err := os.Stat(archiveBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local archive is not mounted")
		os.Exit(1)
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create fetcher channel")
		os.Exit(1)
	}

	// xmlFetcher reads XML from file
	xmlFetcher := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- XMLRecord) {

		// report when more records to process
		defer wg.Done()

		var buf bytes.Buffer

		for ext := range inp {

			buf.Reset()

			str := fetchOneXMLRecord(ext.Text, stsh, pfx, sfx, zipp, buf)

			// trim any header included in archive XML file
			if str != "" && ptrn != "" {
				pos := strings.Index(str, "<"+ptrn+">")
				if pos > 0 {
					// remove any leading xml and DOCTYPE lines
					str = str[pos:]
				}
			}

			runtime.Gosched()

			out <- XMLRecord{Index: ext.Index, Ident: ext.Ident, Text: str}
		}
	}

	var wg sync.WaitGroup

	// launch multiple fetcher goroutines
	for range numServe {
		wg.Add(1)
		go xmlFetcher(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all fetchers are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// CreateCacheStreamers returns compressed records from archive, multithreaded for speed,
// could be used for sending records over network to be decompressed later by client
func CreateCacheStreamers(stsh, db, pfx, sfx string, skip int, inp <-chan XMLRecord) <-chan XMLRecord {

	if inp == nil || stsh == "" {
		return nil
	}

	// obtain path from environment variable
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local archive path")
		os.Exit(1)
	}

	archiveBase := base + "Archive"

	// check to make sure local archive is mounted
	_, err := os.Stat(archiveBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local archive is not mounted")
		os.Exit(1)
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create streamer channel")
		os.Exit(1)
	}
	sfx += ".gz"

	getRecord := func(id string, buf bytes.Buffer) []byte {

		if id == "" {
			return nil
		}

		dir, file := ArchiveTrie(id)

		if dir == "" || file == "" {
			return nil
		}

		fpath := filepath.Join(stsh, dir, pfx+file+sfx)
		if fpath == "" {
			return nil
		}

		inFile, err := os.Open(fpath)

		if err != nil {
			msg := err.Error()
			if !strings.HasSuffix(msg, "no such file or directory") && !strings.HasSuffix(msg, "cannot find the path specified.") {
				fmt.Fprintf(os.Stderr, "%s\n", msg)
			}
			return nil
		}

		brd := bufio.NewReader(inFile)

		// copy cached file contents
		buf.ReadFrom(brd)

		data := buf.Bytes()

		inFile.Close()

		return data
	}

	// xmlStreamer reads compressed XML from file
	xmlStreamer := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- XMLRecord) {

		// report when more records to process
		defer wg.Done()

		var buf bytes.Buffer

		for ext := range inp {

			buf.Reset()

			data := getRecord(ext.Text, buf)

			runtime.Gosched()

			// skip past first gzip packet with xml and DOCTYPE
			if skip > 0 {
				if len(data) > skip {
					data = data[skip:]
				}
			} else if db == "pubmed" {
				if len(data) > XMLDoctypeGzipLen {
					data = data[XMLDoctypeGzipLen:]
				}
			}

			out <- XMLRecord{Index: ext.Index, Ident: ext.Ident, Data: data}
		}
	}

	var wg sync.WaitGroup

	// launch multiple streamer goroutines
	for range numServe {
		wg.Add(1)
		go xmlStreamer(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all streamers are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
