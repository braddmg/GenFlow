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
// File Name:  chan.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"io"
	"os"
	"strings"
)

// stringChanReader connects a string output channel to an io.Reader interface
type stringChanReader struct {
	c <-chan string
	s string
}

func (r *stringChanReader) Read(b []byte) (n int, err error) {

	if r.s != "" {
		n = copy(b, []byte(r.s))
		r.s = r.s[n:]
		return
	}

	for str := range r.c {
		r.s = str
		n = copy(b, []byte(r.s))
		r.s = r.s[n:]
		return
	}

	return 0, io.EOF
}

// ChanToReader converts a string channel to an ioReader
func ChanToReader(inp <-chan string) io.Reader {

	if inp == nil {
		return nil
	}

	return &stringChanReader{c: inp}
}

// ChanToStdout sends a string channel to stdout
func ChanToStdout(inp <-chan string) {

	if inp == nil {
		return
	}

	last := ""

	for str := range inp {
		last = str
		os.Stdout.WriteString(str)
	}

	if !strings.HasSuffix(last, "\n") {
		os.Stdout.WriteString("\n")
	}
}

// ChanToString converts a string channel to a string
func ChanToString(inp <-chan string) string {

	if inp == nil {
		return ""
	}

	var buffer strings.Builder

	last := ""

	for str := range inp {
		last = str
		buffer.WriteString(str)
	}

	if !strings.HasSuffix(last, "\n") {
		buffer.WriteString("\n")
	}

	return buffer.String()
}

// ChanToChan buffers a string channel
func ChanToChan(inp <-chan string) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		return nil
	}

	bufferChannel := func(inp <-chan string, out chan<- string) {

		// close channel when all records have been processed
		defer close(out)

		var arry []string

		// drain input channel
		for str := range inp {
			arry = append(arry, str)
		}

		// retransmit buffered strings
		for _, str := range arry {
			out <- str
		}
	}

	// launch single buffering goroutine
	go bufferChannel(inp, out)

	return out
}

// StringToChan sends a single string down a string channel
func StringToChan(str string) <-chan string {

	if str == "" {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		return nil
	}

	sendString := func(str string, out chan<- string) {

		// close channel when all records have been processed
		defer close(out)

		out <- str
	}

	// launch single buffering goroutine
	go sendString(str, out)

	return out
}

// SliceToChan sends slice elements down a string channel
func SliceToChan(values []string) <-chan string {

	if values == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		return nil
	}

	sendStrings := func(values []string, out chan<- string) {

		// close channel when all records have been processed
		defer close(out)

		for _, str := range values {
			out <- str
		}
	}

	// launch single buffering goroutine
	go sendStrings(values, out)

	return out
}
