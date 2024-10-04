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
// File Name:  xml.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"bytes"
	"container/heap"
	"html"
	"io"
	"maps"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// READ XML INPUT FILE INTO CHANNEL OF TRIMMED BLOCKS

// XMLBlock is a string that begins with a left angle bracket and is trimmed back to
// end with a right angle bracket. The excluded characters are saved and prepended
// to the next buffer. Providing complete object tags simplifies subsequent parsing.
type XMLBlock string

// CreateXMLStreamer reads XML input into a channel of trimmed strings that are
// then split by PartitionXML into individual records (which can be processed
// concurrently), or parsed directly into a channel of tokens by CreateTokenizer.
func CreateXMLStreamer(in io.Reader, inp <-chan string) <-chan XMLBlock {

	if inp != nil {

		out := make(chan XMLBlock, chanDepth)
		if out == nil {
			DisplayError("Unable to create XML block reader channel")
			os.Exit(1)
		}

		bufferXMLChannel := func(inp <-chan string, out chan<- XMLBlock) {

			// close channel when all records have been processed
			defer close(out)

			remainder := ""

			for str := range inp {

				idx := strings.LastIndex(str, ">")
				// still need to check doStrict and call local version of htmlBehind
				if idx >= 0 {
					idx++
					prev := remainder
					remainder = str[idx:]
					str = str[:idx]
					rslt := prev + str
					rslt = strings.TrimSpace(rslt)
					rslt = CompressRunsOfSpaces(rslt)
					rslt = strings.TrimPrefix(rslt, " ")
					rslt = strings.TrimSuffix(rslt, " ")
					out <- XMLBlock(rslt)
				} else {
					remainder += str
				}
			}

			if remainder != "" {
				remainder = strings.TrimSpace(remainder)
				remainder = CompressRunsOfSpaces(remainder)
				remainder = strings.TrimPrefix(remainder, " ")
				remainder = strings.TrimSuffix(remainder, " ")
				out <- XMLBlock(remainder)
			}
		}

		// launch single buffering goroutine
		go bufferXMLChannel(inp, out)

		return out
	}

	if in == nil {
		return nil
	}

	out := make(chan XMLBlock, chanDepth)
	if out == nil {
		DisplayError("Unable to create XML block reader channel")
		os.Exit(1)
	}

	// xmlReader sends trimmed XML blocks through the output channel.
	xmlReader := func(in io.Reader, out chan<- XMLBlock) {

		// close channel when all blocks have been processed
		defer close(out)

		// 65536 appears to be the maximum number of characters presented to io.Reader
		// when input is piped from stdin. Increasing the buffer size when input is from
		// a file does not improve program performance. An additional 16384 bytes are
		// reserved for copying the previous remainder to the beginning of the buffer
		// before the next read.
		const XMLBUFSIZE = 65536 + 16384

		buffer := make([]byte, XMLBUFSIZE)
		remainder := ""
		position := int64(0)
		delta := 0
		isClosed := false

		// htmlBehind is used in strict mode to trim back further when a lower-case tag
		// is encountered. This may be a formatting decoration, such as <i> or </i> for
		// italics. Processing HTML, which may have embedded mixed content, requires use
		// of mixed mode.
		htmlBehind := func(bufr []byte, pos, txtlen int) bool {

			for pos >= 0 {
				if bufr[pos] == '<' {
					// detect lower-case markup tags, or DispFormula in PubMed
					return HTMLAhead(string(bufr), pos, txtlen) != 0
				}
				pos--
			}

			return false
		}

		// nextBuffer reads one buffer, trims back to the right-most > character, and
		// retains the remainder for prepending in the next call. It also signals if
		// there was no > character, resulting in subsequent calls to nextBuffer to
		// continue reading a large content string.
		nextBuffer := func() ([]byte, bool, bool) {

			if isClosed {
				return nil, false, true
			}

			// prepend previous remainder to beginning of buffer
			m := copy(buffer, remainder)
			remainder = ""
			if m > 16384 {
				// previous remainder is larger than reserved section,
				// write and signal the need to continue reading.
				return buffer[:m], true, false
			}

			// read next block, append behind copied remainder from previous read
			n, err := in.Read(buffer[m:])
			// with data piped through stdin, read function may not always return the
			// same number of bytes each time
			if err != nil {
				if err != io.EOF {
					// real error.
					DisplayError("%s", err.Error())
					// ignore bytes - non-conforming implementations of io.Reader may
					// return mangled data on non-EOF errors
					isClosed = true
					return nil, false, true
				}
				// end of file.
				isClosed = true
				if n == 0 {
					// if EOF and no more data, do not send final remainder (not terminated
					// by right angle bracket that is used as a sentinel)
					return nil, false, true
				}
			}
			if n < 0 {
				// reality check - non-conforming implementations of io.Reader may return -1
				DisplayError("io.Reader returned negative count %d", n)
				// treat as n == 0 in order to update file offset and avoid losing previous remainder
				n = 0
			}

			// keep track of file offset
			position += int64(delta)
			delta = n

			// slice of actual characters read
			bufr := buffer[:n+m]

			// Look for last > character. It is safe to back up on UTF-8 rune array when looking
			// for a 7-bit ASCII character.
			pos := -1
			for pos = len(bufr) - 1; pos >= 0; pos-- {
				if bufr[pos] == '>' {
					if doStrict {
						// optionally skip backwards past embedded i, b, u, sub, and sup
						// HTML open, close, and empty tags, and MathML instructions
						if htmlBehind(bufr, pos, len(bufr)) {
							continue
						}
					}
					// found end of XML tag, break
					break
				}
			}

			// trim back to last > character, save remainder for next buffer
			if pos > -1 {
				pos++
				remainder = string(bufr[pos:])
				return bufr[:pos], false, false
			}

			// no > found, signal need to continue reading long content
			return bufr[:], true, false
		}

		// nextBlock reads buffer, concatenates if necessary to place long element content
		// into a single string. All result strings end in > character that is used as a
		// sentinel in subsequent code.
		nextBlock := func() string {

			// read next buffer
			line, cont, closed := nextBuffer()

			if closed {
				// no sentinel in remainder at end of file
				return ""
			}

			if cont {
				// current line does not end with > character
				var buff bytes.Buffer

				// keep reading long content blocks
				for {
					if len(line) > 0 {
						buff.Write(line)
					}
					if !cont {
						// last buffer ended with sentinel
						break
					}
					line, cont, closed = nextBuffer()
					if closed {
						// no sentinel in multi-block buffer at end of file
						return ""
					}
				}

				// concatenate blocks
				return buff.String()
			}

			return string(line)
		}

		// read XML and send blocks through channel
		for {
			str := nextBlock()

			// trimming spaces here would throw off line tracking

			// optionally compress/cleanup tags/attributes and contents
			if doCleanup {
				if HasBadSpace(str) {
					str = CleanupBadSpaces(str)
				}
				if HasAdjacentSpaces(str) {
					str = CompressRunsOfSpaces(str)
				}
			}

			out <- XMLBlock(str)

			// bail after sending empty string sentinel
			if str == "" {
				return
			}
		}
	}

	// launch single block reader goroutine
	go xmlReader(in, out)

	return out
}

// PARSE XML BLOCK STREAM INTO STRINGS FROM <PATTERN> TO </PATTERN>

