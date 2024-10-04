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
// File Name:  poster.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/klauspost/pgzip"
	"github.com/surgebase/porter2"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// POSTINGS FILE CREATION FROM MERGED INVERTED INDEX FILES

// CreatePromoters creates term lists and postings files from merged inverted
// index files. All specified fields (e.g., "TITL TIAB YEAR TREE") are completed
// in a single scan of the inverted files.
//
// Postings consist of three files (.mst, .trm, and .pst) for all terms, plus
// two additional files (.uqi and .ofs) for terms with position data.
//
// Master index files (with .mst suffixes) contain pairs of 32-bit values, in
// little endian form, pointing to an offset into the term list (.trm files,
// saved as lines of text words), and an offset into the postings list (.pst
// files,  containing 32-bit PMIDs).
//
// For position data, the .uqi file is parallel to the .pst file (one entry
// for each PMID associated with a given term), and contains 32-bit offsets
// to 16-bit paragraph position values.
//
// An extra entry at the end, pointing just past the end of data, allows
// the length of a term, or the size of a postings list, or the number of
// positions for a term in a given PMID, to be calculated as the difference
// between two adjacent pointers.
//
// The number of term positions per PMID is the term frequency (TF). The
// number of PMIDs per term is the document frequency (DF). All that remains
// for calculating TF-IDF term weights, which can support ranked retrieval,
// is the total number of live PubMed documents, which could easily be saved
// during indexing.
func CreatePromoters(prom, db, fields string, isLink bool, files []string) <-chan string {

	if files == nil {
		return nil
	}

	// obtain paths from environment variable(s)
	base, _ := GetLocalArchivePaths(db)

	if base == "" {

		DisplayError("Unable to get local postings path")
		os.Exit(1)
	}

	postingsBase := base + "Postings"

	// check to make sure local postings directory is mounted
	_, err := os.Stat(postingsBase)
	if err != nil && os.IsNotExist(err) {
		DisplayError("Local postings directory '%s' is not mounted", postingsBase)
		os.Exit(1)
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create promoter channel")
		os.Exit(1)
	}

	flds := strings.Split(fields, " ")

	// xmlPromoter saves records in a single set of term/posting files
	xmlPromoter := func(wg *sync.WaitGroup, fileName string, out chan<- string) {

		defer wg.Done()

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

		getOnePosting := func(field, text string) (string, []int32, []string) {

			var data []int32
			var atts []string

			term := ""

			doPromote := func(tag, attr, content string) {

				if tag == "InvKey" {

					// term used for postings file name
					term = content

					term = strings.ToLower(term)

				} else if tag == field {

					// convert UID string to integer
					if content == "" {
						DisplayError("Empty UID for term '%s'", term)
						return
					}
					value, err := strconv.ParseInt(content, 10, 32)
					if err != nil {
						fmt.Fprintf(os.Stderr, "%s\n", err.Error())
						return
					}
					data = append(data, int32(value))

					if strings.HasPrefix(attr, "pos=\"") {
						attr = attr[5:]
						lgth := len(attr)
						if lgth > 1 && attr[lgth-1] == '"' {
							// "
							attr = attr[:lgth-1]
						}
						atts = append(atts, attr)
					}
				}
			}

			// explore data fields
			StreamValues(text[:], "InvDocument", doPromote)

			if term == "" || len(data) < 1 {
				return "", nil, nil
			}

			return term, data, atts
		}

		var (
			termPos int32
			postPos int32
			ofstPos int32

			indxList bytes.Buffer
			termList bytes.Buffer
			postList bytes.Buffer
			uqidList bytes.Buffer
			ofstList bytes.Buffer
		)

		retlength := len("\n")

		addOnePosting := func(term string, data []int32, atts []string) {

			tlength := len(term)
			dlength := len(data)
			alength := len(atts)

			// write to term list buffer
			termList.WriteString(term[:])
			termList.WriteString("\n")

			// write to postings buffer
			binary.Write(&postList, binary.LittleEndian, data)

			// write to master index buffer
			binary.Write(&indxList, binary.LittleEndian, termPos)
			binary.Write(&indxList, binary.LittleEndian, postPos)

			postPos += int32(dlength * 4)
			termPos += int32(tlength + retlength)

			// return if no position attributes
			if alength < 1 {
				return
			}
			if dlength != alength {
				fmt.Fprintf(os.Stderr, "dlength %d, alength %d\n", dlength, alength)
				return
			}

			// write term offset list for each UID
			for _, attr := range atts {

				binary.Write(&uqidList, binary.LittleEndian, ofstPos)

				atrs := strings.Split(attr, ",")
				atln := len(atrs)
				for _, att := range atrs {
					if att == "" {
						continue
					}
					value, err := strconv.ParseInt(att, 10, 32)
					if err != nil {
						fmt.Fprintf(os.Stderr, "%s\n", err.Error())
						return
					}
					binary.Write(&ofstList, binary.LittleEndian, uint16(value))
				}

				ofstPos += int32(atln * 2)
			}
		}

		topOffMaster := func() {

			// phantom term and postings positions eliminates special case calculation at end
			binary.Write(&indxList, binary.LittleEndian, termPos)
			binary.Write(&indxList, binary.LittleEndian, postPos)
			binary.Write(&uqidList, binary.LittleEndian, ofstPos)
		}

		writeFile := func(dpath, fname string, bfr bytes.Buffer) {

			fpath := filepath.Join(dpath, fname)
			if fpath == "" {
				return
			}

			// overwrites and truncates existing file
			fl, err := os.Create(fpath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				return
			}

			data := bfr.Bytes()

			wrtr := bufio.NewWriter(fl)

			_, err = wrtr.Write(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			}

			wrtr.Flush()

			// fl.Sync()

			fl.Close()
		}

		writeFiveFiles := func(field, key string) {

			dpath, ky := PostingPath(postingsBase, field, key, isLink)
			if dpath == "" {
				return
			}

			// make subdirectories, if necessary
			err := os.MkdirAll(dpath, os.ModePerm)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				return
			}

			writeFile(dpath, ky+"."+field+".trm", termList)

			writeFile(dpath, ky+"."+field+".pst", postList)

			writeFile(dpath, ky+"."+field+".mst", indxList)

			// do not write position index and offset data files
			// for fields with no position attributes recorded
			if uqidList.Len() > 0 && ofstList.Len() > 0 {

				writeFile(dpath, ky+"."+field+".uqi", uqidList)

				writeFile(dpath, ky+"."+field+".ofs", ofstList)
			}
		}

		processOneField := func(field string, recs []string) {

			tag := ""

			for _, str := range recs {

				term, data, atts := getOnePosting(field, str)

				if term == "" || data == nil {
					continue
				}

				// use first few characters of identifier
				if tag == "" {
					if isLink {
						tag = term
						if len(tag) > LinkLen {
							tag = tag[:LinkLen]
						}
					} else {
						tag = IdentifierKey(term)
					}
				}

				addOnePosting(term, data, atts)
			}

			if tag != "" {

				topOffMaster()
				writeFiveFiles(field, tag)
			}

			// reset buffers and position counters
			termPos = 0
			postPos = 0
			ofstPos = 0

			indxList.Reset()
			termList.Reset()
			postList.Reset()
			uqidList.Reset()
			ofstList.Reset()
		}

		find := ParseIndex("InvKey")

		currTag := ""
		prevTag := ""

		var arry []string

		// read next array of InvDocument records with same key
		PartitionXML("InvDocument", "", false, rdr,
			func(str string) {

				id := FindIdentifier(str[:], "InvDocument", find)
				if id == "" {
					return
				}

				if isLink {
					if len(id) > LinkLen {
						id = id[:LinkLen]
					}
					currTag = id
				} else {
					// use first few characters of identifier
					currTag = IdentifierKey(id)
				}

				if prevTag != currTag {

					// after IdentifierKey converts space to underscore,
					// okay that xxx_ and xxx0 will be out of alphabetical order

					// records with same identifier key as a unit
					if prevTag != "" {
						for _, fld := range flds {
							processOneField(fld, arry)
						}
						out <- prevTag
					}

					// empty the slice
					arry = nil
				}

				// collect next InvDocument record
				arry = append(arry, str[:])

				prevTag = currTag
			})

		if arry != nil {

			// remaining records with last identifier key
			for _, fld := range flds {
				processOneField(fld, arry)
			}
			out <- prevTag
		}
	}

	var wg sync.WaitGroup

	// launch multiple promoter goroutines
	for _, str := range files {
		wg.Add(1)
		go xmlPromoter(&wg, str, out)
	}

	// launch separate anonymous goroutine to wait until all promoters are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// POSTINGS FILE LOW-LEVEL USAGE FUNCTIONS

