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
// File Name:  rchive.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package main

import (
	"bufio"
	"bytes"
	"eutils"
	"fmt"
	"github.com/klauspost/pgzip"
	"hash/crc32"
	"html"
	"io"
	"io/ioutil"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CONCURRENT GOROUTINE SERVERS

// processes with single goroutine call defer close(out) so consumer(s) can range over channel
// processes with multiple instances call defer wg.Done(), separate goroutine uses wg.Wait() to delay close(out)

// MAIN FUNCTION

func main() {

	// skip past executable name
	args := os.Args[1:]

	if len(args) < 1 {
		eutils.DisplayError("No command-line arguments supplied to rchive")
		os.Exit(1)
	}

	// print EDirect local archive master and working paths, separated by colon
	if args[0] == "-local" && len(args) > 1 {
		db := args[1]
		mstr, wrkg := eutils.GetLocalArchivePaths(db)
		if mstr != "" && wrkg != "" {
			fmt.Fprintf(os.Stdout, "%s:%s\n", mstr, wrkg)
		}
		return
	}

	origArgs := args

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
	doStem := false
	deStop := true

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

	// garbage collector control can be set by environment variable or default value with -gogc 0
	goGc = 200
	gcdefault := true

	// -flag sets -strict or -mixed cleanup flags from argument
	flgs := ""

	// read data from file instead of stdin
	fileName := ""

	// -e2incIndex path to local archive
	archivePath := ""

	// -e2incInvert (and -e2incIndex) paths to incremental indices and inverted indices
	indicesPath := ""
	incrementPath := ""

	// flag for indexed input file
	turbo := false

	// debugging
	mpty := false
	idnt := false
	dbug := false
	stts := false
	timr := false

	// profiling
	prfl := false

	// -transform path can be added early or late in command line
	tform := ""

	// element to use as local data index
	indx := ""

	// file of index values for removing duplicates
	unqe := ""

	// database argument, built-in support for pubmed (default), pmc, and taxonomy
	db := ""

	// pattern argument, built-in support for PubmedArticle, PMCInfo, and TaxonInfo
	recname := ""

	// indexing options (e.g., STEM)
	options := ""

	// file with indexing argument lines
	idxargs := ""

	// rolling count limit for printing progress dot
	dotmax := 0

	// path for local data indexed as trie
	stsh := ""
	dlet := ""
	idcs := ""
	incr := ""
	ftch := ""
	strm := ""

	// offset for skipping separately-compressed xml/DOCTYPE block
	recskip := 0

	// path for local extra link data
	smmn := ""

	// flag for inverted index
	nvrt := false

	// flag for combining sets of inverted files
	join := false

	// flag for combining sets of inverted files
	fuse := false

	// destination directory for merging and splitting inverted files
	merg := ""
	isLink := false

	// base destination directory for promoting inverted index to retrieval indices
	prom := ""

	// fields for promoting inverted index files
	fild := ""

	// base for queries
	base := ""

	// query by phrase, normalized terms (with truncation wildcarding)
	phrs := ""
	xact := false
	titl := false
	mtch := false
	mock := false
	btch := false

	// print term list with counts
	trms := ""
	plrl := false
	psns := false

	// link field
	lnks := ""

	// use gzip compression on local data files
	zipp := false

	// create Pubmed-entry ASN.1 file from PubmedArticle XML
	pma2pme := false

	// make 6-digit .inv trie from PMID
	invt := false

	// print UIDs and hash values
	hshv := false

	// convert UIDs to archive trie
	trei := false

	arcvTrei := false
	idcsTrei := false
	pstgTrei := false
	linkTrei := false

	// pad PMIDs with leading zeros
	padz := false

	// compare input record against stash
	cmpr := false
	cmprType := ""
	ignr := ""

	// flag missing identifiers
	msng := false

	// flag records with damaged embedded HTML tags
	dmgd := false
	dmgdType := ""

	// kludge to use non-threaded fetching for windows
	windows := false

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
			gcdefault = false

		// read data from file
		case "-input":
			fileName = eutils.GetStringArg(args, "Input file name")
			args = args[1:]

		// path to local archive and index folders for incremental updating of cached index components
		case "-e2incIndex":
			archivePath = eutils.GetStringArg(args, "Path to local archive")
			args = args[1:]
			indicesPath = eutils.GetStringArg(args, "Path to local indices")
			args = args[1:]
			// should be followed by -transform meshtree.txt -e2index

		// path to local index folder for incremental updating of cached inverted index components
		case "-e2incInvert":
			indicesPath = eutils.GetStringArg(args, "Path to local indices")
			args = args[1:]
			incrementPath = eutils.GetStringArg(args, "Path to local increment")
			args = args[1:]

		// input is indexed with <NEXT_RECORD_SIZE> objects
		case "-turbo":
			turbo = true

		// file with selected indexes for removing duplicates
		case "-unique":
			unqe = eutils.GetStringArg(args, "Unique identifier file")
			args = args[1:]

		// database (currently pubmed, pmc, or taxonomy)
		case "-db":
			db = eutils.GetStringArg(args, "Local archive database")
			db = strings.ToLower(db)
			args = args[1:]

		case "-tag":
			recname = eutils.GetStringArg(args, "Local archive pattern")
			args = args[1:]

		case "-skip":
			recskip = eutils.GetNumericArg(args, "Bytes to skip compressed xml/DOCTYPE block", 0, 0, 1000)
			args = args[1:]

		case "-options":
			options = eutils.GetStringArg(args, "Local archive options")
			options = strings.ToLower(options)
			args = args[1:]

		case "-idxargs":
			idxargs = eutils.GetStringArg(args, "File with local archive indexing argument lines")
			args = args[1:]

		case "-dotmax":
			dotmax = eutils.GetNumericArg(args, "Progress dot printing frequency", 0, 0, 10000)
			args = args[1:]

		// local directory path for indexing
		case "-archive", "-stash":
			if len(args) < 4 {
				eutils.DisplayError("Archive, indices, and increment path needed")
				os.Exit(1)
			}
			stsh = eutils.GetStringArg(args, "Archive path")
			if stsh != "" && !strings.HasSuffix(stsh, "/") {
				stsh += "/"
			}
			args = args[1:]
			idcs = eutils.GetStringArg(args, "Indices path")
			if idcs != "" && !strings.HasSuffix(idcs, "/") {
				idcs += "/"
			}
			args = args[1:]
			incr = eutils.GetStringArg(args, "Increment path")
			if incr != "" && !strings.HasSuffix(incr, "/") {
				incr += "/"
			}
			args = args[1:]
		// local directory path for deletion
		case "-delete":
			if len(args) < 4 {
				eutils.DisplayError("Archive, indices, and increment path needed")
				os.Exit(1)
			}
			dlet = eutils.GetStringArg(args, "Archive path")
			if dlet != "" && !strings.HasSuffix(dlet, "/") {
				dlet += "/"
			}
			args = args[1:]
			idcs = eutils.GetStringArg(args, "Indices path")
			if idcs != "" && !strings.HasSuffix(idcs, "/") {
				idcs += "/"
			}
			args = args[1:]
			incr = eutils.GetStringArg(args, "Increment path")
			if incr != "" && !strings.HasSuffix(incr, "/") {
				incr += "/"
			}
			args = args[1:]
		// local directory path for retrieval
		case "-fetch":
			ftch = eutils.GetStringArg(args, "Fetch path")
			if ftch != "" && !strings.HasSuffix(ftch, "/") {
				ftch += "/"
			}
			args = args[1:]
		// local directory path for retrieval of compressed XML
		case "-stream":
			strm = eutils.GetStringArg(args, "Stream path")
			if strm != "" && !strings.HasSuffix(strm, "/") {
				strm += "/"
			}
			args = args[1:]

		// local directory path for extra link retrieval
		case "-summon":
			smmn = eutils.GetStringArg(args, "Summon path")
			args = args[1:]

		// index transformation file
		case "-transform":
			tform = eutils.GetStringArg(args, "Transformation file")
			args = args[1:]

		// data element for indexing
		case "-index":
			indx = eutils.GetStringArg(args, "Index element")
			args = args[1:]

		// build inverted index
		case "-e2invert":
			nvrt = true

		// combine sets of inverted index files
		case "-join":
			join = true

		case "-fuse":
			fuse = true

		case "-mergelink":
			isLink = true
			fallthrough
		// merge inverted index files, distribute by prefix
		case "-merge":
			merg = eutils.GetStringArg(args, "Merge field")
			args = args[1:]

		case "-promotelink":
			isLink = true
			fallthrough
		// promote inverted index to term-specific postings files
		case "-promote":
			if len(args) < 3 {
				eutils.DisplayError("Promote path is missing")
				os.Exit(1)
			}
			prom = args[1]
			fild = args[2]
			// skip past first and second arguments
			args = args[2:]

		case "-path":
			base = eutils.GetStringArg(args, "Postings path")
			args = args[1:]

		case "-title":
			titl = true
			fallthrough
		case "-exact":
			xact = true
			fallthrough
		case "-query", "-search":
			phrs = eutils.GetStringArg(args, "Query argument")
			args = args[1:]

		case "-match", "-partial":
			mtch = true
			phrs = eutils.GetStringArg(args, "Match argument")
			args = args[1:]

		case "-link":
			lnks = eutils.GetStringArg(args, "Links field")
			isLink = true
			args = args[1:]

		case "-batch":
			btch = true

		case "-mockt":
			titl = true
			fallthrough
		case "-mockx":
			xact = true
			fallthrough
		case "-mock", "-mocks":
			phrs = eutils.GetStringArg(args, "Query argument")
			mock = true
			args = args[1:]

		// -countp tests the files containing positions of terms per UID (undocumented)
		case "-countp":
			psns = true
			fallthrough
		case "-counts":
			plrl = true
			fallthrough
		case "-count", "-countr":
			trms = eutils.GetStringArg(args, "Count argument")
			args = args[1:]

		case "-gzip":
			zipp = true
		case "-asn":
			pma2pme = true
		case "-xml":
			pma2pme = false
		case "-inv":
			invt = true
		case "-hash":
			hshv = true
		case "-trie":
			trei = true
			if len(args) > 1 {
				next := args[1]
				// if next argument is not another flag
				if next != "" && next[0] != '-' {
					// get type of trie
					switch next {
					case "archive":
						arcvTrei = true
					case "indices":
						idcsTrei = true
					case "posting", "postings":
						pstgTrei = true
					case "link", "links":
						linkTrei = true
					}
					// skip past first of two arguments
					args = args[1:]
				}
			}
		case "-padz":
			padz = true
		// check for missing records
		case "-missing":
			msng = true

		// use non-threaded fetch function for windows (undocumented)
		case "-windows":
			windows = true

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

		case "-unicode":
			// DoUnicode = true
		case "-script":
			// DoScript = true
		case "-mathml":
			// DoMathML = true

		case "-flag", "-flags":
			if len(args) < 2 {
				eutils.DisplayError("-flags argument is missing")
				os.Exit(1)
			}
			flgs = eutils.GetStringArg(args, "Flags argument")
			args = args[1:]

		// debugging flags
		case "-damaged", "-damage", "-broken":
			dmgd = true
			if len(args) > 1 {
				next := args[1]
				// if next argument is not another flag
				if next != "" && next[0] != '-' {
					// get optional extraction class (SELF, SINGLE, DOUBLE, AMPER, or ALL)
					dmgdType = next
					// skip past first of two arguments
					args = args[1:]
				}
			}
		case "-prepare":
			cmpr = true
			if len(args) > 1 {
				next := args[1]
				// if next argument is not another flag
				if next != "" && next[0] != '-' {
					// get optional data source specifier
					cmprType = next
					// skip past first of two arguments
					args = args[1:]
				}
			}
		case "-ignore":
			ignr = eutils.GetStringArg(args, "-ignore value")
			args = args[1:]

		// debugging flags
		case "-debug":
			dbug = true
		case "-stats", "-stat":
			stts = true
		case "-timer":
			timr = true
		case "-profile":
			prfl = true

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

	// -flag allows script to set -strict or -mixed (or -stops) from argument
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
		UnicodeFix = ParseMarkup(unicodePolicy, "-unicode")
		ScriptFix = ParseMarkup(scriptPolicy, "-script")
		MathMLFix = ParseMarkup(mathmlPolicy, "-mathml")

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

	// if copying from local files accessed by identifier, add dummy argument to bypass length tests
	if stsh != "" && indx == "" {
		args = append(args, "-dummy")
	} else if dlet != "" {
		args = append(args, "-dummy")
	} else if ftch != "" || strm != "" || smmn != "" {
		args = append(args, "-dummy")
	} else if base != "" {
		args = append(args, "-dummy")
	} else if trei || padz || dmgd || cmpr {
		args = append(args, "-dummy")
	}

	// expand -archive ~/ to home directory path
	if stsh != "" {

		if stsh[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				stsh = strings.Replace(stsh, "~/", hom+"/", 1)
			}
		}
	}
	if dlet != "" {

		if dlet[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				dlet = strings.Replace(dlet, "~/", hom+"/", 1)
			}
		}
	}
	if idcs != "" {

		if idcs[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				idcs = strings.Replace(idcs, "~/", hom+"/", 1)
			}
		}
	}
	if incr != "" {

		if incr[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				incr = strings.Replace(incr, "~/", hom+"/", 1)
			}
		}
	}

	// expand -fetch ~/ to home directory path
	if ftch != "" {

		if ftch[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				ftch = strings.Replace(ftch, "~/", hom+"/", 1)
			}
		}
	}

	// expand -stream ~/ to home directory path
	if strm != "" {

		if strm[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				strm = strings.Replace(strm, "~/", hom+"/", 1)
			}
		}
	}

	// expand -promote ~/ to home directory path
	if prom != "" {

		if prom[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				prom = strings.Replace(prom, "~/", hom+"/", 1)
			}
		}
	}

	// expand -summon ~/ to home directory path
	if smmn != "" {

		if smmn[:2] == "~/" {
			cur, err := user.Current()
			if err == nil {
				hom := cur.HomeDir
				smmn = strings.Replace(smmn, "~/", hom+"/", 1)
			}
		}
	}

	// DOCUMENTATION COMMANDS

	if len(args) > 0 {

		inSwitch = true

		switch args[0] {
		case "-version":
			fmt.Printf("%s\n", eutils.EDirectVersion)
		case "-help", "help", "--help":
			eutils.PrintHelp("rchive", "rchive-help.txt")
		case "-extras", "-extra", "-advanced":
			eutils.PrintHelp("rchive", "rchive-extras.txt")
		case "-internal", "-internals":
			eutils.PrintHelp("rchive", "rchive-internal.txt")
		default:
			// if not any of the documentation commands, keep going
			inSwitch = false
		}

		if inSwitch {
			return
		}
	}

	// FILE NAME CAN BE SUPPLIED WITH -input COMMAND

	in := os.Stdin

	// check for data being piped into stdin
	isPipe := false
	fi, staterr := os.Stdin.Stat()
	if staterr == nil {
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

	transform := make(map[string]string)

	populateTx := func(tf string) {

		inFile, err := os.Open(tf)
		if err != nil {
			eutils.DisplayError("Unable to open transformation file %s", err.Error())
			os.Exit(1)
		}
		defer inFile.Close()

		scanr := bufio.NewScanner(inFile)

		// populate transformation map for -translate (and -matrix) output
		for scanr.Scan() {

			line := scanr.Text()
			frst, scnd := eutils.SplitInTwoLeft(line, "\t")

			transform[frst] = scnd
		}
	}

	// also check for -transform late in command line
	if len(args) > 2 && args[0] == "-transform" {
		tform = args[1]
		args = args[2:]
	}

	if tform != "" {
		populateTx(tform)
	}

	// SPECIFY STRINGS TO GO BEFORE AND AFTER ENTIRE OUTPUT OR EACH RECORD

	head := ""
	tail := ""

	hd := ""
	tl := ""

	parseHeadTail := func() {

		for {

			if len(args) < 1 {
				break
			}

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
				eutils.DisplayError("Insufficient command-line arguments supplied to rchive")
				os.Exit(1)
			}
		}
	}

	// -e2incIndex FOLLOWED BY -transform meshtree.txt AND -e2index

	if len(args) > 0 && args[0] == "-e2index" && archivePath != "" && indicesPath != "" {

		// skip past command name
		args = args[1:]

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_INDEX_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_INDEX_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)
			}
		}

		pfx := ""

		if recname == "" {
			if db == "pubmed" {
				recname = "PubmedArticle"
			} else if db == "pmc" {
				pfx = "PMC"
				recname = "PMCInfo"
			} else if db == "taxonomy" {
				recname = "TaxonInfo"
			}
		}

		res := eutils.MakeE2Commands(tform, idxargs)

		// data in pipe, so replace arguments, execute dynamically
		args = res

		if len(args) < 1 {
			eutils.DisplayError("-e2index argument generation failure")
			os.Exit(1)
		}

		// parse new -head, -tail, etc.
		parseHeadTail()

		// parse expected -e2index generated arguments
		cmds := eutils.ParseArguments(args, recname)
		if cmds == nil {
			eutils.DisplayError("Problem parsing -e2index arguments after -e2incIndex")
			os.Exit(1)
		}

		callConsumers := func(inp <-chan eutils.XMLRecord) <-chan eutils.XMLRecord {

			// closure allows access to unchanging cmds and transform arguments
			return eutils.CreateXMLConsumers(cmds, "", "<IdxDocument>", "</IdxDocument>", transform, false, nil, inp)
		}

		e2iq := eutils.IncrementalIndex(db, pfx, recname, dotmax, callConsumers)
		if e2iq == nil {
			eutils.DisplayError("Unable to create indexer channel")
			os.Exit(1)
		}

		// drain channel for names of folder-specific inverted index files that were updated
		for range e2iq {
			recordCount++
			// fmt.Fprintf(os.Stdout, "%s\n", itm)
			runtime.Gosched()
		}

		// newline after progress monitor dots
		fmt.Fprintf(os.Stdout, "\n")

		debug.FreeOSMemory()

		if timr {
			printDuration("files")
		}

		return
	}

	// -e2incInvert

	if len(args) == 0 && archivePath == "" && indicesPath != "" && incrementPath != "" {

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_INDEX_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_INDEX_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)
			}
		}

		// parse new -head, -tail, etc.
		parseHeadTail()

		e2iq := eutils.IncrementalInvert(db, dotmax)
		if e2iq == nil {
			eutils.DisplayError("Unable to create inverter channel")
			os.Exit(1)
		}

		// drain channel for names of folder-specific inverted index files that were updated
		for range e2iq {
			recordCount++
			// fmt.Fprintf(os.Stdout, "%s\n", itm)
			runtime.Gosched()
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("files")
		}

		return
	}

	// -delete REMOVES RECORDS AND INCREMENTAL INDICES BY LIST OF PMIDs

	if dlet != "" {

		dltq := eutils.CreateDeleter(dlet, db, in)
		clrq := eutils.CreateClearer(idcs, incr, db, dltq)

		if dltq == nil || clrq == nil {
			eutils.DisplayError("Unable to create deleter")
			os.Exit(1)
		}

		// drain output channel
		for str := range clrq {

			if dbug {
				fmt.Fprintf(os.Stderr, "%s\n", str)
			}

			recordCount++
			runtime.Gosched()
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// -e2index PROCESSING OF PUBMED RECORDS

	if len(args) > 0 && args[0] == "-e2index" {

		// e.g., rchive -transform [meshtree.txt] -e2index

		// skip past command name
		args = args[1:]

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_INDEX_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_INDEX_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, turbo)
			}
		}

		if recname == "" {
			if db == "pubmed" {
				recname = "PubmedArticle"
			} else if db == "pmc" {
				recname = "PMCInfo"
			} else if db == "taxonomy" {
				recname = "TaxonInfo"
			}
		}

		res := eutils.MakeE2Commands(tform, idxargs)

		if !isPipe && !usingFile {
			// no piped input, so write output instructions
			fmt.Printf("rchive")
			if tform != "" {
				fmt.Printf(" -transform %s", tform)
			}
			for _, str := range res {
				if strings.HasPrefix(str, "-") {
					fmt.Printf(" %s", str)
				} else {
					fmt.Printf(" \"%s\"", str)
				}
			}
			fmt.Printf("\n")
			return
		}

		// data in pipe, so replace arguments, execute dynamically
		args = res

		if len(args) < 1 {
			eutils.DisplayError("-e2index argument generation failure")
			os.Exit(1)
		}

		// parse new -head, -tail, etc.
		parseHeadTail()

		// parse expected -e2index generated arguments
		cmds := eutils.ParseArguments(args, recname)
		if cmds == nil {
			eutils.DisplayError("Problem parsing -e2index arguments")
			os.Exit(1)
		}

		rdr := eutils.CreateXMLStreamer(in, nil)

		if rdr == nil {
			eutils.DisplayError("Unable to create XML Block Reader")
			os.Exit(1)
		}

		// launch producer goroutine to partition XML by pattern
		xmlq := eutils.CreateXMLProducer(recname, "", false, rdr)

		// launch consumer goroutines to parse and explore partitioned XML objects
		tblq := eutils.CreateXMLConsumers(cmds, recname, hd, tl, transform, false, nil, xmlq)

		// launch unshuffler goroutine to restore order of results
		unsq := eutils.CreateXMLUnshuffler(tblq)

		if xmlq == nil || tblq == nil || unsq == nil {
			eutils.DisplayError("Unable to create servers")
			os.Exit(1)
		}

		recordCount, byteCount = eutils.DrainExtractions(head, tail, "", mpty, idnt, nil, unsq)

		if timr {
			printDuration("records")
		}

		return
	}

	// PRINT ALL TERMS IN A FIELD

	if len(args) > 1 && args[0] == "-terms" {

		// skip past command name
		args = args[1:]

		fld := args[0]

		trms := eutils.StreamTerms(db, fld)
		if trms == nil {
			eutils.DisplayError("Unable to create term generator")
			os.Exit(1)
		}

		for str := range trms {
			// returning multi-line strings, no need for trailing newline
			fmt.Fprintf(os.Stdout, "%s", str)
		}

		return
	}

	// PRINT ALL TERMS AND COUNTS IN A FIELD

	if len(args) > 1 && args[0] == "-totals" {

		// skip past command name
		args = args[1:]

		fld := args[0]

		trms := eutils.StreamTotals(db, fld)
		if trms == nil {
			eutils.DisplayError("Unable to create totals generator")
			os.Exit(1)
		}

		for str := range trms {
			// returning multi-line strings, no need for trailing newline
			fmt.Fprintf(os.Stdout, "%s", str)
		}

		return
	}

	// JOIN SUBSETS OF INVERTED INDEX FILES

	// -join combines subsets of inverted files for subsequent -merge operation
	if join {

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_JOIN_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		} else if gcdefault {
			// default to 200 for join
			debug.SetGCPercent(200)
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_JOIN_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)
			}
		}

		chns := eutils.CreatePresenters(args)
		mfld := eutils.CreateManifold(chns)
		mrgr := eutils.CreateMergers(mfld)
		unsq := eutils.CreateXMLUnshuffler(mrgr)

		if chns == nil || mfld == nil || mrgr == nil || unsq == nil {
			eutils.DisplayError("Unable to create inverted index joiner")
			os.Exit(1)
		}

		var out io.Writer

		out = os.Stdout

		if zipp {

			zpr, err := pgzip.NewWriterLevel(out, pgzip.BestSpeed)
			if err != nil {
				eutils.DisplayError("Unable to create compressor")
				os.Exit(1)
			}

			// close decompressor when all records have been processed
			defer zpr.Close()

			// use compressor for writing file
			out = zpr
		}

		// create buffered writer layer
		wrtr := bufio.NewWriter(out)

		wrtr.WriteString("<InvDocumentSet>\n")

		// drain channel of alphabetized results
		for curr := range unsq {

			str := curr.Text

			if str == "" {
				continue
			}

			// send result to output
			wrtr.WriteString(str)

			recordCount++
			runtime.Gosched()
		}

		wrtr.WriteString("</InvDocumentSet>\n\n")

		wrtr.Flush()

		debug.FreeOSMemory()

		if timr {
			printDuration("terms")
		}

		return
	}

	// MERGE INVERTED INDEX FILES AND GROUP BY TERM

	// -merge combines inverted files, distributes by prefix
	if merg != "" {

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_MERGE_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		} else if gcdefault {
			// default to 100 for merge
			debug.SetGCPercent(100)
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_MERGE_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)
			}
		}

		chns := eutils.CreatePresenters(args)
		mfld := eutils.CreateManifold(chns)
		mrgr := eutils.CreateMergers(mfld)
		unsq := eutils.CreateXMLUnshuffler(mrgr)
		sptr := eutils.CreateSplitter(merg, db, zipp, isLink, unsq)

		if chns == nil || mfld == nil || mrgr == nil || unsq == nil || sptr == nil {
			eutils.DisplayError("Unable to create inverted index merger")
			os.Exit(1)
		}

		// drain channel, print two-to-four-character index name
		startTime := time.Now()
		first := true
		col := 0
		spaces := "       "

		for str := range sptr {

			stopTime := time.Now()
			duration := stopTime.Sub(startTime)
			seconds := float64(duration.Nanoseconds()) / 1e9

			if timr {
				if first {
					first = false
				} else {
					fmt.Fprintf(os.Stdout, "%.3f\n", seconds)
				}
				fmt.Fprintf(os.Stdout, "%s\t", str)
			} else {
				blank := 7 - len(str)
				if blank > 0 {
					fmt.Fprintf(os.Stdout, "%s", spaces[:blank])
				}
				fmt.Fprintf(os.Stdout, "%s", str)
				col++
				if col >= 10 {
					col = 0
					fmt.Fprintf(os.Stdout, "\n")
				}
			}

			recordCount++
			runtime.Gosched()

			startTime = time.Now()
		}

		stopTime := time.Now()
		duration := stopTime.Sub(startTime)
		seconds := float64(duration.Nanoseconds()) / 1e9

		if timr {
			fmt.Fprintf(os.Stdout, "%.3f\n", seconds)
		} else if col > 0 {
			fmt.Fprintf(os.Stdout, "\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("groups")
		}

		return
	}

	// PROMOTE MERGED INVERTED INDEX TO TERM LIST AND POSTINGS FILES

	if prom != "" && fild != "" {

		prmq := eutils.CreatePromoters(prom, db, fild, isLink, args)

		if prmq == nil {
			eutils.DisplayError("Unable to create new postings file generator")
			os.Exit(1)
		}

		col := 0
		spaces := "       "

		// drain channel, print 2-4 character file prefix
		for str := range prmq {

			blank := 7 - len(str)
			if blank > 0 {
				fmt.Fprintf(os.Stdout, "%s", spaces[:blank])
			}
			fmt.Fprintf(os.Stdout, "%s", str)
			col++
			if col >= 10 {
				col = 0
				fmt.Fprintf(os.Stdout, "\n")
			}

			recordCount++
			runtime.Gosched()
		}

		if col > 0 {
			fmt.Fprintf(os.Stdout, "\n")
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("terms")
		}

		return
	}

	// QUERY POSTINGS FILES

	if btch {

		// read query lines for exact match
		scanr := bufio.NewScanner(in)

		for scanr.Scan() {
			txt := scanr.Text()

			// deStop should match value used in building the indices
			recordCount += eutils.ProcessSearch(db, txt, true, false, false, deStop)
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	if phrs != "" {

		// deStop should match value used in building the indices
		if mock {
			recordCount = eutils.ProcessMock(db, phrs, xact, titl, deStop)
		} else if mtch {
			eutils.ProcessMatch(db, phrs, deStop)
		} else {
			recordCount = eutils.ProcessSearch(db, phrs, xact, titl, false, deStop)
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	if lnks != "" {

		eutils.ProcessLinks(db, lnks)

		debug.FreeOSMemory()

		if timr {
			printDuration("terms")
		}

		return
	}

	if trms != "" {

		// deStop should match value used in building the indices
		recordCount = eutils.ProcessCount(db, trms, plrl, psns, deStop)

		debug.FreeOSMemory()

		if timr {
			printDuration("terms")
		}

		return
	}

	// CONFIRM INPUT DATA AVAILABILITY AFTER RUNNING COMMAND GENERATORS

	if fileName == "" && runtime.GOOS != "windows" {

		fromStdin := bool((fi.Mode() & os.ModeCharDevice) == 0)
		if !isPipe || !fromStdin {
			mode := fi.Mode().String()
			eutils.DisplayError("No data supplied to rchive from stdin or file, mode is '%s'", mode)
			os.Exit(1)
		}
	}

	if !usingFile && !isPipe {

		eutils.DisplayError("No XML input data supplied to rchive")
		os.Exit(1)
	}

	// SPECIFY STRINGS TO GO BEFORE AND AFTER ENTIRE OUTPUT OR EACH RECORD

	parseHeadTail()

	// PAD IDENTIFIER WITH LEADING ZEROS

	if padz {

		scanr := bufio.NewScanner(in)

		// read lines of identifiers
		for scanr.Scan() {

			str := scanr.Text()

			if len(str) > 64 {
				continue
			}

			if eutils.IsAllDigits(str) {

				// pad numeric identifier to 8 characters with leading zeros
				ln := len(str)
				if ln < 8 {
					zeros := "00000000"
					str = zeros[ln:] + str
				}
			}

			os.Stdout.WriteString(str)
			os.Stdout.WriteString("\n")
		}

		return
	}

	// PRODUCE ARCHIVE SUBPATH FROM IDENTIFIER

	// -trie converts identifier to directory subpath plus file name (undocumented)
	if trei {

		scanr := bufio.NewScanner(in)

		sfx := ".xml"
		if pma2pme {
			sfx = ".asn"
		} else if idcsTrei || invt {
			sfx = ".e2x"
		} else if pstgTrei {
			sfx = ""
		}
		if zipp {
			sfx += ".gz"
		}

		// read lines of identifiers
		for scanr.Scan() {

			file := scanr.Text()

			dir := ""
			id := ""

			if arcvTrei {
				dir, id = eutils.ArchiveTrie(file)
			} else if idcsTrei || invt {
				dir, id = eutils.IndexTrie(file)
			} else if pstgTrei {
				dir, id = eutils.PostingsTrie(file)
			} else if linkTrei {
				dir, id = eutils.LinksTrie(file, true)
			} else {
				dir, id = eutils.ArchiveTrie(file)
			}

			if id == "" {
				continue
			}
			if dir == "" {
				continue
			}

			fpath := filepath.Join(dir, id+sfx)
			if fpath == "" {
				continue
			}

			os.Stdout.WriteString(fpath)
			os.Stdout.WriteString("\n")
		}

		return
	}

	// CHECK FOR MISSING RECORDS IN LOCAL DIRECTORY INDEXED BY TRIE ON IDENTIFIER

	// -archive plus -missing checks for missing records
	if stsh != "" && msng {

		scanr := bufio.NewScanner(in)

		sfx := ".xml"
		if zipp {
			sfx += ".gz"
		}

		// read lines of identifiers
		for scanr.Scan() {

			id := scanr.Text()

			pos := strings.Index(id, ".")
			if pos >= 0 {
				// remove version suffix
				id = id[:pos]
			}

			dir, file := eutils.ArchiveTrie(id)

			if dir == "" || file == "" {
				continue
			}

			fpath := filepath.Join(stsh, dir, file+sfx)
			if fpath == "" {
				continue
			}

			_, err := os.Stat(fpath)

			// if failed to find ".xml" file, try ".xml.gz" without requiring -gzip
			if err != nil && os.IsNotExist(err) && !zipp {
				fpath := filepath.Join(stsh, dir, file+".xml.gz")
				if fpath == "" {
					continue
				}
				_, err = os.Stat(fpath)
			}
			if err != nil && os.IsNotExist(err) {
				// record is missing from local file cache
				os.Stdout.WriteString(file)
				os.Stdout.WriteString("\n")
			}
		}

		return
	}

	// RETRIEVE XML COMPONENT RECORDS FROM LOCAL DIRECTORY INDEXED BY TRIE ON IDENTIFIER

	// alternative windows version limits memory by not using goroutines
	if ftch != "" && indx == "" && runtime.GOOS == "windows" && windows {

		scanr := bufio.NewScanner(in)
		if scanr == nil {
			eutils.DisplayError("Unable to create UID scanner")
			os.Exit(1)
		}

		sfx := ".xml"
		if pma2pme {
			sfx = ".asn"
		}
		if zipp {
			sfx += ".gz"
		}

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		var buf bytes.Buffer

		for scanr.Scan() {

			// read next identifier
			id := scanr.Text()

			pos := strings.Index(id, ".")
			if pos >= 0 {
				// remove version suffix
				id = id[:pos]
			}

			dir, file := eutils.ArchiveTrie(id)

			if dir == "" || file == "" {
				continue
			}

			fpath := filepath.Join(ftch, dir, file+sfx)
			if fpath == "" {
				continue
			}

			iszip := zipp

			inFile, err := os.Open(fpath)

			// if failed to find ".xml" file, try ".xml.gz" without requiring -gzip
			if err != nil && os.IsNotExist(err) && !zipp {
				iszip = true
				fpath := filepath.Join(ftch, dir, file+".xml.gz")
				if fpath == "" {
					continue
				}
				inFile, err = os.Open(fpath)
			}
			if err != nil {
				continue
			}

			buf.Reset()

			brd := bufio.NewReader(inFile)

			if iszip {

				zpr, err := pgzip.NewReader(brd)

				if err == nil {
					// copy and decompress cached file contents
					buf.ReadFrom(zpr)
				}

				zpr.Close()

			} else {

				// copy cached file contents
				buf.ReadFrom(brd)
			}

			inFile.Close()

			str := buf.String()

			if str == "" {
				continue
			}

			if !pma2pme {
				pos := strings.Index(str, "<PubmedArticle")
				if pos > 0 {
					// remove leading xml and DOCTYPE lines
					str = str[pos:]
					if str == "" {
						continue
					}
				}
			}

			recordCount++

			if hd != "" {
				os.Stdout.WriteString(hd)
				os.Stdout.WriteString("\n")
			}

			if hshv {
				// calculate hash code for verification table
				hsh := crc32.NewIEEE()
				hsh.Write([]byte(str))
				val := hsh.Sum32()
				res := strconv.FormatUint(uint64(val), 10)
				txt := file + "\t" + res + "\n"
				os.Stdout.WriteString(txt)
			} else {
				// send result to output
				os.Stdout.WriteString(str)
				if !strings.HasSuffix(str, "\n") {
					os.Stdout.WriteString("\n")
				}
			}

			if tl != "" {
				os.Stdout.WriteString(tl)
				os.Stdout.WriteString("\n")
			}

			debug.FreeOSMemory()
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

	// -fetch without -index retrieves XML files in trie-based directory structure
	if ftch != "" && indx == "" {

		pfx := ""
		sfx := ".xml"

		if db == "pmc" {
			pfx = "PMC"
		}

		if pma2pme {
			sfx = ".asn"
		}

		if recname == "" {
			if db == "pubmed" {
				recname = "PubmedArticle"
			} else if db == "pmc" {
				recname = "PMCInfo"
			} else if db == "taxonomy" {
				recname = "TaxonInfo"
			}
		}

		uidq := eutils.CreateUIDReader(in)
		strq := eutils.CreateFetchers(ftch, db, pfx, sfx, recname, zipp, uidq)
		unsq := eutils.CreateXMLUnshuffler(strq)

		if uidq == nil || strq == nil || unsq == nil {
			eutils.DisplayError("Unable to create archive reader")
			os.Exit(1)
		}

		retlength := len("\n")

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

			if hshv {
				// calculate hash code for verification table
				hsh := crc32.NewIEEE()
				hsh.Write([]byte(str))
				val := hsh.Sum32()
				res := strconv.FormatUint(uint64(val), 10)
				txt := curr.Ident + "\t" + res + "\n"
				os.Stdout.WriteString(txt)
			} else {
				// send result to output
				newln := false
				if !strings.HasSuffix(str, "\n") {
					newln = true
				}

				if turbo {
					os.Stdout.WriteString("<NEXT_RECORD_SIZE>")
					nxt := len(str)
					if newln {
						nxt += retlength
					}
					val := strconv.Itoa(nxt)
					os.Stdout.WriteString(val)
					os.Stdout.WriteString("</NEXT_RECORD_SIZE>\n")
				}

				os.Stdout.WriteString(str)
				if newln {
					os.Stdout.WriteString("\n")
				}
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

	// -stream without -index retrieves compressed XML files in trie-based directory structure
	if strm != "" && indx == "" {

		pfx := ""
		sfx := ".xml"

		if pma2pme {
			sfx = ".asn"
		}

		if db == "pmc" {
			pfx = "PMC"
		}

		uidq := eutils.CreateUIDReader(in)
		strq := eutils.CreateCacheStreamers(strm, db, pfx, sfx, recskip, uidq)
		unsq := eutils.CreateXMLUnshuffler(strq)

		if uidq == nil || strq == nil || unsq == nil {
			eutils.DisplayError("Unable to create archive reader")
			os.Exit(1)
		}

		// drain output channel
		for curr := range unsq {

			data := curr.Data

			if data == nil {
				continue
			}

			recordCount++
			runtime.Gosched()

			_, err := os.Stdout.Write(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			}
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// -summon retrieves link files in trie-based directory structure
	if smmn != "" && indx == "" {

		if recname == "" {
			if db == "pubmed" {
				recname = "PubmedArticle"
			} else if db == "pmc" {
				recname = "PMCInfo"
			} else if db == "taxonomy" {
				recname = "TaxonInfo"
			}
		}

		uidq := eutils.CreateUIDReader(in)
		strq := eutils.CreateFetchers(smmn, db, "", ".e2x", recname, zipp, uidq)
		unsq := eutils.CreateXMLUnshuffler(strq)

		if uidq == nil || strq == nil || unsq == nil {
			eutils.DisplayError("Unable to create link reader")
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

			if hshv {
				// calculate hash code for verification table
				hsh := crc32.NewIEEE()
				hsh.Write([]byte(str))
				val := hsh.Sum32()
				res := strconv.FormatUint(uint64(val), 10)
				txt := curr.Ident + "\t" + res + "\n"
				os.Stdout.WriteString(txt)
			} else {
				// send result to output
				os.Stdout.WriteString(str)
				if !strings.HasSuffix(str, "\n") {
					os.Stdout.WriteString("\n")
				}
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

	// ENTREZ INDEX INVERSION

	// -e2invert reads IdxDocumentSet XML and creates an inverted index
	if nvrt {

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_INVERT_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_INVERT_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)
			}
		}

		byt, err := io.ReadAll(in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return
		}

		str := string(byt)
		if str == "" {
			return
		}

		if !strings.HasSuffix(str, "\n") {
			str += "\n"
		}

		colq := eutils.StringToChan(str)
		iifq := eutils.InvertIndexedFile(colq)

		if colq == nil || iifq == nil {
			eutils.DisplayError("Unable to create inverter")
			os.Exit(1)
		}

		var out io.Writer

		out = os.Stdout

		if zipp {

			zpr, err := pgzip.NewWriterLevel(out, pgzip.BestSpeed)
			if err != nil {
				eutils.DisplayError("Unable to create compressor")
				os.Exit(1)
			}

			// close decompressor when all records have been processed
			defer zpr.Close()

			// use compressor for writing file
			out = zpr
		}

		// create buffered writer layer
		wrtr := bufio.NewWriter(out)

		wrtr.WriteString("<InvDocumentSet>\n")

		// drain channel of alphabetized results
		for str := range iifq {

			// send result to output
			wrtr.WriteString(str)

			recordCount++
			runtime.Gosched()
		}

		wrtr.WriteString("</InvDocumentSet>\n\n")

		wrtr.Flush()

		debug.FreeOSMemory()

		if timr {
			printDuration("terms")
		}

		return
	}

	// CREATE XML BLOCK READER FROM STDIN OR FILE

	rdr := eutils.CreateXMLStreamer(in, nil)
	if rdr == nil {
		eutils.DisplayError("Unable to create XML Block Reader")
		os.Exit(1)
	}

	// FUSE SUBSETS OF INVERTED INDEX FILES

	// -fuse combines subsets of inverted files for subsequent -merge operation
	if fuse {

		// environment variable can override garbage collector (undocumented)
		gcEnv := os.Getenv("EDIRECT_FUSE_GOGC")
		if gcEnv != "" {
			val, err := strconv.Atoi(gcEnv)
			if err == nil {
				if val >= 50 && val <= 1000 {
					debug.SetGCPercent(val)
				} else {
					debug.SetGCPercent(100)
				}
			}
		} else if gcdefault {
			// default to 100 for fuse and merge
			debug.SetGCPercent(100)
		}

		// environment variable can override number of servers (undocumented)
		svEnv := os.Getenv("EDIRECT_FUSE_SERV")
		if svEnv != "" {
			val, err := strconv.Atoi(svEnv)
			if err == nil {
				if val >= 1 && val <= 128 {
					numServe = val
				} else {
					numServe = 1
				}
				eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)
			}
		}

		chns := eutils.CreateXMLProducer("InvDocument", "", false, rdr)
		fusr := eutils.CreateFusers(chns)
		mrgr := eutils.CreateMergers(fusr)
		unsq := eutils.CreateXMLUnshuffler(mrgr)

		if chns == nil || fusr == nil || mrgr == nil || unsq == nil {
			eutils.DisplayError("Unable to create inverted index fuser")
			os.Exit(1)
		}

		var out io.Writer

		out = os.Stdout

		if zipp {

			zpr, err := pgzip.NewWriterLevel(out, pgzip.BestSpeed)
			if err != nil {
				eutils.DisplayError("Unable to create compressor")
				os.Exit(1)
			}

			// close decompressor when all records have been processed
			defer zpr.Close()

			// use compressor for writing file
			out = zpr
		}

		// create buffered writer layer
		wrtr := bufio.NewWriter(out)

		wrtr.WriteString("<InvDocumentSet>\n")

		// drain channel of alphabetized results
		for curr := range unsq {

			str := curr.Text

			if str == "" {
				continue
			}

			// send result to output
			wrtr.WriteString(str)

			recordCount++
			runtime.Gosched()
		}

		wrtr.WriteString("</InvDocumentSet>\n\n")

		wrtr.Flush()

		debug.FreeOSMemory()

		if timr {
			printDuration("terms")
		}

		return
	}

	// ENSURE PRESENCE OF PATTERN ARGUMENT

	if len(args) < 1 {
		eutils.DisplayError("Insufficient command-line arguments supplied to rchive")
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

	// REPORT RECORDS THAT CONTAIN DAMAGED EMBEDDED HTML TAGS

	reportEncodedMarkup := func(typ, id, str string) {

		var buffer strings.Builder

		max := len(str)

		lookAhead := func(txt string, to int) string {

			mx := len(txt)
			if to > mx {
				to = mx
			}
			pos := strings.Index(txt[:to], "gt;")
			if pos > 0 {
				to = pos + 3
			}
			return txt[:to]
		}

		findContext := func(fr, to int) string {

			numSpaces := 0

			for fr > 0 {
				ch := str[fr]
				if ch == ' ' {
					numSpaces++
					if numSpaces > 1 {
						fr++
						break
					}
				} else if ch == '\n' || ch == '>' {
					fr++
					break
				}
				fr--
			}

			numSpaces = 0

			for to < max {
				ch := str[to]
				if ch == ' ' {
					numSpaces++
					if numSpaces > 1 {
						break
					}
				} else if ch == '\n' || ch == '<' {
					break
				}
				to++
			}

			return str[fr:to]
		}

		reportMarkup := func(lbl string, fr, to int, txt string) {

			if lbl == typ || typ == "ALL" {
				// extract XML of SELF, SINGLE, DOUBLE, or AMPER types, or ALL
				buffer.WriteString(str)
				buffer.WriteString("\n")
			} else if typ == "" {
				// print report
				buffer.WriteString(id)
				buffer.WriteString("\t")
				buffer.WriteString(lbl)
				buffer.WriteString("\t")
				buffer.WriteString(txt)
				buffer.WriteString("\t| ")
				ctx := findContext(fr, to)
				buffer.WriteString(ctx)
				if eutils.HasUnicodeMarkup(ctx) {
					ctx = eutils.RepairUnicodeMarkup(ctx, eutils.SPACE)
				}
				ctx = eutils.RepairEncodedMarkup(ctx)
				buffer.WriteString("\t| ")
				buffer.WriteString(ctx)
				if eutils.HasAmpOrNotASCII(ctx) {
					ctx = html.UnescapeString(ctx)
				}
				buffer.WriteString("\t| ")
				buffer.WriteString(ctx)
				buffer.WriteString("\n")
			}
		}

		/*
			badTags := [10]string{
				"<i/>",
				"<i />",
				"<b/>",
				"<b />",
				"<u/>",
				"<u />",
				"<sup/>",
				"<sup />",
				"<sub/>",
				"<sub />",
			}
		*/

		skip := 0

		/*
			var prev rune
		*/

		for i, ch := range str {
			if skip > 0 {
				skip--
				continue
			}
			/*
				if ch > 127 {
					if IsUnicodeSuper(ch) {
						if IsUnicodeSubsc(prev) {
							// reportMarkup("UNIUP", i, i+2, string(ch))
						}
					} else if IsUnicodeSubsc(ch) {
						if IsUnicodeSuper(prev) {
							// reportMarkup("UNIDN", i, i+2, string(ch))
						}
					} else if ch == '\u0038' || ch == '\u0039' {
						// reportMarkup("ANGLE", i, i+2, string(ch))
					}
					prev = ch
					continue
				} else {
					prev = ' '
				}
			*/
			if ch == '<' {
				/*
					j := i + 1
					if j < max {
						nxt := str[j]
						if nxt == 'i' || nxt == 'b' || nxt == 'u' || nxt == 's' {
							for _, tag := range badTags {
								if strings.HasPrefix(str, tag) {
									k := len(tag)
									reportMarkup("SELF", i, i+k, tag)
									break
								}
							}
						}
					}
					if strings.HasPrefix(str[i:], "</sup><sub>") {
						// reportMarkup("SUPSUB", i, i+11, "</sup><sub>")
					} else if strings.HasPrefix(str[i:], "</sub><sup>") {
						// reportMarkup("SUBSUP", i, i+11, "</sub><sup>")
					}
				*/
				continue
			} else if ch != '&' {
				continue
			} else if strings.HasPrefix(str[i:], "&lt;") {
				sub := lookAhead(str[i:], 14)
				_, ok := eutils.HTMLRepair(sub)
				if ok {
					skip = len(sub) - 1
					reportMarkup("SINGLE", i, i+skip+1, sub)
					continue
				}
			} else if strings.HasPrefix(str[i:], "&amp;lt;") {
				sub := lookAhead(str[i:], 22)
				_, ok := eutils.HTMLRepair(sub)
				if ok {
					skip = len(sub) - 1
					reportMarkup("DOUBLE", i, i+skip+1, sub)
					continue
				}
			} else if strings.HasPrefix(str[i:], "&amp;amp;") {
				reportMarkup("AMPER", i, i+9, "&amp;amp;")
				skip = 8
				continue
			}
		}

		res := buffer.String()

		os.Stdout.WriteString(res)
	}

	// -damaged plus -index plus -pattern reports records with multiply-encoded HTML tags
	if dmgd && indx != "" {

		find := eutils.ParseIndex(indx)

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				id := eutils.FindIdentifier(str[:], parent, find)
				if id == "" {
					return
				}

				// remove default version suffix
				if strings.HasSuffix(id, ".1") {
					idlen := len(id)
					id = id[:idlen-2]
				}

				reportEncodedMarkup(dmgdType, id, str)
			})

		if timr {
			printDuration("records")
		}

		return
	}

	// COMPARE XML UPDATES TO LOCAL DIRECTORY, RETAIN NEW OR SUBSTANTIVELY CHANGED RECORDS

	// -prepare plus -archive plus -index plus -pattern compares XML files against stash
	if stsh != "" && indx != "" && cmpr {

		doReport := false
		if cmprType == "" || cmprType == "report" {
			doReport = true
		} else if cmprType != "release" {
			eutils.DisplayError("-prepare argument must be release or report")
			os.Exit(1)
		}

		find := eutils.ParseIndex(indx)

		if head != "" {
			os.Stdout.WriteString(head)
			os.Stdout.WriteString("\n")
		}

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				id := eutils.FindIdentifier(str[:], parent, find)
				if id == "" {
					return
				}

				pos := strings.Index(id, ".")
				if pos >= 0 {
					// remove version suffix
					id = id[:pos]
				}

				dir, file := eutils.ArchiveTrie(id)

				if dir == "" || file == "" {
					return
				}

				fpath := filepath.Join(stsh, dir, file+".xml")
				if fpath == "" {
					return
				}

				// print new or updated XML record
				printRecord := func(stn string, isNew bool) {

					if stn == "" {
						return
					}

					if doReport {
						if isNew {
							os.Stdout.WriteString("NW ")
							os.Stdout.WriteString(id)
							os.Stdout.WriteString("\n")
						} else {
							os.Stdout.WriteString("UP ")
							os.Stdout.WriteString(id)
							os.Stdout.WriteString("\n")
						}
						return
					}

					if hd != "" {
						os.Stdout.WriteString(hd)
						os.Stdout.WriteString("\n")
					}

					os.Stdout.WriteString(stn)
					os.Stdout.WriteString("\n")

					if tl != "" {
						os.Stdout.WriteString(tl)
						os.Stdout.WriteString("\n")
					}
				}

				_, err := os.Stat(fpath)
				if err != nil && os.IsNotExist(err) {
					// new record
					printRecord(str, true)
					return
				}
				if err != nil {
					return
				}

				buf, err := ioutil.ReadFile(fpath)
				if err != nil {
					return
				}

				txt := string(buf[:])
				txt = strings.TrimSuffix(txt, "\n")

				// check for optional -ignore argument
				if ignr != "" {

					// ignore differences inside specified object
					ltag := "<" + ignr + ">"
					sleft, _ := eutils.SplitInTwoLeft(str, ltag)
					tleft, _ := eutils.SplitInTwoLeft(txt, ltag)

					rtag := "</" + ignr + ">"
					_, srght := eutils.SplitInTwoRight(str, rtag)
					_, trght := eutils.SplitInTwoRight(txt, rtag)

					if sleft == tleft && srght == trght {
						if doReport {
							os.Stdout.WriteString("NO ")
							os.Stdout.WriteString(id)
							os.Stdout.WriteString("\n")
						}
						return
					}

				} else {

					// compare entirety of objects
					if str == txt {
						if doReport {
							os.Stdout.WriteString("NO ")
							os.Stdout.WriteString(id)
							os.Stdout.WriteString("\n")
						}
						return
					}
				}

				// substantively modified record
				printRecord(str, false)
			})

		if tail != "" {
			os.Stdout.WriteString(tail)
			os.Stdout.WriteString("\n")
		}

		if timr {
			printDuration("records")
		}

		return
	}

	// SAVE XML COMPONENT RECORDS TO LOCAL DIRECTORY INDEXED BY TRIE ON IDENTIFIER

	// common xml + DOCTYPE header for PubmedArticle XML
	pmaSetHead := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE PubmedArticle PUBLIC "-//NLM//DTD PubMedArticle, 1st January 2019//EN" "https://dtd.nlm.nih.gov/ncbi/pubmed/out/pubmed_190101.dtd">
`

	// common xml without DOCTYPE header for PMCInfo XML
	pmcSetHead := `<?xml version="1.0" encoding="UTF-8"?>
`

	// -archive plus -index plus -pattern saves XML files in trie-based directory structure
	if stsh != "" && indx != "" {

		asn := false
		pfx := ""
		sfx := ".xml"
		xmlString := ""
		report := 1000

		if db == "pmc" {
			pfx = "PMC"
			xmlString = pmcSetHead
		} else if db == "pubmed" {
			xmlString = pmaSetHead
		} else if db == "taxonomy" {
			report = 50000
		}

		if pma2pme {
			asn = true
			sfx = ".asn"
		}

		xmlq := eutils.CreateXMLProducer(topPattern, star, false, rdr)
		stsq := eutils.CreateStashers(stsh, parent, indx, pfx, sfx, db, xmlString, transform, hshv, zipp, asn, report, xmlq)
		clrq := eutils.CreateClearer(idcs, incr, db, stsq)

		if xmlq == nil || stsq == nil || clrq == nil {
			eutils.DisplayError("Unable to create stash generator")
			os.Exit(1)
		}

		if hshv {
			// if printing hash, do not clear incremental indices, just pass hash value
			clrq = stsq
		}

		// drain output channel
		for str := range clrq {

			if hshv {
				// print table of UIDs and hash values
				os.Stdout.WriteString(str)
			}

			if dbug {
				fmt.Fprintf(os.Stderr, "%s\n", str)
			}

			recordCount++
			runtime.Gosched()
		}

		debug.FreeOSMemory()

		if timr {
			printDuration("records")
		}

		return
	}

	// READ FILE OF IDENTIFIERS AND EXTRACT SELECTED RECORDS FROM XML INPUT FILE

	// -index plus -unique [plus -head/-tail/-hd/-tl] plus -pattern with no other extraction arguments
	// takes an XML input file and a file of its UIDs and keeps only the last version of each record
	if indx != "" && unqe != "" && len(args) == 2 {

		// read file of identifiers to use for filtering
		fl, err := os.Open(unqe)
		if err != nil {
			eutils.DisplayError("Unable to open identifier file '%s'", unqe)
			os.Exit(1)
		}

		// create map that counts instances of each UID
		order := make(map[string]int)

		scanr := bufio.NewScanner(fl)

		// read lines of identifiers
		for scanr.Scan() {

			id := scanr.Text()

			// map records count for given identifier
			val := order[id]
			val++
			order[id] = val
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
				if id == "" {
					return
				}

				val, ok := order[id]
				if !ok {
					// not in identifier list, skip
					return
				}
				// decrement count in map
				val--
				order[id] = val
				if val > 0 {
					// only write last record with a given identifier
					return
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

		if timr {
			printDuration("records")
		}

		return
	}

	// GENERATE RECORD INDEX ON XML INPUT FILE

	// -index plus -pattern prints record identifier and XML size
	if indx != "" {

		lbl := ""
		// check for optional filename label after -pattern argument (undocumented)
		if len(args) > 3 && args[2] == "-lbl" {
			lbl = args[3]

			lbl = strings.TrimSpace(lbl)
			if strings.HasPrefix(lbl, "pubmed") {
				lbl = lbl[7:]
			}
			if strings.HasSuffix(lbl, ".xml.gz") {
				xlen := len(lbl)
				lbl = lbl[:xlen-7]
			}
			lbl = strings.TrimSpace(lbl)
		}

		// legend := "ID\tREC\tSIZE"

		find := eutils.ParseIndex(indx)

		eutils.PartitionXML(topPattern, star, false, rdr,
			func(str string) {
				recordCount++

				id := eutils.FindIdentifier(str[:], parent, find)
				if id == "" {
					return
				}
				if lbl != "" {
					fmt.Printf("%s\t%d\t%s\n", id, len(str), lbl)
				} else {
					fmt.Printf("%s\t%d\n", id, len(str))
				}
			})

		if timr {
			printDuration("records")
		}

		return
	}

	// SORT XML RECORDS BY IDENTIFIER

	// -pattern record_name -sort parent/element@attribute^version, strictly alphabetic sort order (undocumented)
	if len(args) == 4 && args[2] == "-sort" {

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

		// sort fields in alphabetical order, unlike xtract version, which sorts numbers by numeric value
		keys := slices.Sorted(maps.Keys(order))

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

	// REPORT UNRECOGNIZED COMMAND

	eutils.DisplayError("Unrecognized rchive command")

	for _, str := range origArgs {
		fmt.Fprintf(os.Stderr, "%s\n", str)
	}
	fmt.Fprintf(os.Stderr, "\n")

	os.Exit(1)
}