// PartitionXML splits XML input from <pattern> to </pattern> and sends individual
// records to a callback. Requiring the input to be an XMLBlock channel of trimmed
// strings, generated by CreateXMLStreamer, simplifies the code by eliminating the
// need to check for an incomplete object tag at the end.
func PartitionXML(pat, star string, turbo bool, inp <-chan XMLBlock, proc func(string)) {

	if pat == "" || inp == nil || proc == nil {
		return
	}

	patlen := len(pat)

	// position of last character in pattern
	last := patlen - 1

	var skip [256]int

	// initialize Boyer-Moore-Horspool bad character displacement table
	for i := range skip {
		skip[i] = patlen
	}
	for i := range last {
		ch := pat[i]
		skip[ch] = last - i
	}

	// isAnElement checks surroundings of match candidate
	isAnElement := func(text string, lf, rt, mx int) bool {

		if (lf >= 0 && text[lf] == '<') || (lf > 0 && text[lf] == '/' && text[lf-1] == '<') {
			if (rt < mx && (text[rt] == '>' || text[rt] == ' ' || text[rt] == '\n')) ||
				(rt+1 < mx && text[rt] == '/' && text[rt+1] == '>') {
				return true
			}
		}

		return false
	}

	// findNextMatch is a modified Boyer-Moore-Horspool search function for maximum partitioning speed
	findNextMatch := func(text string, offset int) (int, int, int) {

		if text == "" {
			return -1, -1, -1
		}

		txtlen := len(text)

		max := txtlen - patlen
		last := patlen - 1

		i := offset

		for i <= max {

			// start at right-most character
			j := last
			k := i + last
			for j >= 0 && text[k] == pat[j] {
				j--
				k--
			}

			// require match candidate to be element name, i.e.,
			// <pattern ... >, </pattern ... >, or <pattern ... />
			if j < 0 && isAnElement(text, i-1, i+patlen, txtlen) {

				// find positions of flanking angle brackets
				lf := i - 1
				for lf > 0 && text[lf] != '<' {
					lf--
				}
				rt := i + patlen
				for rt < txtlen && text[rt] != '>' {
					rt++
				}
				return i + 1, lf, rt + 1
			}

			// find character in text above last character in pattern
			ch := text[i+last]
			// displacement table can shift pattern by one or more positions
			i += skip[ch]
		}

		return -1, -1, -1
	}

	// pattern type keys for XML parsing
	const (
		noPat = iota
		startPat
		selfPat
		stopPat
	)

	// nextPattern finds next element with pattern name
	nextPattern := func(text string, pos int) (int, int, int, int) {

		if text == "" {
			return noPat, 0, 0, 0
		}

		prev := pos

		for {
			next, start, stop := findNextMatch(text, prev)
			if next < 0 {
				return noPat, 0, 0, 0
			}

			prev = next + 1

			if text[start+1] == '/' {
				return stopPat, start, stop, prev
			} else if text[stop-2] == '/' {
				return selfPat, start, stop, prev
			} else {
				return startPat, start, stop, prev
			}
		}
	}

	// doNormal handles -pattern Object construct, keeping track of nesting level
	doNormal := func() {

		// current depth of -pattern objects
		level := 0

		inPattern := false

		var accumulator strings.Builder

		for {

			match := noPat
			start := 0
			stop := 0
			next := 0

			begin := 0

			text := string(<-inp)
			if text == "" {
				return
			}

			for {
				match, start, stop, next = nextPattern(text, next)
				if match == startPat {
					if level == 0 {
						inPattern = true
						begin = start
					}
					level++
				} else if match == stopPat {
					level--
					if level == 0 {
						inPattern = false
						accumulator.WriteString(text[begin:stop])
						// read and process one -pattern object at a time
						str := accumulator.String()
						if str != "" {
							proc(str[:])
						}
						// reset accumulator
						accumulator.Reset()
					}
				} else if match == selfPat {
					if level == 0 {
						str := text[start:stop]
						if str != "" {
							proc(str[:])
						}
					}
				} else {
					if inPattern {
						accumulator.WriteString(text[begin:])
					}
					break
				}
			}
		}
	}

	// doTurbo reads an XML file that has NEXT_RECORD_SIZE objects with
	// the number of bytes to read to get the next indexed pattern
	doTurbo := func() {

		var accumulator strings.Builder

		for {

			// read next XMLBlock ending with '>' character
			text := string(<-inp)
			if text == "" {
				return
			}

			// should be at next NEXT_RECORD_SIZE object
			for {

				// find start tag of next record size object
				idx := strings.Index(text, "<NEXT_RECORD_SIZE>")
				if idx < 0 {
					break
				}
				text = text[idx+18:]

				// if end of buffer, read next XMLBlock
				if text == "" {
					text = string(<-inp)
					if text == "" {
						return
					}
				}

				// find stop tag of next record size object
				idx = strings.Index(text, "</NEXT_RECORD_SIZE>")
				if idx < 0 {
					break
				}
				str := text[:idx]
				text = text[idx+19:]
				if strings.HasPrefix(text, "\n") {
					text = text[1:]
				}

				// convert object value to integer
				size, err := strconv.Atoi(str)
				if err != nil {
					break
				}

				accumulator.Reset()

				for {
					// size of remaining text in block
					max := len(text)

					// is record completely contained in current block
					if size < max {
						rec := text[:size]
						text = text[size:]
						// prepend any text collected from previous blocks
						prev := accumulator.String()
						res := prev + rec
						res = strings.TrimPrefix(res, "\n")
						res = strings.TrimSuffix(res, "\n")
						proc(res[:])
						break
					}

					// otherwise record remainder of block
					accumulator.WriteString(text)
					// decrement remaining size
					size -= len(text)
					// read next block
					text = string(<-inp)
					if text == "" {
						// last record on final block
						res := accumulator.String()
						res = strings.TrimPrefix(res, "\n")
						res = strings.TrimSuffix(res, "\n")
						proc(res[:])
						return
					}
					// and keep going until desired size is collected
				}
			}
		}
	}

	// doStar handles -pattern Parent/* construct for heterogeneous objects. It can work
	// with concatenated files, but not if components are recursive or self-closing objects.
	// Process the latter through transmute -format -self first.
	doStar := func() {

		// nextStarPat scans for next XML element matching the current inner pattern
		nextStarPat := func(scr *BMHSearcher, text string, offset int) (int, int, int, int) {

			if scr == nil || text == "" {
				return noPat, -1, -1, -1
			}

			ptyp := noPat
			start := 0
			stop := 0
			prev := 0

			txtlen := len(text)

			scr.Search(text[:], offset,
				func(str, ptn string, pos int) bool {

					patlen := len(ptn)

					// require match candidate to be element name, i.e.,
					// <pattern ... >, </pattern ... >, or <pattern ... />
					if isAnElement(text[:], pos-1, pos+patlen, txtlen) {

						// find positions of flanking angle brackets
						lf := pos - 1
						for lf > 0 && text[lf] != '<' {
							lf--
						}
						rt := pos + patlen
						for rt < txtlen && text[rt] != '>' {
							rt++
						}

						// save function return values
						prev = pos + 1
						start = lf
						stop = rt + 1

						// switch on type of match
						if text[start+1] == '/' {
							ptyp = stopPat
						} else if text[stop-2] == '/' {
							ptyp = selfPat
						} else {
							ptyp = startPat
						}

						// callback signal to end search
						return false
					}

					// keep going to next candidate
					return true
				})

			return ptyp, start, stop, prev
		}

		text := ""

		match := noPat
		start := 0
		stop := 0
		next := 0

		// read to first <pattern> element
		for {

			next = 0

			text = string(<-inp)
			if text == "" {
				break
			}

			match, start, stop, next = nextPattern(text, next)
			if match == startPat {
				break
			}
		}

		if match != startPat {
			return
		}

		// current depth of -pattern objects
		level := 0

		begin := 0

		inPattern := false

		var accumulator strings.Builder

		last := ""

		// string search for inner objects
		var scr *BMHSearcher

		// read and process heterogeneous objects immediately below <pattern> parent
		for {

			// find next element in XML
			nextElement := func(txt string, pos int) string {

				txtlen := len(txt)

				tag := ""
				for i := pos; i < txtlen; i++ {
					if txt[i] == '<' {
						tag = txt[i+1:]
						break
					}
				}
				if tag == "" {
					return ""
				}
				if tag[0] == '/' {
					if strings.HasPrefix(tag[1:], pat) {
						//should be </pattern> at end, want to continue if concatenated files
						return "/"
					}
					return ""
				}
				for i, ch := range tag {
					if ch == '>' || ch == ' ' || ch == '/' {
						return tag[0:i]
					}
				}

				return ""
			}

			tag := nextElement(text, next)
			if tag == "" {

				begin = 0
				next = 0

				text = string(<-inp)
				if text == "" {
					break
				}

				tag = nextElement(text, next)
			}
			if tag == "" {
				return
			}

			// check for concatenated parent set files
			if tag[0] == '/' {
				// confirm end </pattern> just found
				match, start, stop, next = nextPattern(text, next)
				if match != stopPat {
					return
				}
				// now look for a new start <pattern> tag
				for {
					match, start, stop, next = nextPattern(text, next)
					if match == startPat {
						break
					}
					next = 0
					text = string(<-inp)
					if text == "" {
						break
					}
				}
				if match != startPat {
					return
				}
				// continue with processing loop
				continue
			}

			if tag != last {
				// search for inner pattern (may change for heterogeneous sets)
				scr = StringSearcher(tag, true, false, false, false, false)
				if scr == nil {
					return
				}
				last = tag
			}

			for {
				match, start, stop, next = nextStarPat(scr, text, next)
				if match == startPat {
					if level == 0 {
						inPattern = true
						begin = start
					}
					level++
				} else if match == stopPat {
					level--
					if level == 0 {
						inPattern = false
						accumulator.WriteString(text[begin:stop])
						// read and process one -pattern/* object at a time
						str := accumulator.String()
						if str != "" {
							proc(str[:])
						}
						// reset accumulator
						accumulator.Reset()
						break
					}
				} else if match == selfPat {
					if level == 0 {
						str := text[start:stop]
						if str != "" {
							proc(str[:])
						}
					}
				} else {
					if inPattern {
						accumulator.WriteString(text[begin:])
					}

					begin = 0
					next = 0

					text = string(<-inp)
					if text == "" {
						break
					}
				}
			}
		}
	}

	// call appropriate handler
	if turbo {
		doTurbo()
	} else if star == "" {
		doNormal()
	} else if star == "*" {
		doStar()
	}
}

