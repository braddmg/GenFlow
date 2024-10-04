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
// File Name:  xtract.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package main

import (
	"bufio"
	"cmp"
	"eutils"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"time"
)

// GLOBAL VARIABLES

var (
	doStem bool
	deStop bool
)

/*
// UTILITIES

func parseMarkup(str, cmd string) int {

	switch str {
	case "fuse", "fused":
		return eutils.FUSE
	case "space", "spaces":
		return eutils.SPACE
	case "period", "periods":
		return eutils.PERIOD
	case "concise":
		return eutils.CONCISE
	case "bracket", "brackets":
		return eutils.BRACKETS
	case "markdown":
		return eutils.MARKDOWN
	case "slash":
		return eutils.SLASH
	case "tag", "tags":
		return eutils.TAGS
	case "terse":
		return eutils.TERSE
	default:
		if str != "" {
			eutils.DisplayError("Unrecognized %s value '%s'", cmd, str)
			os.Exit(1)
		}
	}
	return eutils.NOMARKUP
}
*/

// MAIN FUNCTION

// e.g., xtract -pattern PubmedArticle -element MedlineCitation/PMID -block Author -sep " " -element Initials,LastName

func main() {

	// skip past executable name
	args := os.Args[1:]

	if len(args) < 1 {
		eutils.DisplayError("No command-line arguments supplied to xtract")
		os.Exit(1)
	}

	// pretty-printing of xtract arguments
	if args[0] == "-pretty" {
		eutils.PrettyArguments(args[1:])
		return
	}

	// performance arguments
	chanDepth := 0
	farmSize := 0
	heapSize := 0
	numServe := 0
	goGc := 0

	// processing option arguments
	doCompress := false
	doCleanup := false
	doStrict := false
	doMixed := false
	doSelf := false
	deAccent := false
	deSymbol := false
	doASCII := false
	doStem = false
	deStop = true

	/*
		doUnicode := false
		doScript := false
		doMathML := false
	*/

	// CONCURRENCY, CLEANUP, AND DEBUGGING FLAGS

	// do these first because -defcpu and -maxcpu can be sent from wrapper before other arguments

	ncpu := runtime.NumCPU()
	if ncpu < 1 {
		ncpu = 1
	}

	// wrapper can limit maximum number of processors to use (undocumented)
	maxProcs := ncpu
	defProcs := 0

	// concurrent performance tuning parameters, can be overridden by -proc and -cons
	numProcs := 0
	serverRatio := 4

	// -flag sets -strict or -mixed cleanup flags from argument
	flgs := ""

	/*
		unicodePolicy := ""
		scriptPolicy := ""
		mathmlPolicy := ""
	*/

	// read data from file instead of stdin
	fileName := ""

	// flag for indexed input file
	turbo := false

	// debugging
	mpty := false
	idnt := false
	stts := false
	timr := false

	// profiling
	prfl := false

	// repeat the specified extraction 5 times for each -proc from 1 to nCPU
	trial := false

	inSwitch := true

	// get concurrency, cleanup, and debugging flags in any order
	for {

		inSwitch = true

		switch args[0] {
		// concurrency override arguments can be passed in by local wrapper script (undocumented)
		case "-maxcpu":
			maxProcs = eutils.GetNumericArg(args, "Maximum number of processors", 1, 1, ncpu)
			args = args[1:]
		case "-defcpu":
			defProcs = eutils.GetNumericArg(args, "Default number of processors", ncpu, 1, ncpu)
			args = args[1:]
		// performance tuning flags
		case "-proc":
			numProcs = eutils.GetNumericArg(args, "Number of processors", ncpu, 1, ncpu)
			args = args[1:]
		case "-cons":
			serverRatio = eutils.GetNumericArg(args, "Parser to processor ratio", 4, 1, 32)
			args = args[1:]
		case "-serv":
			numServe = eutils.GetNumericArg(args, "Concurrent parser count", 0, 1, 128)
			args = args[1:]
		case "-chan":
			chanDepth = eutils.GetNumericArg(args, "Communication channel depth", 0, ncpu, 128)
			args = args[1:]
		case "-heap":
			heapSize = eutils.GetNumericArg(args, "Unshuffler heap size", 8, 8, 64)
			args = args[1:]
		case "-farm":
			farmSize = eutils.GetNumericArg(args, "Node buffer length", 4, 4, 2048)
			args = args[1:]
		case "-gogc":
			goGc = eutils.GetNumericArg(args, "Garbage collection percentage", 0, 50, 1000)
			args = args[1:]

		// read data from file
		case "-input":
			fileName = eutils.GetStringArg(args, "Input file name")
			args = args[1:]

		// input is indexed with <NEXT_RECORD_SIZE> objects
		case "-turbo":
			turbo = true

		// data cleanup flags
		case "-compress", "-compressed":
			doCompress = true
		case "-spaces", "-cleanup":
			doCleanup = true
		case "-strict":
			doStrict = true
		case "-mixed":
			doMixed = true
		case "-self":
			doSelf = true
		case "-accent":
			deAccent = true
		case "-symbol":
			deSymbol = true
		case "-ascii":
			doASCII = true

		// previously visible processing flags (undocumented)
		case "-stems", "-stem":
			doStem = true
		case "-stops", "-stop":
			deStop = false

		// allow setting of unicode, script, and mathml flags (undocumented)
		case "-unicode":
			// unicodePolicy = GetStringArg(args, "Unicode argument")
			args = args[1:]
		case "-script":
			// scriptPolicy = GetStringArg(args, "Script argument")
			args = args[1:]
		case "-mathml":
			// mathmlPolicy = GetStringArg(args, "MathML argument")
			args = args[1:]

		case "-flag", "-flags":
			flgs = eutils.GetStringArg(args, "Flags argument")
			args = args[1:]

		// debugging flags
		case "-debug":
			// dbug = true
		case "-empty":
			mpty = true
		case "-ident":
			idnt = true
		case "-stats", "-stat":
			stts = true
		case "-timer":
			timr = true
		case "-profile":
			prfl = true
		case "-trial", "-trials":
			trial = true

		default:
			// if not any of the controls, set flag to break out of for loop
			inSwitch = false
		}

		if !inSwitch {
			break
		}

		// skip past argument
		args = args[1:]

		if len(args) < 1 {
			break
		}
	}

	// -flag allows script to set -strict or -mixed (or -stems, or -stops) from argument
	switch flgs {
	case "strict":
		doStrict = true
	case "mixed":
		doMixed = true
	case "stems", "stem":
		doStem = true
	case "stops", "stop":
		deStop = false
	case "none", "default":
	default:
		if flgs != "" {
			eutils.DisplayError("Unrecognized -flag value '%s'", flgs)
			os.Exit(1)
		}
	}

	/*
		UnicodeFix = parseMarkup(unicodePolicy, "-unicode")
		ScriptFix = parseMarkup(scriptPolicy, "-script")
		MathMLFix = parseMarkup(mathmlPolicy, "-mathml")

		if UnicodeFix != NOMARKUP {
			doUnicode = true
		}

		if ScriptFix != NOMARKUP {
			doScript = true
		}

		if MathMLFix != NOMARKUP {
			doMathML = true
		}
	*/

	if numProcs == 0 {
		if defProcs > 0 {
			numProcs = defProcs
		} else if maxProcs > 0 {
			numProcs = maxProcs
		}
	}
	if numProcs > ncpu {
		numProcs = ncpu
	}
	if numProcs > maxProcs {
		numProcs = maxProcs
	}

	eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, turbo)

	eutils.SetOptions(doStrict, doMixed, doSelf, deAccent, deSymbol, doASCII, doCompress, doCleanup, doStem, deStop)

	// -stats prints number of CPUs and performance tuning values if no other arguments (undocumented)
	if stts && len(args) < 1 {

		eutils.PrintStats()

		return
	}

	if len(args) < 1 {
		eutils.DisplayError("Insufficient command-line arguments supplied to xtract")
		os.Exit(1)
	}

	// DOCUMENTATION COMMANDS

	inSwitch = true

	switch args[0] {
	case "-version":
		fmt.Printf("%s\n", eutils.EDirectVersion)
	case "-help", "help", "--help":
		eutils.PrintHelp("xtract", "xtract-help.txt")
	case "-examples", "-example":
		eutils.PrintHelp("xtract", "xtract-examples.txt")
	case "-extras", "-extra", "-advanced":
		fmt.Printf("Please run rchive -help for local record indexing information\n")
	case "-internal", "-internals":
		eutils.PrintHelp("xtract", "xtract-internal.txt")
	case "-keys":
		eutils.PrintHelp("xtract", "xtract-keys.txt")
	case "-unix":
		eutils.PrintHelp("xtract", "xtract-unix.txt")
	default:
		// if not any of the documentation commands, keep going
		inSwitch = false
	}

	if inSwitch {
		return
	}

	// FILE NAME CAN BE SUPPLIED WITH -input COMMAND

	in := os.Stdin

	// check for data being piped into stdin
	isPipe := false
	fi, err := os.Stdin.Stat()
	if err == nil {
		isPipe = bool((fi.Mode() & os.ModeNamedPipe) != 0)
	}

	usingFile := false

	if fileName != "" {

		inFile, err := os.Open(fileName)
		if err != nil {
			eutils.DisplayError("Unable to open input file '%s'", fileName)
			os.Exit(1)
		}

		defer inFile.Close()

		// use indicated file instead of stdin
		in = inFile
		usingFile = true

		if isPipe && runtime.GOOS != "windows" {
			mode := fi.Mode().String()
			eutils.DisplayError("Input data from both stdin and file '%s', mode is '%s'", fileName, mode)
			os.Exit(1)
		}
	}

	// check for -input command after extraction arguments
	for _, str := range args {
		if str == "-input" {
			eutils.DisplayError("Misplaced -input command")
			os.Exit(1)
		}
	}

	// START PROFILING IF REQUESTED

	if prfl {

		f, err := os.Create("cpu.pprof")
		if err != nil {
			eutils.DisplayError("Unable to create profile output file")
			os.Exit(1)
		}

		pprof.StartCPUProfile(f)

		defer pprof.StopCPUProfile()
	}

	// INITIALIZE RECORD COUNT

	recordCount := 0
	byteCount := 0

	// print processing rate and program duration
	printDuration := func(name string) {

		eutils.PrintDuration(name, recordCount, byteCount)
	}

	// NAME OF OUTPUT STRING TRANSFORMATION FILE

	tform := ""
	transform := make(map[string]string)

	populateTx := func(tf string, special bool) {

		inFile, err := os.Open(tf)
		if err != nil {
			eutils.DisplayError("Unable to open transformation file %s", err.Error())
			os.Exit(1)
		}
		defer inFile.Close()

		scanr := bufio.NewScanner(inFile)

		// populate transformation map for -translate, -keywords, and -matrix output
		for scanr.Scan() {

			line := scanr.Text()

			if special && strings.HasPrefix(line, "#") {
				continue
			}

			frst, scnd := eutils.SplitInTwoLeft(line, "\t")

			if special && scnd == "-" {
				delete(transform, frst)
			} else {
				transform[frst] = scnd
			}
		}
	}

	forClassify := false

	if len(args) > 2 {
		if args[0] == "-transform" || args[0] == "-aliases" || args[0] == "-transfigure" {
			special := false
			if args[0] == "-aliases" {
				forClassify = true
			} else if args[0] == "-transfigure" {
				special = true
			}
			tform = args[1]
			args = args[2:]
			if tform != "" {
				populateTx(tform, special)
			}
		}
	}

	// SEQUENCE RECORD EXTRACTION COMMAND GENERATOR

	// -insd simplifies extraction of INSDSeq qualifiers
	if args[0] == "-insd" || args[0] == "-insd-" || args[0] == "-insd-idx" || args[0] == "-insd-xml" {

		addDash := true
		doIndex := false
		makeXML := false

		// -insd- variant suppresses use of dash as placeholder for missing qualifiers (undocumented)
		if args[0] == "-insd-" {
			addDash = false
		}
		// -insd-idx variant creates word index using the -wrp TIAB -indexer command (undocumented)
		if args[0] == "-insd-idx" {
			doIndex = true
			addDash = false
		}
		// -insd-xml variant used for -insdx and subsequent conversion from table to XML (undocumented)
		if args[0] == "-insd-xml" {
			makeXML = true
		}

		args = args[1:]

		insd := eutils.ProcessINSD(args, isPipe || usingFile, addDash, doIndex, makeXML)

		if !isPipe && !usingFile {
			// no piped input, so write output instructions
			fmt.Printf("xtract")
			for _, str := range insd {
				fmt.Printf(" %s", str)
			}
			fmt.Printf("\n")
			return
		}

		// data in pipe, so replace arguments, execute dynamically
		args = insd
	}

	// BIOTHINGS EXTRACTION COMMAND GENERATOR

	// -biopath takes a parent object and a dotted exploration path for BioThings resources (undocumented)
	if args[0] == "-biopath" {

		args = args[1:]

		biopath := eutils.ProcessBiopath(args, isPipe || usingFile)

		if !isPipe && !usingFile {
			// no piped input, so write output instructions
			fmt.Printf("xtract")
			for _, str := range biopath {
				fmt.Printf(" %s", str)
			}
			fmt.Printf("\n")
			return
		}

		// data in pipe, so replace arguments, execute dynamically
		args = biopath
	}

	// SPECIFY STRINGS TO GO BEFORE AND AFTER ENTIRE OUTPUT OR EACH RECORD

	head := ""
	tail := ""

	hd := ""
	tl := ""

	for {

		inSwitch = true

		switch args[0] {
		case "-head":
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -head command")
				os.Exit(1)
			}
			head = eutils.ConvertSlash(args[1])
			// allow splitting of -head argument, keep appending until next command (undocumented)
			ofs, nxt := 0, args[2:]
			for {
				if len(nxt) < 1 {
					break
				}
				tmp := nxt[0]
				if strings.HasPrefix(tmp, "-") {
					break
				}
				ofs++
				txt := eutils.ConvertSlash(tmp)
				if head != "" && !strings.HasSuffix(head, "\t") {
					head += "\t"
				}
				head += txt
				nxt = nxt[1:]
			}
			if ofs > 0 {
				args = args[ofs:]
			}
		case "-tail":
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -tail command")
				os.Exit(1)
			}
			tail = eutils.ConvertSlash(args[1])
		case "-hd":
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -hd command")
				os.Exit(1)
			}
			hd = eutils.ConvertSlash(args[1])
		case "-tl":
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -tl command")
				os.Exit(1)
			}
			tl = eutils.ConvertSlash(args[1])
		case "-wrp":
			// shortcut to wrap records in XML tags
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -wrp command")
				os.Exit(1)
			}
			tmp := eutils.ConvertSlash(args[1])
			lft, rgt := eutils.SplitInTwoLeft(tmp, ",")
			if lft != "" {
				head = "<" + lft + ">"
				tail = "</" + lft + ">"
			}
			if rgt != "" {
				hd = "<" + rgt + ">"
				tl = "</" + rgt + ">"
			}
		case "-set":
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -set command")
				os.Exit(1)
			}
			tmp := eutils.ConvertSlash(args[1])
			if tmp != "" {
				head = "<" + tmp + ">"
				tail = "</" + tmp + ">"
			}
		case "-rec":
			if len(args) < 2 {
				eutils.DisplayError("Pattern missing after -rec command")
				os.Exit(1)
			}
			tmp := eutils.ConvertSlash(args[1])
			if tmp != "" {
				hd = "<" + tmp + ">"
				tl = "</" + tmp + ">"
			}
		default:
			// if not any of the controls, set flag to break out of for loop
			inSwitch = false
		}

		if !inSwitch {
			break
		}

		// skip past arguments
		args = args[2:]

		if len(args) < 1 {
			eutils.DisplayError("Insufficient command-line arguments supplied to xtract")
			os.Exit(1)
		}
	}

	// CREATE XML BLOCK READER FROM STDIN OR FILE

	const FirstBuffSize = 4096

	getFirstBlock := func() string {

		buffer := make([]byte, FirstBuffSize)
		n, err := in.Read(buffer)
		if err != nil && err != io.EOF {
			eutils.DisplayError("Unable to read first block: %s", err.Error())
			// os.Exit(1)
		}
		bufr := buffer[:n]
		return string(bufr)
	}

	first := getFirstBlock()

	// trim leading byte order mark, if present
	first = eutils.TrimBOM(first)

	mlt := io.MultiReader(strings.NewReader(first), in)

	isJsn := false
	isAsn := false
	isGbf := false
	matched := 0

	// auto-detect XML, JSON, or ASN.1 format
	if first != "" {
		posJ1 := strings.Index(first, "{")
		posJ2 := strings.Index(first, "\":")
		posJ3 := strings.Index(first, "\" :")
		if posJ1 >= 0 && posJ2 >= 0 && posJ1 < posJ2 {
			isJsn = true
			matched++
		} else if posJ1 >= 0 && posJ3 >= 0 && posJ1 < posJ3 {
			isJsn = true
			matched++
		} else {
			posJ1 = FirstBuffSize
			posJ2 = FirstBuffSize
		}
		posA1 := strings.Index(first, "::=")
		posA2 := strings.Index(first, "{")
		if posA1 >= 0 && posA2 >= 0 && posA1 < posA2 {
			isAsn = true
			matched++
		} else {
			posA1 = FirstBuffSize
			posA2 = FirstBuffSize
		}
		posG1 := strings.Index(first, "LOCUS")
		posG2 := strings.Index(first, "DEFINITION")
		if posG1 >= 0 && posG2 >= 0 && posG1 < posG2 {
			isGbf = true
			matched++
		} else {
			posG1 = FirstBuffSize
			posG2 = FirstBuffSize
		}
		posX1 := strings.Index(first, "<")
		posX2 := strings.Index(first, ">")
		if posX1 >= 0 && posX2 >= 0 && posX1 < posX2 {
			matched++
		} else {
			posX1 = FirstBuffSize
			posX2 = FirstBuffSize
		}
		if matched > 1 {
			if posX1 < posJ1 && posX1 < posA1 && posX1 < posG1 {
				isJsn = false
				isAsn = false
				isGbf = false
			} else if posJ1 < posA1 && posJ1 < posG1 {
				isAsn = false
				isGbf = false
			} else if posA1 < posJ1 && posA1 < posG1 {
				isJsn = false
				isGbf = false
			} else if posG1 < posJ1 && posG1 < posA1 {
				isJsn = false
				isAsn = false
			}
		}
	}

	var xmlChan <-chan string

	if isJsn {
		xmlChan = eutils.JSONConverter(mlt, "root", "", "element")
		mlt = nil
	} else if isAsn {
		xmlChan = eutils.ASN1Converter(mlt, "", "")
		mlt = nil
	} else if isGbf {
		xmlChan = eutils.GenBankConverter(mlt)
		mlt = nil
	}

	rdr := eutils.CreateXMLStreamer(mlt, xmlChan)
	if rdr == nil {
		eutils.DisplayError("Unable to create XML Block Reader")
		os.Exit(1)
	}

	// CONFIRM INPUT DATA AVAILABILITY AFTER RUNNING COMMAND GENERATORS

	if fileName == "" && runtime.GOOS != "windows" {

		fromStdin := bool((fi.Mode() & os.ModeCharDevice) == 0)
		if !isPipe || !fromStdin {
			mode := fi.Mode().String()
			eutils.DisplayError("No data supplied to xtract from stdin or file, mode is '%s'", mode)
			os.Exit(1)
		}
	}

	if !usingFile && !isPipe {

		eutils.DisplayError("No XML input data supplied to xtract")
		os.Exit(1)
	}

	// XML VALIDATION

	nextArg := func() (string, bool) {

		if len(args) < 1 {
			return "", false
		}

		// remove next token from slice
		nxt := args[0]
		args = args[1:]

		return nxt, true
	}

	if args[0] == "-verify" || args[0] == "-validate" {

		// skip past command name
		args = args[1:]

		find := ""
		html := false
		max := 0

		// look for optional arguments
		for {
			arg, ok := nextArg()
			if !ok {
				break
			}

			switch arg {
			case "-find":
				// set identifier object
				find, _ = nextArg()
			case "-html":
				html = true
			case "-max":
				// override reportable depth
				cutoff, _ := nextArg()
				val, err := strconv.Atoi(cutoff)
				if err == nil && val > 0 {
					max = val
				}
			}
		}

		recordCount = eutils.ValidateXML(rdr, find, html, max)

		debug.FreeOSMemory()

		// suppress printing of lines if not properly counted
		if recordCount == 1 {
			recordCount = 0
		}

		if timr {
			printDuration("lines")
		}

		return
	}

	// MISCELLANEOUS TIMING COMMANDS

	if args[0] == "-chunk" {

		for str := range rdr {
			recordCount++
			byteCount += len(str)
		}

		printDuration("blocks")

		return
	}

	if args[0] == "-split" {

		if len(args) > 1 {
			if args[1] == "-pattern" {
				// skip past -split if followed by -pattern
				args = args[1:]
			}
		}
		if len(args) < 2 {
			eutils.DisplayError("Pattern missing after -split command")
			os.Exit(1)
		}
		pat := args[1]

		eutils.PartitionXML(pat, "", turbo, rdr,
			func(str string) {
				recordCount++
				byteCount += len(str)
			})

		printDuration("patterns")

		return
	}

	if args[0] == "-token" {

		eutils.StreamTokens(rdr,
			func(tkn eutils.XMLToken) {
				recordCount++
				byteCount += len(tkn.Name) + len(tkn.Attr)
			})

		printDuration("tokens")

		return
	}

	// INDEXED XML FILE PREPARATION

	// cat carotene.xml | xtract -timer -index -pattern PubmedArticle > carindex.txt
	// xtract -timer -turbo -input carindex.txt -pattern PubmedArticle -element LastName
	if args[0] == "-index" {

		if len(args) > 1 {
			if args[1] == "-pattern" {
				// skip past -index if followed by -pattern
				args = args[1:]
			}
		}
		if len(args) < 2 {
			eutils.DisplayError("Pattern missing after -index command")
			os.Exit(1)
		}
		pat := args[1]

		retlength := len("\n")

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		eutils.PartitionXML(pat, "", false, rdr,
			func(str string) {
				recordCount++
				nxt := len(str)
				byteCount += nxt
				newln := false

				if !strings.HasSuffix(str, "\n") {
					nxt += retlength
					newln = true
				}

				os.Stdout.WriteString("<NEXT_RECORD_SIZE>")
				val := strconv.Itoa(nxt)
				os.Stdout.WriteString(val)
				os.Stdout.WriteString("</NEXT_RECORD_SIZE>\n")

				os.Stdout.WriteString(str)
				if newln {
					os.Stdout.WriteString("\n")
				}
			})

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// ENSURE PRESENCE OF PATTERN ARGUMENT

	if len(args) < 1 {
		eutils.DisplayError("Insufficient command-line arguments supplied to xtract")
		os.Exit(1)
	}

	// allow -record as synonym of -pattern (undocumented)
	if args[0] == "-record" || args[0] == "-Record" {
		args[0] = "-pattern"
	}

	// make sure top-level -pattern command is next
	if args[0] != "-pattern" && args[0] != "-Pattern" {
		eutils.DisplayError("No -pattern in command-line arguments")
		os.Exit(1)
	}
	if len(args) < 2 {
		eutils.DisplayError("Item missing after -pattern command")
		os.Exit(1)
	}

	topPat := args[1]
	if topPat == "" {
		eutils.DisplayError("Item missing after -pattern command")
		os.Exit(1)
	}
	if strings.HasPrefix(topPat, "-") {
		eutils.DisplayError("Misplaced %s command", topPat)
		os.Exit(1)
	}

	// look for -pattern Parent/* construct for heterogeneous data, e.g., -pattern PubmedArticleSet/*
	topPattern, star := eutils.SplitInTwoLeft(topPat, "/")
	if topPattern == "" {
		return
	}

	parent := ""
	if star == "*" {
		parent = topPattern
	} else if star != "" {
		eutils.DisplayError("-pattern Parent/Child construct is not supported")
		os.Exit(1)
	}

	// SAVE ONLY RECORDS WITH NON-ASCII CHARACTERS

	// -pattern record_name -select -nonascii
	if len(args) == 4 && args[2] == "-select" && args[3] == "-nonascii" {

		xmlq := eutils.CreateXMLProducer(topPattern, star, false, rdr)
		fchq := eutils.CreateUnicoders(xmlq)
		unsq := eutils.CreateXMLUnshuffler(fchq)

		if xmlq == nil || fchq == nil || unsq == nil {
			eutils.DisplayError("Unable to create selector")
			os.Exit(1)
		}

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		// drain output channel
		for curr := range unsq {

			str := curr.Text

			if str == "" {
				continue
			}

			if hd != "" {
				os.Stdout.WriteString(hd)
				os.Stdout.WriteString("\n")
			}

			// send result to output
			os.Stdout.WriteString(str)
			if !strings.HasSuffix(str, "\n") {
				os.Stdout.WriteString("\n")
			}

			if tl != "" {
				os.Stdout.WriteString(tl)
				os.Stdout.WriteString("\n")
			}

			recordCount++
			runtime.Gosched()
		}

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// READ FILE OF IDENTIFIERS AND CONCURRENTLY EXTRACT SELECTED RECORDS

	// -pattern record_name -select parent/element@attribute^version -in file_of_identifiers
	if len(args) == 6 && args[2] == "-select" && (args[4] == "-in" || args[4] == "-retaining") {

		indx := args[3]
		unqe := args[5]

		// read file of identifiers to use for filtering
		fl, err := os.Open(unqe)
		if err != nil {
			eutils.DisplayError("Unable to open identifier file '%s'", unqe)
			os.Exit(1)
		}

		// create map that records each UID
		order := make(map[string]bool)

		scanr := bufio.NewScanner(fl)

		// read lines of identifiers
		for scanr.Scan() {

			line := scanr.Text()
			id, _ := eutils.SplitInTwoLeft(line, "\t")

			id = eutils.SortStringByWords(id)

			// add identifier to map
			order[id] = true
		}

		fl.Close()

		xmlq := eutils.CreateXMLProducer(topPattern, star, false, rdr)
		fchq := eutils.CreateSelectors(topPattern, indx, order, xmlq)
		unsq := eutils.CreateXMLUnshuffler(fchq)

		if xmlq == nil || fchq == nil || unsq == nil {
			eutils.DisplayError("Unable to create selector")
			os.Exit(1)
		}

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		// drain output channel
		for curr := range unsq {

			str := curr.Text

			if str == "" {
				continue
			}

			if hd != "" {
				os.Stdout.WriteString(hd)
				os.Stdout.WriteString("\n")
			}

			// send result to output
			os.Stdout.WriteString(str)
			if !strings.HasSuffix(str, "\n") {
				os.Stdout.WriteString("\n")
			}

			if tl != "" {
				os.Stdout.WriteString(tl)
				os.Stdout.WriteString("\n")
			}

			recordCount++
			runtime.Gosched()
		}

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// READ FILE OF IDENTIFIERS AND EXCLUDE SELECTED RECORDS

	// -pattern record_name -exclude element -excluding file_of_identifiers (undocumented)
	if len(args) == 6 && args[2] == "-select" && args[4] == "-excluding" {

		indx := args[3]
		unqe := args[5]

		// read file of identifiers to use for filtering
		fl, err := os.Open(unqe)
		if err != nil {
			eutils.DisplayError("Unable to open identifier file '%s'", unqe)
			os.Exit(1)
		}

		// create map that records each UID
		order := make(map[string]bool)

		scanr := bufio.NewScanner(fl)

		// read lines of identifiers
		for scanr.Scan() {

			line := scanr.Text()
			id, _ := eutils.SplitInTwoLeft(line, "\t")
			id = strings.ToLower(id)

			// add identifier to map
			order[id] = true
		}

		fl.Close()

		find := eutils.ParseIndex(indx)

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				id := eutils.FindIdentifier(str[:], parent, find)
				if id != "" {
					id = strings.ToLower(id)
					_, ok := order[id]
					if ok {
						// in exclusion list, skip
						return
					}
				}

				if hd != "" {
					os.Stdout.WriteString(hd)
					os.Stdout.WriteString("\n")
				}

				// write selected record
				os.Stdout.WriteString(str[:])
				os.Stdout.WriteString("\n")

				if tl != "" {
					os.Stdout.WriteString(tl)
					os.Stdout.WriteString("\n")
				}
			})

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// READ ORDERED FILE OF IDENTIFIERS AND XML STRINGS, APPEND XML JUST INSIDE CLOSING TAG OF APPROPRIATE RECORD

	// -pattern record_name -select element -appending file_of_identifiers_and_metadata (undocumented)
	if len(args) == 6 && args[2] == "-select" && args[4] == "-appending" {

		indx := args[3]
		apnd := args[5]

		fl, err := os.Open(apnd)
		if err != nil {
			eutils.DisplayError("Unable to open transformation file '%s'", apnd)
			os.Exit(1)
		}

		scanr := bufio.NewScanner(fl)

		find := eutils.ParseIndex(indx)

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		rgt := "</" + topPattern + ">"

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				id := eutils.FindIdentifier(str[:], parent, find)
				if id == "" {
					return
				}
				id = strings.ToLower(id)

				for scanr.Scan() {

					line := scanr.Text()
					frst, scnd := eutils.SplitInTwoLeft(line, "\t")
					frst = strings.ToLower(frst)

					if id != frst {
						return
					}
					if !strings.HasSuffix(str, rgt) {
						return
					}

					lft := strings.TrimSuffix(str, rgt)
					str = lft + "  " + scnd + "\n" + rgt

					if hd != "" {
						os.Stdout.WriteString(hd)
						os.Stdout.WriteString("\n")
					}

					os.Stdout.WriteString(str[:])
					os.Stdout.WriteString("\n")

					if tl != "" {
						os.Stdout.WriteString(tl)
						os.Stdout.WriteString("\n")
					}

					break
				}
			})

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		fl.Close()

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// SORT XML RECORDS BY IDENTIFIER

	// -pattern record_name -sort parent/element@attribute^version
	if len(args) == 4 && (args[2] == "-sort" || args[2] == "-sort-fwd" || args[2] == "-sort-rev") {

		sortInReverse := false
		if args[2] == "-sort-rev" {
			sortInReverse = true
		}

		indx := args[3]

		// create map that records each UID
		order := make(map[string][]string)

		find := eutils.ParseIndex(indx)

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				id := eutils.FindIdentifier(str[:], parent, find)
				if id == "" {
					return
				}

				data, ok := order[id]
				if !ok {
					data = make([]string, 0, 1)
				}
				data = append(data, str)
				// always need to update order, since data may be reallocated
				order[id] = data
			})

		// sort fields in alphabetical or numeric order, modified from sort.Slice (Less) to slices.SortedFunc (Compare) style
		xtractSort := func(i, j string) int {

			// check for numeric (integer or decimal) representation in both string arguments
			if eutils.IsAllDigitsOrPeriod(i) && eutils.IsAllDigitsOrPeriod(j) {

				// isolate integer and fractional component if decimal number
				ni, _ := eutils.SplitInTwoLeft(i, ".")
				nj, _ := eutils.SplitInTwoLeft(j, ".")
				// get lengths of whole number
				lni, lnj := len(ni), len(nj)
				// shorter string is numerically lower, assuming no leading zeros
				if lni < lnj {
					return -1
				}
				if lni > lnj {
					return 1
				}
				// integer components are same length, thus right (or decimal point) aligned for text
				// comparison of numeric values without needing expensive string-to-integer conversion
			}

			// same length integers or non-numeric, can now do string comparison on contents
			return cmp.Compare(i, j)
		}

		sortKeys := xtractSort
		if sortInReverse {
			sortKeys = func(i, j string) int {
				// sends arguments in reverse order
				return xtractSort(j, i)
			}
		}

		keys := slices.SortedFunc(maps.Keys(order), sortKeys)

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		for _, id := range keys {

			strs := order[id]
			for _, str := range strs {
				os.Stdout.WriteString(str)
				os.Stdout.WriteString("\n")
			}
		}

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// SPLIT FILE BY BY RECORD COUNT

	// split XML record into subfiles by count
	if len(args) == 8 && args[2] == "-split" && args[4] == "-prefix" && args[6] == "-suffix" {

		// e.g., -head "<IdxDocumentSet>" -tail "</IdxDocumentSet>" -pattern IdxDocument -split 250000 -prefix "biocon" -suffix "e2x"
		count := 0
		fnum := 0
		var (
			fl  *os.File
			err error
		)
		chunk, err := strconv.Atoi(args[3])
		if err != nil {
			eutils.DisplayError("-split argument '%s' is not an integer", err.Error())
			return
		}
		prefix := args[5]
		suffix := args[7]

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				if count >= chunk {
					if tail != "" {
						fl.WriteString(tail)
						fl.WriteString("\n")
					}
					fl.Close()
					count = 0
				}
				if count == 0 {
					fpath := fmt.Sprintf("%s%03d.%s", prefix, fnum, suffix)
					fl, err = os.Create(fpath)
					if err != nil {
						eutils.DisplayError("Unable to create path '%s'", err.Error())
						return
					}
					os.Stderr.WriteString(fpath + "\n")
					fnum++
					if head != "" {
						fl.WriteString(head)
						fl.WriteString("\n")
					}
				}
				count++

				fl.WriteString(str[:])
				fl.WriteString("\n")
			})

		if count >= chunk {
			if tail != "" {
				fl.WriteString(tail)
				fl.WriteString("\n")
			}
			fl.Close()
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// PARSE AND VALIDATE EXTRACTION ARGUMENTS

	// parse nested exploration instruction from command-line arguments
	cmds := eutils.ParseArguments(args, topPattern)
	if cmds == nil {
		eutils.DisplayError("Problem parsing command-line arguments")
		os.Exit(1)
	}

	// GLOBAL MAP FOR SORT-UNIQ-COUNT HISTOGRAM ARGUMENT

	histogram := make(map[string]int)

	// PERFORMANCE TIMING COMMAND

	// -stats with an extraction command prints XML size and processing time for each record
	if stts {

		legend := "REC\tOFST\tSIZE\tTIME"

		rec := 0

		// find := eutils.ParseIndex("MedlineCitation/PMID")

		eutils.PartitionXML(topPattern, star, turbo, rdr,
			func(str string) {
				rec++
				// id := eutils.FindIdentifier(str[:], parent, find)
				// if eutils.HasCombiningAccent(str[:]) && id != "" { fmt.Printf("%s\n", id) }
				beginTime := time.Now()
				eutils.ProcessExtract(str[:], parent, rec, hd, tl, transform, nil, histogram, cmds)
				endTime := time.Now()
				duration := endTime.Sub(beginTime)
				micro := int(float64(duration.Nanoseconds()) / 1e3)
				if legend != "" {
					fmt.Printf("%s\n", legend)
					legend = ""
				}
				fmt.Printf("%d\t%d\t%d\n", rec, len(str), micro)
			})

		return
	}

	// PERFORMANCE OPTIMIZATION FUNCTION

	// -trial -input fileName runs the specified extraction for each -proc from 1 to nCPU
	if trial && fileName != "" {

		legend := "CPU\tRATE\tDEV"

		for numServ := 1; numServ <= ncpu; numServ++ {

			numServe = numServ

			eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, turbo)

			runtime.GOMAXPROCS(numServ)

			sum := 0
			count := 0
			mean := 0.0
			m2 := 0.0

			// calculate mean and standard deviation of processing rate
			for range 5 {

				inFile, err := os.Open(fileName)
				if err != nil {
					eutils.DisplayError("Unable to open input file '%s'", fileName)
					os.Exit(1)
				}

				trdr := eutils.CreateXMLStreamer(inFile, nil)
				if trdr == nil {
					eutils.DisplayError("Unable to read input file")
					os.Exit(1)
				}

				xmlq := eutils.CreateXMLProducer(topPattern, star, turbo, trdr)
				tblq := eutils.CreateXMLConsumers(cmds, parent, hd, tl, transform, false, histogram, xmlq)

				if xmlq == nil || tblq == nil {
					eutils.DisplayError("Unable to create servers")
					os.Exit(1)
				}

				begTime := time.Now()
				recordCount = 0

				for range tblq {
					recordCount++
					runtime.Gosched()
				}

				inFile.Close()

				debug.FreeOSMemory()

				endTime := time.Now()
				expended := endTime.Sub(begTime)
				secs := float64(expended.Nanoseconds()) / 1e9

				if secs >= 0.000001 && recordCount > 0 {
					speed := int(float64(recordCount) / secs)
					sum += speed
					count++
					x := float64(speed)
					delta := x - mean
					mean += delta / float64(count)
					m2 += delta * (x - mean)
				}
			}

			if legend != "" {
				fmt.Printf("%s\n", legend)
				legend = ""
			}
			if count > 1 {
				vrc := m2 / float64(count-1)
				dev := int(math.Sqrt(vrc))
				fmt.Printf("%d\t%d\t%d\n", numServ, sum/count, dev)
			}
		}

		return
	}

	// PROCESS SINGLE SELECTED RECORD IF -pattern ARGUMENT IS IMMEDIATELY FOLLOWED BY -position COMMAND

	posn := ""
	if cmds.Visit == topPat {
		if cmds.Position == "outer" ||
			cmds.Position == "inner" ||
			cmds.Position == "even" ||
			cmds.Position == "odd" ||
			cmds.Position == "all" {
			// filter by record position when draining unshuffler channel
			posn = cmds.Position
			cmds.Position = ""
		}
	}

	if cmds.Visit == topPat && cmds.Position != "" && cmds.Position != "select" {

		qry := ""
		idx := 0
		rec := 0

		if cmds.Position == "first" {

			eutils.PartitionXML(topPattern, star, turbo, rdr,
				func(str string) {
					rec++
					if rec == 1 {
						qry = str
						idx = rec
					}
				})

		} else if cmds.Position == "last" {

			eutils.PartitionXML(topPattern, star, turbo, rdr,
				func(str string) {
					qry = str
					idx = rec
				})

		} else {

			// use numeric position
			number, err := strconv.Atoi(cmds.Position)
			if err != nil {
				eutils.DisplayError("Unrecognized position '%s'", cmds.Position)
				os.Exit(1)
			}

			eutils.PartitionXML(topPattern, star, turbo, rdr,
				func(str string) {
					rec++
					if rec == number {
						qry = str
						idx = rec
					}
				})
		}

		if qry == "" {
			return
		}

		// clear position on top node to prevent condition test failure
		cmds.Position = ""

		// process single selected record
		res := eutils.ProcessExtract(qry[:], parent, idx, hd, tl, transform, nil, histogram, cmds)

		if res != "" {
			fmt.Printf("%s", res)
		}

		return
	}

	// LAUNCH PRODUCER, CONSUMER, AND UNSHUFFLER GOROUTINES

	// launch producer goroutine to partition XML by pattern
	xmlq := eutils.CreateXMLProducer(topPattern, star, turbo, rdr)

	// launch consumer goroutines to parse and explore partitioned XML objects
	tblq := eutils.CreateXMLConsumers(cmds, parent, hd, tl, transform, forClassify, histogram, xmlq)

	// launch unshuffler goroutine to restore order of results
	unsq := eutils.CreateXMLUnshuffler(tblq)

	if xmlq == nil || tblq == nil || unsq == nil {
		eutils.DisplayError("Unable to create servers")
		os.Exit(1)
	}

	// PERFORMANCE SUMMARY

	/*
		if dbug {

			// drain results, but suppress extraction output
			for ext := range unsq {
				byteCount += len(ext.Text)
				recordCount++
				runtime.Gosched()
			}

			// force garbage collection, return memory to operating system
			debug.FreeOSMemory()

			// print processing parameters as XML object
			stopTime := time.Now()
			duration := stopTime.Sub(StartTime)
			seconds := float64(duration.Nanoseconds()) / 1e9

			// Threads is a more easily explained concept than GOMAXPROCS
			fmt.Printf("<Xtract>\n")
			fmt.Printf("  <Threads>%d</Threads>\n", numProcs)
			fmt.Printf("  <Parsers>%d</Parsers>\n", NumServe)
			fmt.Printf("  <Time>%.3f</Time>\n", seconds)
			if seconds >= 0.001 && recordCount > 0 {
				rate := int(float64(recordCount) / seconds)
				fmt.Printf("  <Rate>%d</Rate>\n", rate)
			}
			fmt.Printf("</Xtract>\n")

			return
		}
	*/

	// DRAIN OUTPUT CHANNEL TO EXECUTE EXTRACTION COMMANDS, RESTORE OUTPUT ORDER WITH HEAP

	recordCount, byteCount = eutils.DrainExtractions(head, tail, posn, mpty, idnt, histogram, unsq)

	if timr {
		printDuration("records")
	}
}
