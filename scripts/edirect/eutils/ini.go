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
// File Name:  ini.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"os"
	"runtime"
	"strings"
)

// INIConverter parses INI files into XML object stream
func INIConverter(inp io.Reader) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create INI converter channel")
		os.Exit(1)
	}

	convertINI := func(inp io.Reader, out chan<- string) {

		// close channel when all records have been sent
		defer close(out)

		okay := false
		row := 0

		var buffer strings.Builder

		scanr := bufio.NewScanner(inp)

		buffer.WriteString("<ConfigFile>\n")

		var currSect []string

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
				buffer.WriteString("                    ")
				i -= 10
			}
			if i < 0 {
				return
			}
			buffer.WriteString(indentSpaces[i])
		}

		indent := 0

		changeSection := func(sect string, curr []string) ([]string, int) {

			splt := strings.Split(sect, ".")

			slen, clen := len(splt), len(curr)
			mlen := min(slen, clen)

			// find last common subsection
			i := 0
			for range mlen {
				if splt[i] != curr[i] {
					break
				}
				i++
			}

			// descend down to common point, closing old levels
			for j := clen - 1; j >= i; j-- {
				if curr[j] == "" {
					continue
				}
				doIndent(j)
				buffer.WriteString("  </" + curr[j] + ">\n")
			}

			// add new levels from common point
			k := 0
			for k = i; k < slen; k++ {
				if splt[k] == "" {
					continue
				}
				doIndent(k)
				buffer.WriteString("  <" + splt[k] + ">\n")
			}

			return splt, k
		}

		for scanr.Scan() {

			line := scanr.Text()

			row++

			line = strings.TrimSpace(line)

			// ignore comment lines
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}

			if strings.HasPrefix(line, "[") {
				if !strings.HasSuffix(line, "]") {
					fmt.Fprintf(os.Stderr, "Improper Section '%s'\n", line)
					continue
				}

				line = strings.TrimPrefix(line, "[")
				line = strings.TrimSuffix(line, "]")
				line = strings.TrimSpace(line)

				// check for relative section
				if strings.HasPrefix(line, ".") {
					if len(currSect) < 1 || len(line) < 2 {
						fmt.Fprintf(os.Stderr, "Improper Relative Section '%s'\n", line)
						continue
					}
					line = strings.Join(currSect, ".") + line
				}

				currSect, indent = changeSection(line, currSect)

				continue
			}

			lft, rgt, found := strings.Cut(line, "=")
			if !found || lft == "" || rgt == "" {
				fmt.Fprintf(os.Stderr, "Improper Item '%s'\n", line)
				continue
			}
			lft = strings.TrimSpace(lft)
			rgt = strings.TrimSpace(rgt)
			rgt = strings.TrimPrefix(rgt, "\"")
			rgt = strings.TrimSuffix(rgt, "\"")
			rgt = strings.TrimSpace(rgt)
			rgt = html.EscapeString(rgt)

			doIndent(indent)
			buffer.WriteString("  <" + lft + ">" + rgt + "</" + lft + ">\n")

			okay = true
		}

		currSect, indent = changeSection("", currSect)

		buffer.WriteString("</ConfigFile>\n")

		if okay {
			txt := buffer.String()
			if txt != "" {
				// send remaining result through output channel
				out <- txt
			}
		}

		buffer.Reset()

		runtime.Gosched()
	}

	go convertINI(inp, out)

	return out
}

// INItoXML converts INI to an XML string
func INItoXML(ini string) string {

	return StringToXML(ini, INIConverter)
}
