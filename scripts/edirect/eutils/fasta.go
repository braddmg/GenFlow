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
// File Name:  fasta.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// FASTARecord contains parsed data from a FASTA format record
type FASTARecord struct {
	SeqID    string
	Title    string
	Length   int
	Sequence string
}

// FASTAConverter reads a concatenated FASTA stream and sends individual records down a channel
func FASTAConverter(inp io.Reader, caseSensitive bool) <-chan FASTARecord {

	if inp == nil {
		return nil
	}

	// internal channel sends either definition line or combined sequence lines
	lns := make(chan string, chanDepth)

	// second channel send assembled FASTA records to the caller one at a time
	out := make(chan FASTARecord, chanDepth)

	if lns == nil || out == nil {
		DisplayWarning("Unable to create FASTA converter channels")
		os.Exit(1)
	}

	// fastaSplitter partitions FASTA input stream into a full definition line or a sequence segment
	fastaSplitter := func(inp io.Reader, lns chan<- string) {

		// close channel when all strings have been sent
		defer close(lns)

		const FASTABUFSIZE = 65536

		buffer := make([]byte, FASTABUFSIZE)
		isClosed := false

		// read the next large block of characters from input stream
		nextBuffer := func() string {

			if isClosed {
				return ""
			}

			n, err := inp.Read(buffer[:])

			if err != nil {
				if err != io.EOF {
					// real error
					DisplayWarning("%s", err.Error())

					// ignore bytes - non-conforming implementations of io.Reader may
					// return mangled data on non-EOF errors
					isClosed = true

					return ""
				}

				// end of file
				isClosed = true

				if n == 0 {
					// if EOF and no more data
					return ""
				}
			}

			if n < 0 {
				// reality check - non-conforming implementations of io.Reader may return -1
				DisplayWarning("io.Reader returned negative count %d", n)

				// treat as n == 0
				return ""
			}

			// slice of actual characters read
			bufr := buffer[:n]

			return string(bufr[:])
		}

		line := ""

		for {

			if line == "" {
				line = nextBuffer()
				if line == "" {
					break
				}
			}

			// look for start of FASTA defline
			pos := strings.Index(line, ">")

			if pos < 0 {
				// no angle bracket in buffer, send remainder as sequence
				lns <- line
				line = ""
				continue
			}

			if pos > 0 {
				// send sequence buffer up to angle bracket
				str := line[:pos]
				lns <- str
				// shrink buffer to start at angle bracket
				line = line[pos:]
				continue
			}

			// look for end of FASTA defline
			pos = strings.Index(line, "\n")

			if pos > 0 {
				// send full defline within buffer
				str := line[:pos]
				lns <- str
				// shrink buffer to start after defline and newline character
				line = line[pos+1:]
				continue
			}

			// defline continues into next buffer, look for next newline
			defln := line

			for {

				// read next buffer
				line = nextBuffer()
				if line == "" {
					// file ends in defline
					lns <- defln
					break
				}

				pos = strings.Index(line, "\n")

				if pos < 0 {
					// add full buffer to defline
					defln += line
					// continue with next buffer
					continue
				}

				// found newline, send constructed defline
				defln += line[:pos]
				lns <- defln
				line = line[pos+1:]
				break
			}
		}
	}

	// fastaStreamer sends FASTA records down a channel
	fastaStreamer := func(lns <-chan string, out chan<- FASTARecord) {

		// close channel when all records have been processed
		defer close(out)

		seqid := ""
		title := ""

		var fasta []string

		sendFasta := func() {

			seq := strings.Join(fasta, "")
			seqlen := len(seq)

			if seqlen > 0 {
				out <- FASTARecord{SeqID: seqid, Title: title, Length: seqlen, Sequence: seq[:]}
			}

			seqid = ""
			title = ""

			// reset sequence accumulator
			fasta = nil
		}

		for line := range lns {

			if strings.HasPrefix(line, ">") {

				// send current record, clear sequence buffer
				sendFasta()

				// parse next defline
				line = line[1:]
				seqid, title = SplitInTwoLeft(line, " ")

				continue
			}

			if !caseSensitive {
				// optionally convert FASTA letters to upper case
				line = strings.ToUpper(line)
			}

			// leave only letters, asterisk, or hyphen
			line = strings.Map(func(c rune) rune {
				if c >= 'A' && c <= 'Z' {
					return c
				}
				if c >= 'a' && c <= 'z' {
					return c
				}
				if c == '*' || c == '-' {
					return c
				}
				return -1
			}, line)

			// append current line
			fasta = append(fasta, line)
		}

		// send final record
		sendFasta()
	}

	// launch single fasta splitter goroutine
	go fastaSplitter(inp, lns)

	// launch single fasta streamer goroutine
	go fastaStreamer(lns, out)

	return out
}

// FormatFASTA sends lines of specified width to Stdout
func FormatFASTA(inp io.Reader, width int, requireHeader, caseSensitive bool) {

	if inp == nil {
		return
	}

	fcnv := FASTAConverter(inp, caseSensitive)
	if fcnv == nil {
		DisplayError("Unable to create FASTA converter")
		return
	}

	if width < 1 || width > 100 {
		// default width is 70 characters per line
		width = 70
	}

	var buffer strings.Builder
	count := 0
	recordCount := 0

	wrtr := bufio.NewWriter(os.Stdout)

	for fsa := range fcnv {

		seqid, title, sequence := fsa.SeqID, fsa.Title, fsa.Sequence

		recordCount++

		if seqid != "" || title != "" || requireHeader {
			sep := ""
			buffer.WriteString(">")

			if seqid == "" {
				seqid = fmt.Sprintf("lcl|%d", recordCount)
			}
			if seqid != "" {
				buffer.WriteString(seqid)
				sep = " "
			}
			if title != "" {
				buffer.WriteString(sep)
				buffer.WriteString(title)
			}

			buffer.WriteString("\n")
		}

		for sequence != "" {

			mx := len(sequence)
			if mx > width {
				mx = width
			}
			item := sequence[:mx]
			sequence = sequence[mx:]
			item = strings.ToUpper(item)
			buffer.WriteString(item)
			buffer.WriteString("\n")

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
	}

	txt := buffer.String()
	if txt != "" {
		// print last buffer
		wrtr.WriteString(txt[:])
	}
	buffer.Reset()

	wrtr.Flush()
}

// FASTAtoXML converts a FASTA flatfile to an XML string
func FASTAtoXML(fasta string, caseSensitive bool) string {

	if fasta == "" {
		return ""
	}

	fcnv := FASTAConverter(strings.NewReader(fasta), caseSensitive)
	if fcnv == nil {
		DisplayError("Unable to create FASTA converter")
		return ""
	}

	var buffer strings.Builder

	for fsa := range fcnv {

		buffer.WriteString("<FASTA>\n")
		if fsa.SeqID != "" {
			buffer.WriteString("  <ID>")
			buffer.WriteString(fsa.SeqID)
			buffer.WriteString("</ID>\n")
		}
		if fsa.Title != "" {
			buffer.WriteString("  <Title>")
			buffer.WriteString(fsa.Title)
			buffer.WriteString("</Title>\n")
		}
		if fsa.Length != 0 {
			buffer.WriteString("  <Length>")
			buffer.WriteString(strconv.Itoa(fsa.Length))
			buffer.WriteString("</Length>\n")
		}
		if fsa.Sequence != "" {
			buffer.WriteString("  <Seq>")
			buffer.WriteString(fsa.Sequence)
			buffer.WriteString("</Seq>\n")
		}
		buffer.WriteString("</FASTA>\n")
	}

	res := buffer.String()

	buffer.Reset()

	return res
}