// XMLRecord wraps a numbered XML record or the results of data extraction on
// that record. The Index field stores the record's original position in the
// input stream. The Data field is used for binary compressed PubmedArticle XML.
type XMLRecord struct {
	Index int
	Ident string
	Text  string
	Data  []byte
}

// CreateXMLProducer partitions an XML set and sends records down a channel.
// After processing asynchronously in multiple concurrent go routines, the
// original order can be restored by passage through the XMLUnshuffler.
func CreateXMLProducer(pat, star string, turbo bool, rdr <-chan XMLBlock) <-chan XMLRecord {

	if rdr == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create XML producer channel")
		os.Exit(1)
	}

	// xmlProducer sends partitioned XML strings through channel.
	xmlProducer := func(pat, star string, turbo bool, rdr <-chan XMLBlock, out chan<- XMLRecord) {

		// close channel when all records have been processed
		defer close(out)

		rec := 0

		// partition all input by pattern and send XML substring to available consumer through channel
		PartitionXML(pat, star, turbo, rdr,
			func(str string) {
				rec++
				out <- XMLRecord{rec, "", str, nil}
			})
	}

	// launch single producer goroutine
	go xmlProducer(pat, star, turbo, rdr, out)

	return out
}

// UNSHUFFLER USES HEAP TO RESTORE OUTPUT OF MULTIPLE CONSUMERS TO ORIGINAL RECORD ORDER

// xmlRecordHeap collects asynchronous processing results for presentation in the original order.
type xmlRecordHeap []XMLRecord

