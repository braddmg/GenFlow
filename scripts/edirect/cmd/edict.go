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
// File Name:  edict.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package main

import (
	"eutils"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// network server for EDirect local PubMed archive and search system

// run "go run edict.go" to start local server listening on port 8080,
// or run "go build edict.go" to generate "edict" binary executable

// ignore warnings printed when server is started (for now)

// nquire -edict is a convenience shortcut for -url "localhost:8080"

// export NQUIRE_EDICT_SERVER to override nquire -edict address when
// connecting to a remote server instance

var edictHelp = `
PubMed Local Archive Term Queries

 Query evaluation includes Boolean operations and parenthetical expressions:

  nquire -edict search -query "(literacy AND numeracy) NOT (adolescent OR child)"

 Adjacent words in the query are treated as a contiguous phrase:

  nquire -edict search -query "selective serotonin reuptake inhibitor [TITL]"

 Terms are truncated with trailing asterisks:

  nquire -edict search -query "Cozzarelli N* [AUTH] AND 2000:2005 [YEAR]"

 Each plus sign will replace a single word inside a phrase:

  nquire -edict search -query "vitamin c + + common cold"

 Runs of tildes indicate the maximum distance between sequential phrases:

  nquire -edict search -query "vitamin c ~ ~ common cold"

PubMed Record Retrieval

  nquire -edict fetch -id 6275390 13970600

Retrieval of Compressed Records for faster Network Transfer

  nquire -edict stream -id 6275390 13970600 | gunzip -c

Combined Query and Retrieval

  nquire -edict search -query "PNAS [JOUR]" |
  join-into-groups-of 1000 |
  xargs -n 1 nquire -edict fetch -id

PubmedArticleSet Wrappers

  (
    nquire -edict fetch head
    nquire -edict search -query "catabolite repress* [TIAB]" |
    join-into-groups-of 1000 |
    xargs -n 1 nquire -edict fetch -id
    nquire -edict fetch tail
  )

  (
    nquire -edict stream head
    nquire -edict search -query "catabolite repress* [TIAB]" |
    join-into-groups-of 1000 |
    xargs -n 1 nquire -edict stream -id
    nquire -edict stream tail
  ) | gunzip -c

Citation Matching

 From Command-Line Arguments:

  nquire -edict match \
    -author "Kans JA" -author "Casadaban MJ" \
    -title "nucleotide sequences required for tn3 transposition immunity" \
    -journal "J Bacteriol" -year 1989 -volume 171 -issue 4 -page 1904-14

 From GenBank Flatfile:

  efetch -db nuccore -id J01714 -format gb |
  gbf2ref | transmute -format compact | grep CITATION |
  grep -e published -e inpress | tr '\n' '\0' |
  xargs -0 -n 50 nquire -edict match -citation |
  xtract -pattern CITATION -def "-" -element ACCN REF PMID FAUT TITL

 From ASN.1 Sequence Record:

  efetch -db nuccore -id J01714 -format asn |
  asn2ref | transmute -format compact |
  grep CITATION | tr '\n' '\0' |
  xargs -0 -n 50 nquire -edict match -citation

Journal Name Lookup

  nquire -edict journal -query "biorxiv"

Documentation

  nquire -edict help

  nquire -edict version

`

var pmaSetHead = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE PubmedArticle PUBLIC "-//NLM//DTD PubMedArticle, 1st January 2019//EN" "https://dtd.nlm.nih.gov/ncbi/pubmed/out/pubmed_190101.dtd">
<PubmedArticleSet>
`

var pmaSetTail = `</PubmedArticleSet>
`

var streamContentType = "application/octet-stream"

func main() {

	// skip past executable name
	args := os.Args[1:]

	// default host and port set up for local test server
	host := "0.0.0.0"
	port := "8080"

	// HOST, PORT, AND CONCURRENCY FLAGS

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

	// process any arguments on the command line
	if len(args) > 0 {

		inSwitch := true

		// loop through arguments
		for {
			inSwitch = true

			switch args[0] {

			// host and port arguments
			case "-host":
				host = eutils.GetStringArg(args, "Host name")
				args = args[1:]
			case "-port":
				port = eutils.GetStringArg(args, "Port number")
				args = args[1:]

			// concurrency arguments
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

			default:
				// set flag to break out of for loop
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
	}

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

	eutils.SetTunings(numProcs, numServe, serverRatio, chanDepth, farmSize, heapSize, goGc, false)

	eutils.SetOptions(doStrict, doMixed, doSelf, deAccent, deSymbol, doASCII, doCompress, doCleanup, doStem, deStop)

	// DATA AVAILABILITY REALITY CHECKS

	// obtain path from environment variable
	base := os.Getenv("EDIRECT_PUBMED_MASTER")
	if base != "" {
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
	}
	if base == "" {
		eutils.DisplayError("Local archive path is not specified")
		os.Exit(1)
	}

	archiveBase := base + "Archive"
	postingsBase := base + "Postings"
	dataBase := base + "Data"

	// check to make sure local archive and search index directories are mounted
	_, err := os.Stat(archiveBase)
	if err != nil && os.IsNotExist(err) {
		eutils.DisplayError("Local archive is not mounted")
		os.Exit(1)
	}
	_, err = os.Stat(postingsBase)
	if err != nil && os.IsNotExist(err) {
		eutils.DisplayError("Local search index is not mounted")
		os.Exit(1)
	}
	_, err = os.Stat(dataBase)
	if err != nil && os.IsNotExist(err) {
		eutils.DisplayError("Local mapping data is not mounted")
		os.Exit(1)
	}

	// CREATE GIN ROUTER

	// create gin router with default middleware
	r := gin.Default()

	// PRINT HELP TEXT

	// nquire -get "localhost:8080/help"
	r.GET("/help", func(c *gin.Context) {
		c.String(http.StatusOK, edictHelp)
	})
	// nquire -url "localhost:8080/help"
	r.POST("/help", func(c *gin.Context) {
		c.String(http.StatusOK, edictHelp)
	})

	// PRINT VERSION NUMBER

	// nquire -get "localhost:8080/version"
	r.GET("/version", func(c *gin.Context) {
		c.String(http.StatusOK, eutils.EDirectVersion)
	})
	// nquire -url "localhost:8080/version"
	r.POST("version", func(c *gin.Context) {
		c.String(http.StatusOK, eutils.EDirectVersion)
	})

	// FETCH PUBMED ARTICLE SET WRAPPERS

	// nquire -get "localhost:8080/fetch/head"
	r.GET("/fetch/head", func(c *gin.Context) {
		c.String(http.StatusOK, pmaSetHead)
	})
	// nquire -url "localhost:8080/fetch/head"
	r.POST("/fetch/head", func(c *gin.Context) {
		c.String(http.StatusOK, pmaSetHead)
	})

	// nquire -get "localhost:8080/fetch/tail"
	r.GET("/fetch/tail", func(c *gin.Context) {
		c.String(http.StatusOK, pmaSetTail)
	})
	// nquire -url "localhost:8080/fetch/tail"
	r.POST("/fetch/tail", func(c *gin.Context) {
		c.String(http.StatusOK, pmaSetTail)
	})

	// STREAM COMPRESSED PUBMED ARTICLE SET WRAPPERS

	// make gzip-compressed byte arrays of pmaSetHead and pmaSetTail
	gzipHead := eutils.GzipString(pmaSetHead)
	gzipTail := eutils.GzipString(pmaSetTail)

	// nquire -get "localhost:8080/stream/head"
	r.GET("/stream/head", func(c *gin.Context) {
		c.Data(http.StatusOK, streamContentType, gzipHead)
	})
	// nquire -url "localhost:8080/stream/head"
	r.POST("/stream/head", func(c *gin.Context) {
		c.Data(http.StatusOK, streamContentType, gzipHead)
	})

	// nquire -get "localhost:8080/stream/tail"
	r.GET("/stream/tail", func(c *gin.Context) {
		c.Data(http.StatusOK, streamContentType, gzipTail)
	})
	// nquire -url "localhost:8080/stream/tail"
	r.POST("/stream/tail", func(c *gin.Context) {
		c.Data(http.StatusOK, streamContentType, gzipTail)
	})

	// PUBMED XML RECORD RETRIEVAL BY PMID

	// common fetch function
	pubmedFetch := func(c *gin.Context, uids string, tbo string) {

		// concurrent fetching by multiple goroutines
		uidq := eutils.ReadsUIDsFromString(uids)
		strq := eutils.CreateFetchers(archiveBase, "pubmed", "", ".xml", "PubmedArticle", true, uidq)
		unsq := eutils.CreateXMLUnshuffler(strq)

		if uidq == nil || strq == nil || unsq == nil {
			eutils.DisplayError("Unable to create archive reader")
			os.Exit(1)
		}

		turbo := false
		// look for "-turbo true" argument
		if tbo == "true" {
			turbo = true
		}

		retlength := len("\n")

		// drain output channel
		for curr := range unsq {

			str := curr.Text

			if str == "" {
				continue
			}

			newln := false
			if !strings.HasSuffix(str, "\n") {
				newln = true
			}

			if turbo {
				nxt := len(str)
				if newln {
					nxt += retlength
				}
				val := strconv.Itoa(nxt)
				c.String(http.StatusOK, "<NEXT_RECORD_SIZE>"+val+"</NEXT_RECORD_SIZE>\n")
			}

			if newln {
				c.String(http.StatusOK, str+"\n")
			} else {
				c.String(http.StatusOK, str)
			}
		}
	}

	// nquire -get "localhost:8080/fetch" -id "2539356,1937004"
	r.GET("/fetch", func(c *gin.Context) {
		uids := c.Query("id")
		tbo := c.Query("turbo")
		pubmedFetch(c, uids, tbo)
	})
	// nquire -url "localhost:8080/fetch" -id "2539356,1937004"
	r.POST("/fetch", func(c *gin.Context) {
		uids := c.PostForm("id")
		tbo := c.PostForm("turbo")
		pubmedFetch(c, uids, tbo)
	})

	// nquire -get "localhost:8080/fetch/2539356,1937004"
	r.GET("/fetch/:id", func(c *gin.Context) {
		uids := c.Param("id")
		tbo := c.Param("turbo")
		pubmedFetch(c, uids, tbo)
	})
	// nquire -url "localhost:8080/fetch/2539356,1937004"
	r.POST("/fetch/:id", func(c *gin.Context) {
		uids := c.Param("id")
		tbo := c.Param("turbo")
		pubmedFetch(c, uids, tbo)
	})

	// COMPRESSED PUBMED XML RECORD STREAMING BY PMID

	// common stream function
	pubmedStream := func(c *gin.Context, uids string) {

		// concurrent fetching by multiple goroutines
		uidq := eutils.ReadsUIDsFromString(uids)
		strq := eutils.CreateCacheStreamers(archiveBase, "pubmed", "", ".xml", 0, uidq)
		unsq := eutils.CreateXMLUnshuffler(strq)

		if uidq == nil || strq == nil || unsq == nil {
			eutils.DisplayError("Unable to create archive steamer")
			os.Exit(1)
		}

		// drain output channel
		for curr := range unsq {

			data := curr.Data

			if data == nil {
				continue
			}

			c.Data(http.StatusOK, streamContentType, data)
		}
	}

	// nquire -get "localhost:8080/stream" -id "2539356,1937004"
	r.GET("/stream", func(c *gin.Context) {
		uids := c.Query("id")
		pubmedStream(c, uids)
	})
	// nquire -url "localhost:8080/stream" -id "2539356,1937004"
	r.POST("/stream", func(c *gin.Context) {
		uids := c.PostForm("id")
		pubmedStream(c, uids)
	})

	// nquire -get "localhost:8080/stream/2539356,1937004"
	r.GET("/stream/:id", func(c *gin.Context) {
		uids := c.Param("id")
		pubmedStream(c, uids)
	})
	// nquire -url "localhost:8080/stream/2539356,1937004"
	r.POST("/stream/:id", func(c *gin.Context) {
		uids := c.Param("id")
		pubmedStream(c, uids)
	})

	// PMID LOOKUP FROM PUBMED PHRASE AND INDEXED FIELD SEARCH

	// common search function
	pubmedSearch := func(c *gin.Context, query string) {

		uids := eutils.ProcessQuery("pubmed", query, false, false, false, deStop)

		// use buffer to speed up uid printing
		var buffer strings.Builder

		for _, uid := range uids {
			val := strconv.Itoa(int(uid))
			buffer.WriteString(val[:])
			buffer.WriteString("\n")
		}

		txt := buffer.String()
		if txt != "" {
			// print buffer
			c.String(http.StatusOK, txt)
		}
	}

	// nquire -get "localhost:8080/search" -query "tn3 transposition immunity [TIAB] AND 1988:1993 [YEAR]"
	r.GET("/search", func(c *gin.Context) {
		query := c.Query("query")
		pubmedSearch(c, query)
	})
	// nquire -url "localhost:8080/search" -query "(literacy AND numeracy) NOT (adolescent OR child)"
	r.POST("/search", func(c *gin.Context) {
		query := c.PostForm("query")
		pubmedSearch(c, query)
	})

	// POPULATE JOURNAL TITLE LOOKUP MAP

	jtaMap := make(map[string]string)

	jpath := filepath.Join(dataBase, "joursets.txt")
	eutils.TableToMap(jpath, jtaMap)

	// PMID LOOKUP CACHE

	cache := eutils.NewCitCache(500)

	if cache == nil {
		eutils.DisplayError("Unable to create citation matcher cache")
		os.Exit(1)
	}

	// PMID CACHE PRELOAD

	preloadCache := func(c *gin.Context, fileName string) {

		if fileName == "" {
			return
		}

		eutils.PreloadCitCache(fileName, cache)

		c.String(http.StatusOK, "")
	}

	// nquire -get "localhost:8080/preload" -file ...
	r.GET("/preload", func(c *gin.Context) {
		fileName := c.Query("file")
		preloadCache(c, fileName)
	})
	// nquire -url "localhost:8080/preload" -file ...
	r.POST("/preload", func(c *gin.Context) {
		fileName := c.PostForm("file")
		preloadCache(c, fileName)
	})

	// PMID LOOKUP BY CITATION MATCHING

	// common match function
	citMatch := func(c *gin.Context, params map[string][]string) {

		if params == nil {
			return
		}

		buildCitation := func(params map[string][]string) string {

			var arry []string

			arry = append(arry, "<CITATION>")

			addItems := func(names []string, tags []string) {
				for _, name := range names {
					vals, ok := params[name]
					if ok {
						for n, tag := range tags {
							if len(vals) > n {
								// only keep first page
								if tag == "PAGE" {
									vals[n], _ = eutils.SplitInTwoLeft(vals[n], "-")
								}
								arry = append(arry, "<"+tag+">"+vals[n]+"</"+tag+">")
							}
						}
					}
				}
			}

			// allow flexibility in argument names
			addItems([]string{"author", "auth"}, []string{"FAUT", "LAUT"})
			addItems([]string{"faut"}, []string{"FAUT"})
			addItems([]string{"laut"}, []string{"LAUT"})
			addItems([]string{"title", "titl"}, []string{"TITL"})
			addItems([]string{"journal", "jour"}, []string{"JOUR"})
			addItems([]string{"volume", "vol"}, []string{"VOL"})
			addItems([]string{"issue", "iss"}, []string{"ISS"})
			addItems([]string{"pages", "page"}, []string{"PAGE"})
			addItems([]string{"year"}, []string{"YEAR"})

			arry = append(arry, "</CITATION>")

			cit := strings.Join(arry, "")

			return cit
		}

		cit := ""
		isCitationXML := false

		// check for -citation "<CITATION> .. </CITATION>" argument
		ctn, ok := params["citation"]
		if ok && len(ctn) > 0 {
			cit = ctn[0]
			isCitationXML = true
		} else {
			// otherwise read individual -author, -title, -journal, -year, -volume, -issue, and -page arguments
			cit = buildCitation(params)
		}

		sgr := strings.NewReader(cit)
		rdr := eutils.CreateXMLStreamer(sgr, nil)
		xmlq := eutils.CreateXMLProducer("CITATION", "", false, rdr)
		ctmq := eutils.CreateCitMatchers(xmlq, []string{"strict,remote,verify"}, deStop, doStem, cache, jtaMap)
		unsq := eutils.CreateXMLUnshuffler(ctmq)

		if sgr == nil || rdr == nil || xmlq == nil || ctmq == nil || unsq == nil {
			eutils.DisplayError("Unable to create citation matcher")
			os.Exit(1)
		}

		// drain output channel
		for curr := range unsq {

			mtch := curr.Text

			if mtch == "" {
				continue
			}

			// extract value in new <PMID> .. </PMID> object
			_, after, found := strings.Cut(mtch, "<PMID>")
			if !found {
				continue
			}
			before, _, found := strings.Cut(after, "</PMID>")
			if !found {
				continue
			}
			if before == "" {
				continue
			}

			// send result to output
			if isCitationXML {
				c.String(http.StatusOK, mtch+"\n")
			} else {
				c.String(http.StatusOK, before+"\n")
			}
		}
	}

	// nquire -get "localhost:8080/match" -author fst -author lst -title ttl -journal jta -year yr
	r.GET("/match", func(c *gin.Context) {
		paramPairs := c.Request.URL.Query()
		citMatch(c, paramPairs)
	})
	// nquire -url "localhost:8080/match" -author fst -author lst -title ttl -journal jta -year yr
	r.POST("/match", func(c *gin.Context) {
		c.MultipartForm()
		citMatch(c, c.Request.PostForm)
	})

	// JOURNAL LOOKUP FROM JOURNAL TO INDEX MAP

	// journal lookup from jtaMap
	lookupJournal := func(c *gin.Context, query string) {

		query = eutils.NormalizeJournal(query)
		if query != "" {
			query = strings.ToLower(query)
			jta, ok := jtaMap[query]
			if ok && jta != "" {
				c.String(http.StatusOK, jta+"\n")
			}
		}
	}

	// nquire -get "localhost:8080/journal" -query "journal of immunology"
	r.GET("/journal", func(c *gin.Context) {
		query := c.Query("query")
		lookupJournal(c, query)
	})
	// nquire -url "localhost:8080/journal" -query "pnas"
	r.POST("/journal", func(c *gin.Context) {
		query := c.PostForm("query")
		lookupJournal(c, query)
	})

	// START LISTENING ON PORT

	// listen for requests
	r.Run(host + ":" + port)
}
