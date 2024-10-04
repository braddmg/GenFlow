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
// File Name:  yaml.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"bufio"
	"github.com/goccy/go-yaml"
	"io"
	"os"
	"runtime"
	"strings"
)

// YAMLConverter parses YAML files through JSON into XML object stream
func YAMLConverter(inp io.Reader) <-chan string {

	if inp == nil {
		return nil
	}

	out := make(chan string, chanDepth)
	if out == nil {
		DisplayError("Unable to create YAML converter channel")
		os.Exit(1)
	}

	convertYAML := func(inp io.Reader, out chan<- string) {

		// close channel when all records have been sent
		defer close(out)

		var buffer strings.Builder

		scanr := bufio.NewScanner(inp)

		for scanr.Scan() {

			line := scanr.Text()
			buffer.WriteString(line + "\n")
		}

		txt := buffer.String()

		buffer.Reset()

		if txt == "" {
			return
		}

		inputBytes := []byte(txt)
		buff, err := yaml.YAMLToJSON(inputBytes)
		if err != nil {
			DisplayError("YAMLToJSON error '%v'", err)
			os.Exit(1)
		}

		jsn := string(buff)

		if jsn == "" {
			return
		}

		jcnv := JSONConverter(strings.NewReader(jsn), "", "ConfigFile", "element")

		if jcnv == nil {
			DisplayError("Unable to create YAML to JSON to XML converter")
			os.Exit(1)
		}

		for str := range jcnv {
			if str != "" {
				out <- str
			}
		}

		runtime.Gosched()
	}

	go convertYAML(inp, out)

	return out
}

// YAMLtoXML converts YAML to an XML string
func YAMLtoXML(yml string) string {

	return StringToXML(yml, YAMLConverter)
}
