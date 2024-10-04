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
// File Name:  merge.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"container/heap"
	"fmt"
	"github.com/klauspost/pgzip"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
)

// Plex allows distribution of indexing
type Plex struct {
	Which int
	Ident string
	Text  string
	Index int
	Sibs  []string
}

// PlexHeap methods satisfy heap.Interface
type PlexHeap []Plex

func (h PlexHeap) Len() int {
	return len(h)
}
func (h PlexHeap) Less(i, j int) bool {
	return h[i].Ident < h[j].Ident
}
func (h PlexHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Push works on pointer to PlexHeap
func (h *PlexHeap) Push(x interface{}) {
	*h = append(*h, x.(Plex))
}

// Pop works on pointer to PlexHeap
func (h *PlexHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// CreatePresenters creates one channel per input file
func CreatePresenters(files []string) []<-chan Plex {

	if files == nil {
		return nil
	}

	numFiles := len(files)
	if numFiles < 1 {
		DisplayError("Not enough inverted files to merge")
		os.Exit(1)
	}

	chns := make([]<-chan Plex, numFiles)
	if chns == nil {
		DisplayError("Unable to create presenter channel array")
		os.Exit(1)
	}

	// xmlPresenter sends partitioned XML strings through channel
	xmlPresenter := func(fileNum int, fileName string, out chan<- Plex) {

		// close channel when all records have been processed
		defer close(out)

		f, err := os.Open(fileName)
		if err != nil {
			DisplayError("Unable to open input file '%s'", fileName)
			os.Exit(1)
		}

		// close input file when all records have been processed
		defer f.Close()

		var in io.Reader

		in = f

		// if suffix is ".gz", use decompressor
		iszip := false
		if strings.HasSuffix(fileName, ".gz") {
			iszip = true
		}

		if iszip {
			brd := bufio.NewReader(f)
			if brd == nil {
				DisplayError("Unable to create buffered reader on '%s'", fileName)
				os.Exit(1)
			}
			// using parallel pgzip for better performance on large files
			zpr, err := pgzip.NewReader(brd)
			if err != nil {
				DisplayError("Unable to create decompressor on '%s'", fileName)
				os.Exit(1)
			}

			// close decompressor when all records have been processed
			defer zpr.Close()

			// use decompressor for reading file
			in = zpr
		}

		rdr := CreateXMLStreamer(in, nil)

		if rdr == nil {
			DisplayError("Unable to create XML Block Reader")
			os.Exit(1)
		}

		find := ParseIndex("InvKey")

		// partition all input by pattern and send XML substring through channel
		PartitionXML("InvDocument", "", false, rdr,
			func(str string) {
				id := FindIdentifier(str[:], "InvDocument", find)

				out <- Plex{fileNum, id, str, 0, nil}
			})
	}

	// launch multiple presenter goroutines
	for i, str := range files {

		chn := make(chan Plex, chanDepth)
		if chn == nil {
			DisplayError("Unable to create presenter channel")
			os.Exit(1)
		}

		go xmlPresenter(i, str, chn)

		chns[i] = chn
	}

	// no need for separate anonymous goroutine to wait until all presenters are done

	return chns
}

// CreateManifold reads from each file, sends merged postings in sorted order
func CreateManifold(inp []<-chan Plex) <-chan Plex {

	if inp == nil {
		return nil
	}

	out := make(chan Plex, chanDepth)
	if out == nil {
		DisplayError("Unable to create manifold channel")
		os.Exit(1)
	}

	// xmlManifold restores alphabetical order of merged postings
	xmlManifold := func(inp []<-chan Plex, out chan<- Plex) {

		// close channel when all records have been processed
		defer close(out)

		// initialize empty heap
		hp := &PlexHeap{}
		heap.Init(hp)

		// read first object from all input channels in turn
		for _, chn := range inp {
			plx, ok := <-chn
			if ok {
				heap.Push(hp, plx)
			}
		}

		// array to collect strings with same identifier
		var arry []string

		prevIdent := ""
		rec := 0

		// reading from heap returns objects in alphabetical order
		for hp.Len() > 0 {

			// remove lowest item from heap, use interface type assertion
			curr := heap.Pop(hp).(Plex)

			// compare adjacent record identifiers
			if prevIdent == curr.Ident {

				// save next inverted object string in slice
				arry = append(arry, curr.Text)

			} else {

				if len(arry) > 0 {

					rec++
					// send set from previous identifier to output channel
					out <- Plex{0, prevIdent, "", rec, arry}

					// empty the slice
					arry = nil

					runtime.Gosched()
				}

				// remember new identifier
				prevIdent = curr.Ident

				// save first inverted object with this identifier
				arry = append(arry, curr.Text)
			}

			// read next object from channel that just supplied lowest item
			chn := inp[curr.Which]
			plx, ok := <-chn
			if ok {
				heap.Push(hp, plx)
			}
		}

		if len(arry) > 0 {

			rec++
			// send last record
			out <- Plex{0, prevIdent, "", rec, arry}

			arry = nil
		}
	}

	// launch single manifold goroutine
	go xmlManifold(inp, out)

	return out
}

// CreateFusers collects all inverted indices for a given term
func CreateFusers(inp <-chan XMLRecord) <-chan Plex {

	if inp == nil {
		return nil
	}

	out := make(chan Plex, chanDepth)
	if out == nil {
		DisplayError("Unable to create fuser channel")
		os.Exit(1)
	}

	var flock sync.Mutex

	// map for combining inverted indices
	inverted := make(map[string][]string)

	addInverts := func(id, str string) {

		// protect map with mutex
		flock.Lock()

		data, ok := inverted[id]
		if !ok {
			data = make([]string, 0, 1)
		}

		data = append(data, str)
		// always need to update inverted, since data may be reallocated
		inverted[id] = data

		// unlock at end to avoid defer overhead
		flock.Unlock()
	}

	xmlFuser := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- Plex) {

		defer wg.Done()

		find := ParseIndex("InvKey")

		// read partitioned XML from producer channel
		for ext := range inp {

			str := ext.Text[:]
			id := FindIdentifier(str, "InvDocument", find)
			addInverts(id, str)
		}
	}

	var wg sync.WaitGroup

	// launch multiple fuser goroutines
	for range numProcs {
		wg.Add(1)
		go xmlFuser(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all fusers are done
	go func() {
		wg.Wait()

		// sort id keys in alphabetical order
		keys := slices.Sorted(maps.Keys(inverted))

		rec := 0

		for _, id := range keys {

			arry := inverted[id]

			rec++
			// send array of records with same identifier to output channel
			out <- Plex{0, id, "", rec, arry}

			// empty the slice
			arry = nil
		}

		close(out)
	}()

	return out
}

// CreateJoiners repackages adjacent indices for the same term without fusing
func CreateJoiners(inp <-chan Plex) <-chan XMLRecord {

	if inp == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create merger channel")
		os.Exit(1)
	}

	// xmlJoiner unpacks adjacent InvDocument records with the same identifier
	xmlJoiner := func(inp <-chan Plex, out chan<- XMLRecord) {

		defer close(out)

		for plx := range inp {

			rec := plx.Index
			key := plx.Ident
			data := plx.Sibs

			if len(data) < 1 {
				continue
			}

			for _, str := range data {
				out <- XMLRecord{Index: rec, Ident: key, Text: str[:]}
			}

			runtime.Gosched()
		}
	}

	// launch single joiner goroutine
	go xmlJoiner(inp, out)

	return out
}

// CreateMergers combines collected indices for the same term
func CreateMergers(inp <-chan Plex) <-chan XMLRecord {

	if inp == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create merger channel")
		os.Exit(1)
	}

	// xmlMerger fuses adjacent InvDocument records with the same identifier
	xmlMerger := func(wg *sync.WaitGroup, inp <-chan Plex, out chan<- XMLRecord) {

		defer wg.Done()

		fusePostings := func(key string, data []string) string {

			fields := make(map[string]map[string]string)

			addIdents := func(fld, pos, uid string) {

				// no need for mutex here
				positions, ok := fields[fld]
				if !ok {
					positions = make(map[string]string)
					fields[fld] = positions
				}

				positions[uid] = pos
			}

			addUID := func(tag, attr, content string) {

				if tag != "InvKey" {

					addIdents(tag, attr, content)
				}
			}

			for _, str := range data {
				StreamValues(str[:], "InvDocument", addUID)
			}

			// sort fields in alphabetical order
			keys := slices.Sorted(maps.Keys(fields))

			var res []string

			for _, fld := range keys {

				positions := fields[fld]

				arry := slices.SortedFunc(maps.Keys(positions), CompareAlphaOrNumericKeys)

				// print list of UIDs, skipping duplicates
				last := ""
				for _, uid := range arry {
					// detect duplicate UIDs, now in same list after conversion of one term entry from foreign alphabet
					if uid == last {
						continue
					}

					atr := positions[uid]
					if atr != "" {
						str := "<" + fld + " " + atr + ">" + uid + "</" + fld + ">"
						res = append(res, str)
					} else {
						str := "<" + fld + ">" + uid + "</" + fld + ">"
						res = append(res, str)
					}

					last = uid
				}
			}

			itms := strings.Join(res, "\n")

			return "<InvDocument>\n<InvKey>" + key + "</InvKey>\n<InvIDs>\n" + itms + "\n</InvIDs>\n</InvDocument>\n"
		}

		for plx := range inp {

			rec := plx.Index
			key := plx.Ident
			data := plx.Sibs

			if len(data) < 1 {
				continue
			}

			str := fusePostings(key, data)

			out <- XMLRecord{Index: rec, Ident: key, Text: str}

			runtime.Gosched()
		}
	}

	var wg sync.WaitGroup

	// launch multiple merger goroutines
	for range numProcs {
		wg.Add(1)
		go xmlMerger(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all mergers are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// CreateSplitter distributes adjacent records with the same identifier prefix
func CreateSplitter(mergePath, db string, zipp, isLink bool, inp <-chan XMLRecord) <-chan string {

	if inp == nil {
		return nil
	}

	// obtain paths from environment variable(s)
	_, working := GetLocalArchivePaths(db)

	if working == "" {

		DisplayError("Unable to get local merge path")
		os.Exit(1)
	}

	mergeBase := working + "Merged"

	// check to make sure local merge directory is mounted
	_, err := os.Stat(mergeBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local merge directory '%s' is not mounted", mergeBase)
		os.Exit(1)
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create splitter channel")
		os.Exit(1)
	}

	openSaver := func(mergeBase, key string, zipp bool) (*os.File, *bufio.Writer, *pgzip.Writer) {

		var (
			fl   *os.File
			wrtr *bufio.Writer
			zpr  *pgzip.Writer
			err  error
		)

		sfx := ".mrg"
		if zipp {
			sfx += ".gz"
		}

		fpath := filepath.Join(mergeBase, key+sfx)
		if fpath == "" {
			return nil, nil, nil
		}

		// overwrites and truncates existing file
		fl, err = os.Create(fpath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return nil, nil, nil
		}

		var out io.Writer

		out = fl

		if zipp {

			// using parallel pgzip for better performance on large files
			zpr, err = pgzip.NewWriterLevel(fl, pgzip.BestSpeed)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				return nil, nil, nil
			}

			out = zpr
		}

		// create buffered writer layer
		wrtr = bufio.NewWriter(out)
		if wrtr == nil {
			DisplayError("Unable to create bufio.NewWriter")
			return nil, nil, nil
		}

		return fl, wrtr, zpr
	}

	closeSaver := func(fl *os.File, wrtr *bufio.Writer, zpr *pgzip.Writer) {

		wrtr.Flush()
		if zpr != nil {
			zpr.Close()
		}
		// fl.Sync()

		err := fl.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		}
	}

	// xmlSplitter distributes adjacent records with the same identifier prefix
	xmlSplitter := func(inp <-chan XMLRecord, out chan<- string) {

		// close channel when all records have been processed
		defer close(out)

		var (
			fl   *os.File
			wrtr *bufio.Writer
			zpr  *pgzip.Writer
		)

		currTag := ""
		prevTag := ""

		// remember previous record
		prev := XMLRecord{}

		getCurrTag := func(ident string) string {

			// links always use first 4 characters of zero-padded identifier
			if isLink {
				if len(ident) > LinkLen {
					ident = ident[:LinkLen]
				}
				return ident
			}

			// use first few characters of identifier
			tag := IdentifierKey(ident)
			if tag == "" {
				return ""
			}

			// then truncate to 2, 3, or 4 character prefix
			if len(tag) > 2 {
				key := tag[:2]
				num, ok := TrieLen[key]
				if ok {
					if num > 3 && len(tag) > 3 {
						key = tag[:3]
						num, ok = MergLen[key]
						if ok && num > 3 {
							tag = tag[:4]
						} else {
							tag = tag[:3]
						}
					} else if num > 2 {
						tag = tag[:3]
					} else {
						tag = tag[:2]
					}
				} else {
					tag = tag[:2]
				}
			}

			return tag
		}

		for curr := range inp {

			// use first few characters of identifier
			currTag = getCurrTag(curr.Ident)
			if currTag == "" {
				continue
			}

			if fl == nil {
				// open initial file
				fl, wrtr, zpr = openSaver(mergeBase, currTag, zipp)
				if wrtr == nil {
					continue
				}

				// send first opening tag and indent
				wrtr.WriteString("<InvDocumentSet>\n  ")
			}

			// compare keys from adjacent term lists
			if prev.Text != "" && prevTag != currTag {

				// after IdentifierKey converts space to underscore,
				// okay that x_ and x0 will be out of alphabetical order

				// send closing tag
				wrtr.WriteString("</InvDocumentSet>\n")

				closeSaver(fl, wrtr, zpr)

				out <- currTag

				// force garbage collection
				runtime.GC()
				debug.FreeOSMemory()

				runtime.Gosched()

				// open next file
				fl, wrtr, zpr = openSaver(mergeBase, currTag, zipp)
				if wrtr == nil {
					continue
				}

				// send opening tag and indent
				wrtr.WriteString("<InvDocumentSet>\n  ")
			}

			// send one InvDocument
			str := strings.TrimSpace(curr.Text)

			wrtr.WriteString(str)
			if !strings.HasSuffix(str, "\n") {
				wrtr.WriteString("\n")
			}

			// now remember this record
			prev = curr

			prevTag = currTag
		}

		if prev.Text != "" {

			// send last closing tag
			wrtr.WriteString("</InvDocumentSet>\n")

			closeSaver(fl, wrtr, zpr)

			out <- currTag

			// force garbage collection
			runtime.GC()
			debug.FreeOSMemory()

			runtime.Gosched()
		}
	}

	// launch single splitter goroutine
	go xmlSplitter(inp, out)

	return out
}