// Master points to a term and to its postings data
type Master struct {
	TermOffset int32
	PostOffset int32
}

// Arrays contains postings lists and word offsets
type Arrays struct {
	Data []int32
	Ofst [][]uint16
	Dist int
}

func commonOpenFile(dpath, fname string) (*os.File, int64) {

	fpath := filepath.Join(dpath, fname)
	if fpath == "" {
		return nil, 0
	}

	inFile, err := os.Open(fpath)
	if err != nil && os.IsNotExist(err) {
		return nil, 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil, 0
	}

	fi, err := inFile.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil, 0
	}

	size := fi.Size()

	return inFile, size
}

func readMasterIndex(dpath, key, field string) []Master {

	inFile, size := commonOpenFile(dpath, key+"."+field+".mst")
	if inFile == nil {
		return nil
	}

	defer inFile.Close()

	data := make([]Master, size/8)
	if data == nil || len(data) < 1 {
		return nil
	}

	err := binary.Read(inFile, binary.LittleEndian, &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	return data
}

func readTermList(dpath, key, field string) []byte {

	inFile, size := commonOpenFile(dpath, key+"."+field+".trm")
	if inFile == nil {
		return nil
	}

	defer inFile.Close()

	data := make([]byte, size)
	if data == nil || len(data) < 1 {
		return nil
	}

	err := binary.Read(inFile, binary.LittleEndian, &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	return data
}

func readPostingData(dpath, key, field string, offset int32, size int32) []int32 {

	inFile, _ := commonOpenFile(dpath, key+"."+field+".pst")
	if inFile == nil {
		return nil
	}

	defer inFile.Close()

	data := make([]int32, size/4)
	if data == nil || len(data) < 1 {
		return nil
	}

	_, err := inFile.Seek(int64(offset), io.SeekStart)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	err = binary.Read(inFile, binary.LittleEndian, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	return data
}

func readPositionIndex(dpath, key, field string, offset int32, size int32) []int32 {

	inFile, _ := commonOpenFile(dpath, key+"."+field+".uqi")
	if inFile == nil {
		return nil
	}

	defer inFile.Close()

	data := make([]int32, size/4)
	if data == nil || len(data) < 1 {
		return nil
	}

	_, err := inFile.Seek(int64(offset), io.SeekStart)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	err = binary.Read(inFile, binary.LittleEndian, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	return data
}

func readOffsetData(dpath, key, field string, offset int32, size int32) []uint16 {

	inFile, _ := commonOpenFile(dpath, key+"."+field+".ofs")
	if inFile == nil {
		return nil
	}

	defer inFile.Close()

	data := make([]uint16, size/2)
	if data == nil || len(data) < 1 {
		return nil
	}

	_, err := inFile.Seek(int64(offset), io.SeekStart)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	err = binary.Read(inFile, binary.LittleEndian, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	}

	return data
}

func readMasterIndexFuture(dpath, key, field string) <-chan []Master {

	out := make(chan []Master, chanDepth)
	if out == nil {
		DisplayError("Unable to create master index channel")
		os.Exit(1)
	}

	// masterIndexFuture asynchronously gets a master file and sends results through channel
	masterIndexFuture := func(dpath, key, field string, out chan<- []Master) {

		data := readMasterIndex(dpath, key, field)

		out <- data

		close(out)
	}

	// launch single future goroutine
	go masterIndexFuture(dpath, key, field, out)

	return out
}

func readTermListFuture(dpath, key, field string) <-chan []byte {

	out := make(chan []byte, chanDepth)
	if out == nil {
		DisplayError("Unable to create term list channel")
		os.Exit(1)
	}

	// termListFuture asynchronously gets a term list file and sends results through channel
	termListFuture := func(dpath, key, field string, out chan<- []byte) {

		data := readTermList(dpath, key, field)

		out <- data

		close(out)
	}

	// launch single future goroutine
	go termListFuture(dpath, key, field, out)

	return out
}

func getPostingIDs(prom, term, field string, simple, isLink bool) ([]int32, [][]uint16) {

	dpath, key := PostingPath(prom, field, term, isLink)
	if dpath == "" {
		return nil, nil
	}

	// schedule asynchronous fetching
	mi := readMasterIndexFuture(dpath, key, field)

	tl := readTermListFuture(dpath, key, field)

	// fetch master index and term list
	indx := <-mi

	trms := <-tl

	if indx == nil || len(indx) < 1 {
		return nil, nil
	}

	if trms == nil || len(trms) < 1 {
		return nil, nil
	}

	// master index is padded with phantom term and postings position
	numTerms := len(indx) - 1

	strs := make([]string, numTerms)
	if strs == nil || len(strs) < 1 {
		return nil, nil
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

	// if term ends with dollar sign, use porter2 stemming, then add asterisk
	if strings.HasSuffix(term, "$") && term != "$" {
		term = strings.TrimSuffix(term, "$")
		term = porter2.Stem(term)
		term += "*"
	}

	isWildCard := false
	if strings.HasSuffix(term, "*") && term != "*" {
		tlen := len(term)
		isWildCard = true
		term = strings.TrimSuffix(term, "*")
		pdlen := len(PostingDir(term))
		if tlen < pdlen {
			fmt.Fprintf(os.Stderr, "Wildcard term '%s' must be at least %d characters long - ignoring this word\n", term, pdlen)
			return nil, nil
		}
	}

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

	// wild card search scans term lists, fuses adjacent postings lists
	if isWildCard {
		if R < numTerms && strings.HasPrefix(strs[R], term) {
			offset := indx[R].PostOffset
			for R < numTerms && strings.HasPrefix(strs[R], term) {
				R++
			}
			size := indx[R].PostOffset - offset

			// read relevant postings list section
			data := readPostingData(dpath, key, field, offset, size)
			if data == nil || len(data) < 1 {
				return nil, nil
			}

			if simple {

				merged := make(map[int32]bool)

				// combine all postings in term range
				for _, val := range data {
					merged[val] = true
				}

				fused := make([]int32, len(merged))

				// convert map to slice
				i := 0
				for num := range merged {
					fused[i] = num
					i++
				}

				slices.Sort(fused)

				return fused, nil
			}

			// read relevant word position section, includes phantom offset at end
			uqis := readPositionIndex(dpath, key, field, offset, size+4)
			if uqis == nil {
				return nil, nil
			}
			ulen := len(uqis)
			if ulen < 1 {
				return nil, nil
			}

			from := uqis[0]
			to := uqis[ulen-1]

			// read offset section
			ofst := readOffsetData(dpath, key, field, from, to-from)
			if ofst == nil {
				return nil, nil
			}

			combo := make(map[int32][]uint16)

			addPositions := func(uid int32, pos uint16) {

				arrs, ok := combo[uid]
				if !ok {
					arrs = make([]uint16, 0, 1)
				}
				arrs = append(arrs, pos)
				combo[uid] = arrs
			}

			// populate array of positions per UID
			for i, j, k := 0, 1, int32(0); i < ulen-1; i++ {
				uid := data[i]
				num := (uqis[j] - uqis[i]) / 2
				j++
				for q := k; q < k+num; q++ {
					addPositions(uid, ofst[q])
				}
				k += num
			}

			fused := make([]int32, len(combo))

			// convert map to slice
			i := 0
			for num := range combo {
				fused[i] = num
				i++
			}

			slices.Sort(fused)

			// make array of uint16 arrays, populate for each UID
			arrs := make([][]uint16, ulen-1)
			if arrs == nil {
				return nil, nil
			}

			for j, uid := range fused {
				posn := combo[uid]

				if len(posn) > 1 {
					slices.Sort(posn)
				}

				arrs[j] = posn
			}

			return fused, arrs
		}

		return nil, nil
	}

	// regular search requires exact match from binary search
	if R < numTerms && strs[R] == term {

		offset := indx[R].PostOffset
		size := indx[R+1].PostOffset - offset

		// read relevant postings list section
		data := readPostingData(dpath, key, field, offset, size)
		if data == nil || len(data) < 1 {
			return nil, nil
		}

		if simple {
			return data, nil
		}

		// read relevant word position section, includes phantom offset at end
		uqis := readPositionIndex(dpath, key, field, offset, size+4)
		if uqis == nil {
			return nil, nil
		}
		ulen := len(uqis)
		if ulen < 1 {
			return nil, nil
		}

		from := uqis[0]
		to := uqis[ulen-1]

		// read offset section
		ofst := readOffsetData(dpath, key, field, from, to-from)
		if ofst == nil {
			return nil, nil
		}

		// make array of uint16 arrays, populate for each UID
		arrs := make([][]uint16, ulen)
		if arrs == nil || len(arrs) < 1 {
			return nil, nil
		}

		// populate array of positions per UID
		for i, j, k := 0, 1, int32(0); i < ulen-1; i++ {
			num := (uqis[j] - uqis[i]) / 2
			j++
			arrs[i] = ofst[k : k+num]
			k += num
		}

		return data, arrs
	}

	return nil, nil
}

func postingIDsFuture(base, term, field string, dist int, isLink bool) <-chan Arrays {

	out := make(chan Arrays, chanDepth)
	if out == nil {
		DisplayError("Unable to create postings channel")
		os.Exit(1)
	}

	// postingFuture asynchronously gets posting IDs and sends results through channel
	postingFuture := func(base, term, field string, dist int, out chan<- Arrays) {

		data, ofst := getPostingIDs(base, term, field, false, isLink)

		out <- Arrays{Data: data, Ofst: ofst, Dist: dist}

		close(out)
	}

	// launch single future goroutine
	go postingFuture(base, term, field, dist, out)

	return out
}

// BOOLEAN OPERATIONS FOR POSTINGS LISTS

func extendPositionalIDs(N []int32, np [][]uint16, M []int32, mp [][]uint16, delta int, proc func(pn, pm []uint16, dlt uint16) []uint16) ([]int32, [][]uint16) {

	if proc == nil {
		return nil, nil
	}

	n, m := len(N), len(M)

	if n < 1 || len(np) < 1 {
		return M, mp
	}
	if m < 1 || len(mp) < 1 {
		return N, np
	}

	// order matters when extending phrase or testing proximity, do not swap lists based on size

	sz := n
	if sz > m {
		sz = m
	}

	if sz < 1 {
		return N, np
	}

	res := make([]int32, sz)
	ofs := make([][]uint16, sz)

	if res == nil || len(res) < 1 || ofs == nil || len(ofs) < 1 {
		return nil, nil
	}

	i, j, k := 0, 0, 0

	// use local variables for speed
	en, em := N[i], M[j]

	for {
		// do inequality tests first
		if en < em {
			i++
			if i == n {
				break
			}
			en = N[i]
		} else if en > em {
			j++
			if j == m {
				break
			}
			em = M[j]
		} else {
			// specific callbacks test position arrays to match terms by adjacency or phrases by proximity
			adj := proc(np[i], mp[j], uint16(delta))
			if adj != nil && len(adj) > 0 {
				res[k] = en
				ofs[k] = adj
				k++
			}
			i++
			j++
			if i == n || j == m {
				break
			}
			en = N[i]
			em = M[j]
		}
	}

	// truncate output arrays to actual size of intersection
	res = res[:k]
	ofs = ofs[:k]

	return res, ofs
}

func intersectIDs(N, M []int32) []int32 {

	n, m := len(N), len(M)

	// if either list is empty, intersection is empty
	if n < 1 || m < 1 {
		return nil
	}

	// swap to make M the smaller list
	if n < m {
		N, M = M, N
		n, m = m, n
	}

	if m < 1 {
		return N
	}

	res := make([]int32, m)

	i, j, k := 0, 0, 0

	// use local variables for speed
	en, em := N[i], M[j]

	for {
		// do inequality tests first
		if en < em {
			// index to larger list most likely to be advanced
			i++
			if i == n {
				break
			}
			en = N[i]
		} else if en > em {
			j++
			if j == m {
				break
			}
			em = M[j]
		} else {
			// equality (intersection match) least likely
			res[k] = en
			k++
			i++
			j++
			if i == n || j == m {
				break
			}
			en = N[i]
			em = M[j]
		}
	}

	// truncate output array to actual size of intersection
	res = res[:k]

	return res
}

// if m * log(n) < m + n, binary search has fewer comparisons, but processor memory caches make linear algorithm faster
/*
func intersectBinary(N, M []int32) []int32 {

	if N == nil {
		return M
	}
	if M == nil {
		return N
	}

	n, m := len(N), len(M)

	// swap to make M the smaller list
	if n < m {
		N, M = M, N
		n, m = m, n
	}

	if m < 1 {
		return N
	}

	k := 0

	res := make([]int32, m)

	for _, uid := range M {
		// inline binary search is faster than sort.Search
		L, R := 0, n-1
		for L < R {
			mid := (L + R) / 2
			if N[mid] < uid {
				L = mid + 1
			} else {
				R = mid
			}
		}
		// R := sort.Search(len(N), func(i int) bool { return N[i] >= uid })
		if R < n && N[R] == uid {
			res[k] = uid
			k++
			// remove leading part of N for slight speed gain
			N = N[R:]
			n = len(N)
		}
	}

	res = res[:k]

	return res
}
*/

func combineIDs(N, M []int32) []int32 {

	n, m := len(N), len(M)

	if n < 1 {
		return M
	}
	if m < 1 {
		return N
	}

	// swap to make M the smaller list
	if n < m {
		N, M = M, N
		n, m = m, n
	}

	if m < 1 {
		return N
	}

	i, j, k := 0, 0, 0

	res := make([]int32, n+m)

	for i < n && j < m {
		if N[i] < M[j] {
			res[k] = N[i]
			k++
			i++
		} else if N[i] > M[j] {
			res[k] = M[j]
			k++
			j++
		} else {
			res[k] = N[i]
			k++
			i++
			j++
		}
	}
	for i < n {
		res[k] = N[i]
		k++
		i++
	}
	for j < m {
		res[k] = M[j]
		k++
		j++
	}

	res = res[:k]

	return res
}

func excludeIDs(N, M []int32) []int32 {

	n, m := len(N), len(M)

	if n < 1 {
		return nil
	}
	if m < 1 {
		return N
	}

	res := make([]int32, n)

	i, j, k := 0, 0, 0

	// use local variables for speed
	en, em := N[i], M[j]

	for {
		// do inequality tests first
		if en < em {
			// item is not excluded
			res[k] = en
			k++
			i++
			if i == n {
				break
			}
			en = N[i]
		} else if en > em {
			// advance second list pointer
			j++
			if j == m {
				break
			}
			em = M[j]
		} else {
			// exclude
			i++
			j++
			if i == n || j == m {
				break
			}
			en = N[i]
			em = M[j]
		}
	}

	// truncate output array to actual size of result
	res = res[:k]

	return res
}