// methods that satisfy heap.Interface
func (h xmlRecordHeap) Len() int {
	return len(h)
}
func (h xmlRecordHeap) Less(i, j int) bool {
	return h[i].Index < h[j].Index
}
func (h xmlRecordHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
func (h *xmlRecordHeap) Push(x interface{}) {
	*h = append(*h, x.(XMLRecord))
}
func (h *xmlRecordHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// CreateXMLUnshuffler passes the output of multiple concurrent processors to
// a heap, which releases results in the same order as the original records.
func CreateXMLUnshuffler(inp <-chan XMLRecord) <-chan XMLRecord {

	if inp == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create XML unshuffler channel")
		os.Exit(1)
	}

	// xmlUnshuffler restores original order with heap.
	xmlUnshuffler := func(inp <-chan XMLRecord, out chan<- XMLRecord) {

		// close channel when all records have been processed
		defer close(out)

		// initialize empty heap
		hp := &xmlRecordHeap{}
		heap.Init(hp)

		// index of next desired result
		next := 1

		delay := 0

		for ext := range inp {

			// push result onto heap
			heap.Push(hp, ext)

			// Read several values before checking to see if next record to print has been processed.
			// The default heapSize value has been tuned by experiment for maximum performance.
			if delay < heapSize {
				delay++
				continue
			}

			delay = 0

			for hp.Len() > 0 {

				// remove lowest item from heap, use interface type assertion
				curr := heap.Pop(hp).(XMLRecord)

				if curr.Index > next {

					// record should be printed later, push back onto heap
					heap.Push(hp, curr)
					// and go back to waiting on input channel
					break
				}

				// send even if empty to get all record counts for reordering
				out <- XMLRecord{curr.Index, curr.Ident, curr.Text, curr.Data}

				// prevent ambiguous -limit filter from clogging heap (deprecated)
				if curr.Index == next {
					// increment index for next expected match
					next++
				}

				// continue to check heap to see if next result is already available
			}
		}

		// flush remainder of heap to output
		for hp.Len() > 0 {
			curr := heap.Pop(hp).(XMLRecord)

			out <- XMLRecord{curr.Index, curr.Ident, curr.Text, curr.Data}
		}
	}

	// launch single unshuffler goroutine
	go xmlUnshuffler(inp, out)

	return out
}

// CONCURRENT CONSUMER GOROUTINES PARSE AND PROCESS PARTITIONED XML OBJECTS

// StreamBlocks -> SplitPattern => XmlParse => StreamTokens => ProcessExtract -> MergeResults

// processes with single goroutine call defer close(out) so consumer(s) can range over channel
// processes with multiple instances call defer wg.Done(), separate goroutine uses wg.Wait() to delay close(out)

// CreateXMLConsumers runs multiple query processing go routines
func CreateXMLConsumers(cmds *Block, parent, hd, tl string, transform map[string]string, forClassify bool, histogram map[string]int, inp <-chan XMLRecord) <-chan XMLRecord {

	if inp == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create consumer channel")
		os.Exit(1)
	}

	var srchr *FSMSearcher

	if forClassify {
		var patterns []string
		for ky := range transform {
			patterns = append(patterns, ky)
		}
		// initialize string searcher from transform table
		srchr = PatternSearcher(patterns, false, true, true, false, false)
	}

	// xmlConsumer reads partitioned XML from channel and calls parser for processing
	xmlConsumer := func(cmds *Block, parent string, wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- XMLRecord) {

		// report when this consumer has no more records to process
		defer wg.Done()

		// read partitioned XML from producer channel
		for ext := range inp {

			idx := ext.Index
			ident := ext.Ident
			text := ext.Text

			if text == "" {
				// should never see empty input data
				out <- XMLRecord{Index: idx, Ident: ident, Text: text}
				continue
			}

			str := ProcessExtract(text[:], parent, idx, hd, tl, transform, srchr, histogram, cmds)

			// send even if empty to get all record counts for reordering
			out <- XMLRecord{Index: idx, Ident: ident, Text: str}
		}
	}

	var wg sync.WaitGroup

	// launch multiple consumer goroutines
	for range numServe {
		wg.Add(1)
		go xmlConsumer(cmds, parent, &wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all consumers are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// -select SUPPORT FUNCTIONS

// CreateSelectors supports xtract -select parent/element@attribute^version -in file_of_identifiers
func CreateSelectors(parent, indx string, order map[string]bool, inp <-chan XMLRecord) <-chan XMLRecord {

	if parent == "" || indx == "" || order == nil || inp == nil {
		return nil
	}

	find := ParseIndex(indx)

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create selector channel")
		os.Exit(1)
	}

	// xmlSelector reads partitioned XML from channel and matches identifiers of records to keep
	xmlSelector := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- XMLRecord) {

		// report when this selector has no more records to process
		defer wg.Done()

		// read partitioned XML from producer channel
		for ext := range inp {

			text := ext.Text

			found := false

			FindIdentifiers(text[:], parent, find,
				func(id string) {
					id = SortStringByWords(id)
					_, ok := order[id]
					if ok {
						found = true
					}
				})

			if !found {
				// identifier field not found or not in identifier list, send empty placeholder for unshuffler
				out <- XMLRecord{Index: ext.Index, Ident: ext.Ident}
				continue
			}

			// send selected record
			out <- XMLRecord{Index: ext.Index, Ident: ext.Ident, Text: text}
		}
	}

	var wg sync.WaitGroup

	// launch multiple selector goroutines
	for range numServe {
		wg.Add(1)
		go xmlSelector(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all selectors are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// CreateUnicoders supports xtract -pattern record_name -select -nonascii
func CreateUnicoders(inp <-chan XMLRecord) <-chan XMLRecord {

	if inp == nil {
		return nil
	}

	out := make(chan XMLRecord, chanDepth)
	if out == nil {
		DisplayError("Unable to create selector channel")
		os.Exit(1)
	}

	// xmlUnicoder reads partitioned XML from channel and keeps records with non-ASCII characters
	xmlUnicoder := func(wg *sync.WaitGroup, inp <-chan XMLRecord, out chan<- XMLRecord) {

		// report when this selector has no more records to process
		defer wg.Done()

		// read partitioned XML from producer channel
		for ext := range inp {

			text := ext.Text

			if !IsNotASCII(text) {
				// if only ASCII, send empty placeholder for unshuffler
				out <- XMLRecord{Index: ext.Index, Ident: ext.Ident}
				continue
			}

			// send selected record
			out <- XMLRecord{Index: ext.Index, Ident: ext.Ident, Text: text}
		}
	}

	var wg sync.WaitGroup

	// launch multiple unicoder goroutines
	for range numServe {
		wg.Add(1)
		go xmlUnicoder(&wg, inp, out)
	}

	// launch separate anonymous goroutine to wait until all unicoders are done
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// DRAIN OUTPUT CHANNEL TO EXECUTE EXTRACTION COMMANDS, RESTORE OUTPUT ORDER WITH HEAP

// DrainExtractions reads from the unshuffler and writes XML extraction output,
// for xtract and for rchive -e2index if used without -e2invert
func DrainExtractions(head, tail, posn string, mpty, idnt bool, histogram map[string]int, inp <-chan XMLRecord) (int, int) {

	if inp == nil {
		return 0, 0
	}

	recordCount := 0
	byteCount := 0

	var buffer strings.Builder
	count := 0
	okay := false
	lastTime := time.Now()

	wrtr := bufio.NewWriter(os.Stdout)

	// printResult prints output for current pattern, handles -empty and -ident flags, and periodically flushes buffer
	printResult := func(curr XMLRecord) {

		str := curr.Text

		if mpty {

			if str == "" {

				okay = true

				idx := curr.Index
				val := strconv.Itoa(idx)
				buffer.WriteString(val[:])
				buffer.WriteString("\n")

				count++
			}

		} else if str != "" {

			okay = true

			if idnt {
				idx := curr.Index
				val := strconv.Itoa(idx)
				buffer.WriteString(val[:])
				buffer.WriteString("\t")
			}

			// save output to byte buffer
			buffer.WriteString(str[:])

			count++
		}

		thisTime := time.Now()
		duration := thisTime.Sub(lastTime)
		milliSeconds := duration.Milliseconds()

		if count > 1000 || milliSeconds > 4999 {
			count = 0
			lastTime = thisTime
			txt := buffer.String()
			if txt != "" {
				// print current buffer
				wrtr.WriteString(txt[:])
			}
			buffer.Reset()
		}
	}

	if head != "" {
		buffer.WriteString(head[:])
		buffer.WriteString("\n")
	}

	// drain unshuffler channel

	if posn == "outer" {

		// print only first and last records
		var beg *XMLRecord
		var end *XMLRecord

		for curr := range inp {

			if beg == nil {
				beg = &XMLRecord{Index: curr.Index, Ident: curr.Ident, Text: curr.Text}
			} else {
				end = &XMLRecord{Index: curr.Index, Ident: curr.Ident, Text: curr.Text}
			}

			recordCount++
		}

		if beg != nil {
			printResult(*beg)
		}
		if end != nil {
			printResult(*end)
		}

	} else if posn == "inner" {

		// print all but first and last records
		var prev *XMLRecord
		var next *XMLRecord
		first := true

		for curr := range inp {

			if first {
				first = false
			} else {
				prev = next
				next = &XMLRecord{Index: curr.Index, Ident: curr.Ident, Text: curr.Text}
			}

			if prev != nil {
				printResult(*prev)
			}

			recordCount++
		}

	} else if posn == "even" {

		even := false

		for curr := range inp {

			if even {
				printResult(curr)
			}
			even = !even

			recordCount++
		}

	} else if posn == "odd" {

		odd := true

		for curr := range inp {

			if odd {
				printResult(curr)
			}
			odd = !odd

			recordCount++
		}

	} else {

		// default or -position all
		for curr := range inp {

			// send result to output
			printResult(curr)

			recordCount++
			runtime.Gosched()
		}
	}

	if tail != "" {
		buffer.WriteString(tail[:])
		buffer.WriteString("\n")
	}

	// do not print head or tail if no extraction output
	if okay {
		txt := buffer.String()
		if txt != "" {
			// print final buffer
			wrtr.WriteString(txt[:])
		}
	}
	buffer.Reset()

	wrtr.Flush()

	// print -histogram results, if populated
	keys := slices.SortedFunc(maps.Keys(histogram), CompareAlphaOrNumericKeys)

	for _, str := range keys {

		count := histogram[str]
		val := strconv.Itoa(count)
		os.Stdout.WriteString(val)
		os.Stdout.WriteString("\t")
		os.Stdout.WriteString(str)
		os.Stdout.WriteString("\n")
	}

	// force garbage collection and return memory before calculating processing rate
	debug.FreeOSMemory()

	return recordCount, byteCount
}

// PARSE XML INTO TOKENS, IDENTIFIERS, OR STRUCTURED RECORD OBJECT

// XML token type
const (
	NOTAG = iota
	STARTTAG
	SELFTAG
	STOPTAG
	ATTRIBTAG
	CONTENTTAG
	CDATATAG
	COMMENTTAG
	DOCTYPETAG
	PROCESSTAG
	OBJECTTAG
	CONTAINERTAG
	ISCLOSED
	BADTAG
)

// content bit flags for performing special cleanup steps only when needed
const (
	NONE  = iota
	MIXED = 1 << iota
	AMPER
	ASCII
	LFTSPACE
	RGTSPACE
)

// internal tracking state for detecting unrecognized mixed content
const (
	_ = iota
	START
	STOP
	CHAR
	OTHER
)

// XMLNode is the node for an internal tree structure representing a single XML record
type XMLNode struct {
	Name       string
	Parent     string
	Contents   string
	Attributes string
	Attribs    []string
	Children   *XMLNode
	Next       *XMLNode
}

// XMLFind contains individual field values for finding a particular object
type XMLFind struct {
	Index  string
	Parent string
	Match  string
	Attrib string
	Versn  string
}

// XMLToken is the unit of XML parsing
type XMLToken struct {
	Tag   int
	Cont  int
	Name  string
	Attr  string
	Index int
	Line  int
}

// ParseAttributes produces tag/value pairs, only run on request
func ParseAttributes(attrb string) []string {

	if attrb == "" {
		return nil
	}

	attlen := len(attrb)

	// count equal signs
	num := 0
	inQuote := false

	for i := range attlen {
		ch := attrb[i]
		if ch == '"' || ch == '\'' {
			// "
			inQuote = !inQuote
		}
		if ch == '=' && !inQuote {
			num += 2
		}
	}
	if num < 1 {
		return nil
	}

	// allocate array of proper size
	arry := make([]string, num)
	if arry == nil {
		return nil
	}

	start := 0
	idx := 0
	itm := 0
	inQuote = false

	// place tag and value in successive array slots
	for idx < attlen && itm < num {
		ch := attrb[idx]
		if ch == '"' || ch == '\'' {
			// "
			inQuote = !inQuote
		}
		if ch == '=' && !inQuote {
			inQuote = true
			// skip past possible leading blanks
			for start < attlen {
				ch = attrb[start]
				if inBlank[ch] {
					start++
				} else {
					break
				}
			}
			// =
			arry[itm] = strings.TrimSpace(attrb[start:idx])
			itm++
			// skip past equal sign
			idx++
			ch = attrb[idx]
			if ch != '"' && ch != '\'' {
				// "
				// skip past unexpected blanks
				for inBlank[ch] {
					idx++
					ch = attrb[idx]
				}
				if ch != '"' && ch != '\'' {
					// "
					DisplayError("Attribute in '%s' missing double quote", attrb)
				}
			}
			// skip past leading double quote
			idx++
			start = idx
		} else if ch == '"' || ch == '\'' {
			// "
			inQuote = false
			arry[itm] = strings.TrimSpace(attrb[start:idx])
			itm++
			// skip past trailing double quote and (possible) space
			idx += 2
			start = idx
		} else {
			idx++
		}
	}

	return arry
}

// parseXML calls XML parser on a partitioned string or on an XMLBlock channel of trimmed strings.
// It is optimized for maximum processing speed, sends tokens for CDATA and COMMENT sections (for
// unpacking by NormalizeXML), and optionally tracks line numbers (for ValidateXML).
func parseXML(record, parent string, inp <-chan XMLBlock, tokens func(XMLToken), find *XMLFind, ids func(string)) (*XMLNode, string) {

	if record == "" && (inp == nil || tokens == nil) {
		return nil, ""
	}

	if record != "" {
		// logic to skip past leading blanks relies on right angle bracket sentinel at end of string
		record = strings.TrimSpace(record)
	}

	// token parser variables
	recLen := len(record)
	Idx := 0

	// line tracking variables
	lineNum := 1
	lag := 0

	// variables to track COMMENT or CDATA sections that span reader blocks
	which := NOTAG
	skipTo := ""

	// updateLineCount is used to keep track of the correct line count for XML validation
	updateLineCount := func(max int) {
		// count lines
		for i := lag; i < max; i++ {
			if record[i] == '\n' {
				lineNum++
			}
		}
		lag = Idx
	}

	// currentLineCount calculates correct line for warning messages, does not update lineNum or lag variables
	currentLineCount := func(max int) int {
		line := lineNum
		for i := lag; i < max; i++ {
			if record[i] == '\n' {
				line++
			}
		}
		return line
	}

	// nextToken returns the type and content fields for the next XML token
	nextToken := func(idx int) (int, int, string, string, int) {

		if record == "" {
			// buffer is empty
			if inp != nil {
				// read next block if available
				record = string(<-inp)
				recLen = len(record)
				Idx = 0
				idx = 0
				lag = 0
			}

			if record == "" {
				// signal end of XML data
				return ISCLOSED, NONE, "", "", 0
			}

			if which != NOTAG && skipTo != "" {
				// previous block ended inside CDATA object or COMMENT
				text := record[:]
				txtlen := recLen
				whch := which
				start := idx
				found := strings.Index(text[idx:], skipTo)
				if found < 0 {
					// no stop signal found in next block
					str := text[start:]
					if HasFlankingSpace(str) {
						str = strings.TrimSpace(str)
					}

					if countLines {
						updateLineCount(txtlen)
					}

					// signal end of current block
					record = ""

					// leave which and skipTo values unchanged as another continuation signal
					// send CDATA or COMMENT contents
					return whch, NONE, str[:], "", 0
				}
				// otherwise adjust position past end of skipTo string and return to normal processing
				idx += found
				str := text[start:idx]
				if HasFlankingSpace(str) {
					str = strings.TrimSpace(str)
				}
				idx += len(skipTo)
				// clear tracking variables
				which = NOTAG
				skipTo = ""
				// send CDATA or COMMENT contents
				return whch, NONE, str[:], "", idx
			}
		}

		text := record[:]
		txtlen := recLen

		// XML string, and all blocks, end with > character, acts as sentinel to check if past end of text
		if idx >= txtlen {
			if inp != nil {

				if countLines {
					updateLineCount(txtlen)
				}

				// signal end of current block, will read next block on next call
				record = ""

				return NOTAG, NONE, "", "", 0
			}

			// signal end of XML string
			return ISCLOSED, NONE, "", "", 0
		}

		ctype := NONE

		// skip past leading blanks
		ch := text[idx]
		if inBlank[ch] {
			ctype |= LFTSPACE
			idx++
			ch = text[idx]
			for inBlank[ch] {
				idx++
				ch = text[idx]
			}
		}

		start := idx

		plainContent := true

		if doStrict && ch == '<' {
			// check to see if an HTML or MathML element is at the beginning of a content string
			if HTMLAhead(text, idx, txtlen) != 0 {
				plainContent = false
			}
		}

		if plainContent && ch == '<' {

			// at start of element
			idx++
			ch = text[idx]

			// check for legal first character of element
			if inFirst[ch] {

				// read element name
				start = idx
				idx++

				ch = text[idx]
				for inElement[ch] {
					idx++
					ch = text[idx]
				}

				str := text[start:idx]

				if ch == '>' {

					// end of element
					idx++

					return STARTTAG, NONE, str[:], "", idx

				} else if ch == '/' {

					// self-closing element without attributes
					idx++
					ch = text[idx]
					if ch != '>' {
						// skip past unexpected blanks
						for inBlank[ch] {
							idx++
							ch = text[idx]
						}
						if ch != '>' {
							DisplayError("Self-closing element missing right angle bracket")
						}
					}
					idx++

					return SELFTAG, NONE, str[:], "", idx

				} else if inBlank[ch] {

					// attributes
					idx++
					ch = text[idx]
					// skip past unexpected blanks
					for inBlank[ch] {
						idx++
						ch = text[idx]
					}
					start = idx
					for ch != '<' && ch != '>' {
						idx++
						ch = text[idx]
					}
					if ch != '>' {
						DisplayError("Attributes not followed by right angle bracket")
					}
					// walk back past trailing blanks
					lst := idx - 1
					ch = text[lst]
					for inBlank[ch] && lst > start {
						lst--
						ch = text[lst]
					}
					if ch == '/' {
						// self-closing
						atr := text[start:lst]
						idx++

						return SELFTAG, NONE, str[:], atr[:], idx
					}
					atr := text[start:idx]
					idx++

					return STARTTAG, NONE, str[:], atr[:], idx

				} else {

					if countLines {
						DisplayError("Unexpected punctuation '%c' in XML element, line %d", ch, currentLineCount(idx))
					} else {
						DisplayError("Unexpected punctuation '%c' in XML element", ch)
					}

					return STARTTAG, NONE, str[:], "", idx
				}

				// other punctuation character immediately after first angle bracket

			} else if ch == '/' {

				// at start of end tag
				idx++
				start = idx
				ch = text[idx]
				// expect legal first character of element
				if inFirst[ch] {
					idx++
					ch = text[idx]
					for inElement[ch] {
						idx++
						ch = text[idx]
					}
					str := text[start:idx]
					if ch != '>' {
						// skip past unexpected blanks
						for inBlank[ch] {
							idx++
							ch = text[idx]
						}
						if ch != '>' {
							DisplayError("Unexpected characters after end element name")
						}
					}
					idx++

					return STOPTAG, NONE, str[:], "", idx
				}
				// legal character not found after slash
				if countLines {
					DisplayError("Unexpected punctuation '%c' in XML element, line %d", ch, currentLineCount(idx))
				} else {
					DisplayError("Unexpected punctuation '%c' in XML element", ch)
				}

			} else if ch == '!' {

				// skip !DOCTYPE, !COMMENT, and ![CDATA[
				idx++
				start = idx
				ch = text[idx]
				which = NOTAG
				skipTo = ""
				if ch == '[' && strings.HasPrefix(text[idx:], "[CDATA[") {
					which = CDATATAG
					skipTo = "]]>"
					start += 7
				} else if ch == '-' && strings.HasPrefix(text[idx:], "--") {
					which = COMMENTTAG
					skipTo = "-->"
					start += 2
				} else if ch == 'D' && strings.HasPrefix(text[idx:], "DOCTYPE") {
					which = DOCTYPETAG
					skipTo = ">"
				}
				if which != NOTAG && skipTo != "" {
					whch := which
					// CDATA or COMMENT block may contain internal angle brackets
					found := strings.Index(text[idx:], skipTo)
					if found < 0 {
						// string stops in middle of CDATA or COMMENT
						if inp != nil {
							str := text[start:]
							if HasFlankingSpace(str) {
								str = strings.TrimSpace(str)
							}

							if countLines {
								updateLineCount(txtlen)
							}

							// signal end of current block
							record = ""

							// leave which and skipTo values unchanged as another continuation signal
							// send CDATA or COMMENT contents
							return whch, NONE, str[:], "", 0
						}

						return ISCLOSED, NONE, "", "", idx
					}
					// adjust position past end of CDATA or COMMENT
					if inp != nil {
						idx += found
						str := text[start:idx]
						if HasFlankingSpace(str) {
							str = strings.TrimSpace(str)
						}
						idx += len(skipTo)
						// clear tracking variables
						which = NOTAG
						skipTo = ""
						// send CDATA or COMMENT contents
						return whch, NONE, str[:], "", idx
					}

					idx += found + len(skipTo)
					return NOTAG, NONE, "", "", idx
				}
				// otherwise just skip to next right angle bracket
				for ch != '>' {
					idx++
					ch = text[idx]
				}
				idx++
				return NOTAG, NONE, "", "", idx

			} else if ch == '?' {

				// skip ?xml and ?processing instructions
				idx++
				ch = text[idx]
				which = PROCESSTAG
				skipTo = "?>"

				if which != NOTAG && skipTo != "" {
					whch := which
					// xml or processing instruction block may contain internal angle brackets
					found := strings.Index(text[idx:], skipTo)
					if found < 0 {
						// string stops in middle of xml or processing instruction
						if inp != nil {
							str := text[start:]
							if HasFlankingSpace(str) {
								str = strings.TrimSpace(str)
							}

							if countLines {
								updateLineCount(txtlen)
							}

							// signal end of current block
							record = ""

							// leave which and skipTo values unchanged as another continuation signal
							// send PROCESSTAG contents
							return whch, NONE, str[:], "", 0
						}

						return ISCLOSED, NONE, "", "", idx
					}
					// adjust position past end of xml or processing instruction
					if inp != nil {
						idx += found
						str := text[start:idx]
						if HasFlankingSpace(str) {
							str = strings.TrimSpace(str)
						}
						idx += len(skipTo)
						// clear tracking variables
						which = NOTAG
						skipTo = ""
						// send PROCESSTAG contents
						return whch, NONE, str[:], "", idx
					}

					idx += found + len(skipTo)
					return NOTAG, NONE, "", "", idx
				}
				// otherwise just skip to next right angle bracket
				for ch != '>' {
					idx++
					ch = text[idx]
				}
				idx++
				return NOTAG, NONE, "", "", idx

			} else {

				if countLines {
					DisplayError("Unexpected punctuation '%c' (%d) in XML element, line %d", ch, ch, currentLineCount(idx))
				} else {
					DisplayError("Unexpected punctuation '%c' (%d) in XML element", ch, ch)
				}
			}

		} else if ch != '>' {

			// at start of contents
			start = idx

			hasMarkup := false
			hasNonASCII := false

			// find end of contents
			if allowEmbed {

				for {
					for inContent[ch] {
						idx++
						ch = text[idx]
					}
					// set flags to speed up conditional content processing
					if ch == '&' {
						idx++
						ch = text[idx]
						if ch == 'a' {
							if strings.HasPrefix(text[idx:], "amp;") {
								hasMarkup = true
							}
						} else if ch == 'g' {
							if strings.HasPrefix(text[idx:], "gt;") {
								hasMarkup = true
							}
						} else if ch == 'l' {
							if strings.HasPrefix(text[idx:], "lt;") {
								hasMarkup = true
							}
						}
						continue
					}
					if ch > 127 {
						hasNonASCII = true
						idx++
						ch = text[idx]
						continue
					}
					if ch == '<' && doStrict {
						// optionally allow HTML text formatting elements and super/subscripts
						advance := HTMLAhead(text, idx, txtlen)
						if advance > 0 {
							idx += advance
							if idx < txtlen {
								ch = text[idx]
							}
							plainContent = false
							continue
						}
					}
					break
				}

			} else {
				for ch != '<' && ch != '>' {
					if ch > 127 {
						hasNonASCII = true
					}
					idx++
					ch = text[idx]
				}
			}

			// trim back past trailing blanks
			lst := idx - 1
			ch = text[lst]
			if inBlank[ch] && lst > start {
				ctype |= RGTSPACE
				lst--
				ch = text[lst]
				for inBlank[ch] && lst > start {
					lst--
					ch = text[lst]
				}
			}

			str := text[start : lst+1]

			if !plainContent {
				ctype |= MIXED
			}
			if hasMarkup {
				ctype |= AMPER
			}
			if hasNonASCII {
				ctype |= ASCII
			}

			return CONTENTTAG, ctype, str[:], "", idx
		}

		return BADTAG, NONE, "", "", idx
	}

	// node farm variables
	farmPos := 0
	farmMax := farmSize
	farmItems := make([]XMLNode, farmMax)

	// nextNode allocates multiple nodes in a large array for memory management efficiency
	nextNode := func(strt, attr, prnt string) *XMLNode {

		// if farm array slots used up, allocate new array
		if farmPos >= farmMax {
			farmItems = make([]XMLNode, farmMax)
			farmPos = 0
		}

		if farmItems == nil {
			return nil
		}

		// take node from next available slot in farm array
		node := &farmItems[farmPos]

		node.Name = strt[:]
		node.Attributes = attr[:]
		node.Parent = prnt[:]

		farmPos++

		return node
	}

	// Parse tokens into tree structure for exploration

	// parseSpecial recursive definition
	var parseSpecial func(string, string, string) (*XMLNode, bool)

	// parseSpecial parses XML tags into tree structure for searching, no contentMods flags set
	parseSpecial = func(strt, attr, prnt string) (*XMLNode, bool) {

		var obj *XMLNode
		ok := true

		// nextNode obtains next node from farm
		node := nextNode(strt, attr, prnt)
		if node == nil {
			return nil, false
		}

		var lastNode *XMLNode

		status := START
		for {
			tag, _, name, attr, idx := nextToken(Idx)
			Idx = idx

			if tag == BADTAG {
				if countLines {
					DisplayError("Unparsable XML element, line %d", lineNum)
				} else {
					DisplayError("Unparsable XML element")
				}
				break
			}
			if tag == ISCLOSED {
				break
			}

			switch tag {
			case STARTTAG:
				if status == CHAR {
					DisplayError("UNEXPECTED MIXED CONTENT <%s> IN <%s>", name, prnt)
				}
				// read sub tree
				obj, ok = parseSpecial(name, attr, node.Name)
				if !ok {
					break
				}

				// adding next child to end of linked list gives better performance than appending to slice of nodes
				if node.Children == nil {
					node.Children = obj
				}
				if lastNode != nil {
					lastNode.Next = obj
				}
				lastNode = obj
				status = STOP
			case STOPTAG:
				// pop out of recursive call
				return node, ok
			case CONTENTTAG:
				node.Contents = name
				status = CHAR
			case SELFTAG:
				if attr == "" && !doSelf {
					// ignore if self-closing tag has no attributes
					continue
				}

				// self-closing tag has no contents, just create child node
				obj = nextNode(name, attr, node.Name)

				if doSelf {
					// add default value for self-closing tag
					obj.Contents = "1"
				}

				if node.Children == nil {
					node.Children = obj
				}
				if lastNode != nil {
					lastNode.Next = obj
				}
				lastNode = obj
				status = OTHER
				// continue on same level
			default:
				status = OTHER
			}
		}

		return node, ok
	}

	// parseLevel recursive definition
	var parseLevel func(string, string, string) (*XMLNode, bool)

	// parseLevel parses XML tags into tree structure for searching, some contentMods flags set
	parseLevel = func(strt, attr, prnt string) (*XMLNode, bool) {

		var obj *XMLNode
		ok := true

		// obtain next node from farm
		node := nextNode(strt, attr, prnt)
		if node == nil {
			return nil, false
		}

		var lastNode *XMLNode

		status := START
		for {
			tag, ctype, name, attr, idx := nextToken(Idx)
			Idx = idx

			if countLines && Idx > 0 {
				updateLineCount(Idx)
			}

			if tag == BADTAG {
				if countLines {
					DisplayError("Unparsable XML element, line %d", lineNum)
				} else {
					DisplayError("Unparsable XML element")
				}
				break
			}
			if tag == ISCLOSED {
				break
			}

			switch tag {
			case STARTTAG:
				if status == CHAR {
					if doStrict {
						DisplayError("UNRECOGNIZED MIXED CONTENT <%s> IN <%s>", name, prnt)
					} else if !doMixed {
						DisplayError("UNEXPECTED MIXED CONTENT <%s> IN <%s>", name, prnt)
					}
				}
				// read sub tree
				obj, ok = parseLevel(name, attr, node.Name)
				if !ok {
					break
				}

				// adding next child to end of linked list gives better performance than appending to slice of nodes
				if node.Children == nil {
					node.Children = obj
				}
				if lastNode != nil {
					lastNode.Next = obj
				}
				lastNode = obj
				status = STOP
			case STOPTAG:
				// pop out of recursive call
				return node, ok
			case CONTENTTAG:
				if doMixed {
					// create unnamed child node for content string
					con := nextNode("", "", "")
					if con == nil {
						break
					}
					str := CleanupContents(name, (ctype&ASCII) != 0, (ctype&AMPER) != 0, (ctype&MIXED) != 0)
					if (ctype & LFTSPACE) != 0 {
						str = " " + str
					}
					if (ctype & RGTSPACE) != 0 {
						str += " "
					}
					con.Contents = str
					if node.Children == nil {
						node.Children = con
					}
					if lastNode != nil {
						lastNode.Next = con
					}
					lastNode = con
				} else {
					node.Contents = CleanupContents(name, (ctype&ASCII) != 0, (ctype&AMPER) != 0, (ctype&MIXED) != 0)
				}
				status = CHAR
			case SELFTAG:
				if attr == "" && !doSelf {
					// ignore if self-closing tag has no attributes
					continue
				}

				// self-closing tag has no contents, just create child node
				obj = nextNode(name, attr, node.Name)

				if doSelf {
					// add default value for self-closing tag
					obj.Contents = "1"
				}

				if node.Children == nil {
					node.Children = obj
				}
				if lastNode != nil {
					lastNode.Next = obj
				}
				lastNode = obj
				status = OTHER
				// continue on same level
			default:
				status = OTHER
			}
		}

		return node, ok
	}

	// parseIndex recursive definition
	var parseIndex func(string, string, string) string

	// parseIndex parses XML tags looking for trie index element
	parseIndex = func(strt, attr, prnt string) string {

		versn := ""

		// check for version attribute match
		if attr != "" && find.Versn != "" && strings.Contains(attr, find.Versn) {
			if strt == find.Match || find.Match == "" {
				if find.Parent == "" || prnt == find.Parent {
					attribs := ParseAttributes(attr)
					for i := 0; i < len(attribs)-1; i += 2 {
						if attribs[i] == find.Versn {
							versn = attribs[i+1]
						}
					}
				}
			}
		}

		// check for attribute index match
		if attr != "" && find.Attrib != "" && strings.Contains(attr, find.Attrib) {
			if strt == find.Match || find.Match == "" {
				if find.Parent == "" || prnt == find.Parent {
					attribs := ParseAttributes(attr)
					for i := 0; i < len(attribs)-1; i += 2 {
						if attribs[i] == find.Attrib {
							return attribs[i+1]
						}
					}
				}
			}
		}

		for {
			tag, _, name, attr, idx := nextToken(Idx)
			Idx = idx

			if tag == BADTAG {
				if countLines {
					DisplayError("Unparsable XML element, line %d", lineNum)
				} else {
					DisplayError("Unparsable XML element")
				}
				break
			}
			if tag == ISCLOSED {
				break
			}

			switch tag {
			case STARTTAG:
				id := parseIndex(name, attr, strt)
				if id != "" {
					return id
				}
			case SELFTAG:
			case STOPTAG:
				// break recursion
				return ""
			case CONTENTTAG:
				// check for content index match
				if strt == find.Match || find.Match == "" {
					if find.Parent == "" || prnt == find.Parent {
						// append version if specified as parent/element@attribute^version
						if versn != "" {
							name += "."
							name += versn
						}
						if ids != nil {
							ids(name)
						} else {
							return name
						}
					}
				}
			default:
			}
		}

		return ""
	}

	// main loops

	// stream all tokens through callback
	if tokens != nil {

		for {
			tag, ctype, name, attr, idx := nextToken(Idx)
			Idx = idx

			if countLines && Idx > 0 {
				updateLineCount(Idx)
			}

			if tag == BADTAG {
				if countLines {
					DisplayError("Unparsable XML element, line %d", lineNum)
				} else {
					DisplayError("Unparsable XML element")
				}
				break
			}

			tkn := XMLToken{tag, ctype, name, attr, idx, lineNum}

			tokens(tkn)

			if tag == ISCLOSED {
				break
			}
		}

		return nil, ""
	}

	// find value of index element
	if find != nil && find.Index != "" {

		// return indexed identifier

		tag, _, name, attr, idx := nextToken(Idx)

		// loop until start tag
		for {
			Idx = idx

			if tag == BADTAG {
				if countLines {
					DisplayError("Unparsable XML element, line %d", lineNum)
				} else {
					DisplayError("Unparsable XML element")
				}
				break
			}
			if tag == ISCLOSED {
				break
			}

			if tag == STARTTAG {
				break
			}

			tag, _, name, attr, idx = nextToken(Idx)
		}

		return nil, parseIndex(name, attr, parent)
	}

	// otherwise create node tree for general data extraction
	tag, _, name, attr, idx := nextToken(Idx)

	// loop until start tag
	for {
		Idx = idx

		if tag == BADTAG {
			if countLines {
				DisplayError("Unparsable XML element, line %d", lineNum)
			} else {
				DisplayError("Unparsable XML element")
			}
			break
		}
		if tag == ISCLOSED {
			break
		}

		if tag == STARTTAG {
			break
		}

		tag, _, name, attr, idx = nextToken(Idx)
	}

	if contentMods {
		// slower parser also handles mixed content
		top, ok := parseLevel(name, attr, parent)

		if !ok {
			return nil, ""
		}

		return top, ""
	}

	// fastest parsing with no contentMods flags
	top, ok := parseSpecial(name, attr, parent)

	if !ok {
		return nil, ""
	}

	return top, ""
}

// SPECIALIZED PUBLIC parseXML SHORTCUTS

// ParseRecord is the main public access to parseXML
func ParseRecord(text, parent string) *XMLNode {

	pat, _ := parseXML(text, parent, nil, nil, nil, nil)

	return pat
}

// FindIdentifier returns a single identifier
func FindIdentifier(text, parent string, find *XMLFind) string {

	_, id := parseXML(text, parent, nil, nil, find, nil)

	return id
}

// FindIdentifiers returns a set of identifiers through a callback
func FindIdentifiers(text, parent string, find *XMLFind, ids func(string)) {

	parseXML(text, parent, nil, nil, find, ids)
}

// StreamTokens streams tokens from a reader through a callback
func StreamTokens(inp <-chan XMLBlock, streamer func(tkn XMLToken)) {

	parseXML("", "", inp, streamer, nil, nil)
}

// StreamValues streams token values from a parsed record through a callback
func StreamValues(text, parent string, stream func(string, string, string)) {

	elementName := ""
	attributeName := ""

	streamer := func(tkn XMLToken) {

		switch tkn.Tag {
		case STARTTAG:
			elementName = tkn.Name
			attributeName = tkn.Attr
		case CONTENTTAG:
			// send element name and content to callback
			stream(elementName, attributeName, tkn.Name)
		default:
		}
	}

	parseXML(text, parent, nil, streamer, nil, nil)
}

// CreateTokenizer streams tokens through a channel
func CreateTokenizer(inp <-chan XMLBlock) <-chan XMLToken {

	if inp == nil {
		return nil
	}

	out := make(chan XMLToken, chanDepth)
	if out == nil {
		DisplayError("Unable to create XML tokenizer channel")
		os.Exit(1)
	}

	// xmlTokenizer sends XML tokens through channel
	xmlTokenizer := func(inp <-chan XMLBlock, out chan<- XMLToken) {

		// close channel when all records have been processed
		defer close(out)

		// parse XML and send tokens through channel
		parseXML("", "", inp, func(tkn XMLToken) { out <- tkn }, nil, nil)
	}

	// launch single tokenizer goroutine
	go xmlTokenizer(inp, out)

	return out
}

// EXPLORE XML ELEMENTS

// ExploreElements returns matching element values to callback
func ExploreElements(curr *XMLNode, mask, prnt, match, attrib string, wildcard, unescape bool, level int, proc func(string, int)) {

	if curr == nil || proc == nil {
		return
	}

	// **/Object performs deep exploration of recursive data (*/Object also supported)
	deep := false
	if prnt == "**" || prnt == "*" {
		prnt = ""
		deep = true
	}

	// exploreChildren recursive definition
	var exploreChildren func(curr *XMLNode, acc func(string))

	// exploreChildren handles mixed-content chains of embedded tags
	exploreChildren = func(curr *XMLNode, acc func(string)) {

		if curr.Contents != "" {
			acc(curr.Contents)
		}
		for chld := curr.Children; chld != nil; chld = chld.Next {
			if chld.Name != "" {
				acc("<" + chld.Name + ">")
			}
			exploreChildren(chld, acc)
			if chld.Name != "" {
				acc("</" + chld.Name + ">")
			}
		}
	}

	// exploreElements recursive definition
	var exploreElements func(curr *XMLNode, skip string, lev int)

	// exploreElements visits nodes looking for matches to requested object
	exploreElements = func(curr *XMLNode, skip string, lev int) {

		if !deep && curr.Name == skip {
			// do not explore within recursive object
			return
		}

		if curr.Name == match ||
			// parent/* matches any subfield
			(match == "*" && prnt != "") ||
			// wildcard (internal colon) matches any namespace prefix
			(wildcard && strings.HasPrefix(match, ":") && strings.HasSuffix(curr.Name, match)) ||
			(match == "" && attrib != "") {

			if prnt == "" ||
				curr.Parent == prnt ||
				(wildcard && strings.HasPrefix(prnt, ":") && strings.HasSuffix(curr.Parent, prnt)) {

				if attrib != "" {
					if curr.Attributes != "" && curr.Attribs == nil {
						// parse attributes on-the-fly if queried
						curr.Attribs = ParseAttributes(curr.Attributes)
					}
					for i := 0; i < len(curr.Attribs)-1; i += 2 {
						// attributes now parsed into array as [ tag, value, tag, value, tag, value, ... ]
						if curr.Attribs[i] == attrib ||
							(wildcard && strings.HasPrefix(attrib, ":") && strings.HasSuffix(curr.Attribs[i], attrib)) {
							proc(curr.Attribs[i+1], level)
							return
						}
					}

				} else if curr.Contents != "" {

					str := curr.Contents[:]

					if unescape && HasAmpOrNotASCII(str) {
						// processing of <, >, &, ", and ' characters is now delayed until element contents is requested
						// unescape converts &lt;b&gt; to <b>
						// also &#181; to Greek mu character, and &#x...; hex values
						str = html.UnescapeString(str)
					}

					proc(str, level)
					return

				} else if curr.Children != nil {

					if doMixed {
						// match with mixed contents - send all child strings
						var buffr strings.Builder
						exploreChildren(curr, func(str string) {
							if str != "" {
								buffr.WriteString(str)
							}
						})
						str := buffr.String()

						// clean up reconstructed mixed content
						str = DoTrimFlankingHTML(str)
						if HasBadSpace(str) {
							str = CleanupBadSpaces(str)
						}
						if HasAdjacentSpaces(str) {
							str = CompressRunsOfSpaces(str)
						}
						if NeedsTightening(str) {
							str = TightenParentheses(str)
						}
						if unescape && HasAmpOrNotASCII(str) {
							str = html.UnescapeString(str)
						}

						proc(str, level)
						return
					}

					// for XML container object, send empty string to callback to increment count
					proc("", level)
					// and continue exploring

				} else if curr.Attributes != "" {

					// for self-closing object, indicate presence by sending empty string to callback
					proc("", level)
					return
				}
			}
		}

		for chld := curr.Children; chld != nil; chld = chld.Next {
			// inner exploration is subject to recursive object exclusion
			exploreElements(chld, mask, lev+1)
		}
	}

	// start recursive exploration from current scope
	exploreElements(curr, "", level)
}

// EXPLORE XML CONTAINERS

// ExploreNodes visits XML container nodes
func ExploreNodes(curr *XMLNode, prnt, match string, index, level int, proc func(*XMLNode, int, int)) {

	if curr == nil || proc == nil {
		return
	}

	// leading colon indicates namespace prefix wildcard
	wildcard := false
	if strings.HasPrefix(prnt, ":") || strings.HasPrefix(match, ":") {
		wildcard = true
	}

	// Single * allows exploration of heterogeneous data construct without knowing current component name
	if prnt == "" && match == "*" {
		match = curr.Name
	}

	// **/Object performs deep exploration of recursive data
	deep := false
	if prnt == "**" {
		prnt = "*"
		deep = true
	}
	// Object/** performs exhaustive exploration of nodes
	tall := false
	if match == "**" {
		match = "*"
		tall = true
	}

	// exploreNodes recursive definition
	var exploreNodes func(*XMLNode, int, int, bool, func(*XMLNode, int, int)) int

	// exploreNodes visits all nodes that match the selection criteria
	exploreNodes = func(curr *XMLNode, indx, levl int, force bool, proc func(*XMLNode, int, int)) int {

		if curr == nil || proc == nil {
			return indx
		}

		// match is "*" for heterogeneous data constructs, e.g., -group PubmedArticleSet/*
		// wildcard matches any namespace prefix
		if curr.Name == match ||
			match == "*" ||
			(wildcard && strings.HasPrefix(match, ":") && strings.HasSuffix(curr.Name, match)) {

			if prnt == "" ||
				curr.Parent == prnt ||
				force ||
				(wildcard && strings.HasPrefix(prnt, ":") && strings.HasSuffix(curr.Parent, prnt)) {

				proc(curr, indx, levl)
				indx++

				if tall && prnt != "" {
					// exhaustive exploration of child nodes within region of parent match
					for chld := curr.Children; chld != nil; chld = chld.Next {
						indx = exploreNodes(chld, indx, levl+1, true, proc)
					}
				}

				if !deep {
					// do not explore within recursive object
					return indx
				}
			}
		}

		// clearing prnt "*" now allows nested exploration within recursive data, e.g., -pattern Taxon -block */Taxon
		if prnt == "*" {
			prnt = ""
		}

		// explore child nodes
		for chld := curr.Children; chld != nil; chld = chld.Next {
			indx = exploreNodes(chld, indx, levl+1, false, proc)
		}

		return indx
	}

	exploreNodes(curr, index, level, false, proc)
}

// EXPLORE SHORTCUTS

// VisitElements is a shortcut to ExploreElements
// Use mask argument to block exploration into recursive object
func VisitElements(curr *XMLNode, match, mask string, proc func(string)) {

	if curr == nil || proc == nil {
		return
	}

	// parse parent/element@attribute construct
	prnt, match := SplitInTwoRight(match, "/")
	match, attrib := SplitInTwoLeft(match, "@")

	ExploreElements(curr, mask, prnt, match, attrib, false, false, 0, func(str string, lvl int) { proc(str) })
}

// VisitNodes is a shortcut to ExploreNodes
func VisitNodes(curr *XMLNode, match string, proc func(*XMLNode)) {

	if curr == nil || proc == nil {
		return
	}

	// parse parent/element@attribute construct
	prnt, match := SplitInTwoRight(match, "/")
	match, _ = SplitInTwoLeft(match, "@")

	ExploreNodes(curr, prnt, match, 0, 0, func(node *XMLNode, idx, lvl int) { proc(node) })
}

// STRING TO XML UTILITY

// StringToXML supports XXXtoXML convenience functions
func StringToXML(rec string, proc func(io.Reader) <-chan string) string {

	if rec == "" || proc == nil {
		return ""
	}

	rdr := strings.NewReader(rec)
	if rdr == nil {
		DisplayError("Unable to create NewReader")
		return ""
	}

	chn := proc(rdr)
	if chn == nil {
		DisplayError("Unable to create StringToXML converter")
		return ""
	}

	var arry []string

	// drain output of channel
	for str := range chn {
		if str == "" {
			continue
		}

		if !strings.HasSuffix(str, "\n") {
			str += "\n"
		}

		arry = append(arry, str)
	}

	res := strings.Join(arry, "")

	return res
}
