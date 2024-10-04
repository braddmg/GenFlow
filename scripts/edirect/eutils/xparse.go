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
// File Name:  xparse.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// SUPPORT CODE FOR xtract PARSING AND OBJECT PRINTING

// TYPED CONSTANTS

// LevelType is the integer type for exploration arguments
type LevelType int

// LevelType keys for exploration arguments
const (
	_ LevelType = iota
	UNIT
	SUBSETX
	SUBSET
	SECTIONX
	SECTION
	BLOCKX
	BLOCK
	BRANCHX
	BRANCH
	GROUPX
	GROUP
	DIVISIONX
	DIVISION
	PATHX
	PATH
	PATTERN
)

// IndentType is the integer type for XML formatting
type IndentType int

// IndentType keys for XML formatting
const (
	SINGULARITY IndentType = iota
	COMPACT
	FLUSH
	INDENT
	SUBTREE
	WRAPPED
)

// OpType is the integer type for operations
type OpType int

// OpType keys for operations
const (
	UNSET OpType = iota
	ELEMENT
	FIRST
	LAST
	EVEN
	ODD
	BACKWARD
	ENCODE
	DECODE
	UPPER
	LOWER
	CHAIN
	TITLE
	MIRROR
	ALPHA
	ALNUM
	BASIC
	PLAIN
	SIMPLE
	AUTHOR
	PROSE
	JOURNAL
	YEAR
	MONTH
	DATE
	PAGE
	AUTH
	INITIALS
	PROP
	TRIM
	WCT
	DOI
	TRANSLATE
	REPLACE
	TERMS
	WORDS
	PAIRS
	PAIRX
	SPLIT
	ORDER
	REVERSE
	LETTERS
	PENTAMERS
	CLAUSES
	INDEXER
	MESHCODE
	MATRIX
	CLASSIFY
	HISTOGRAM
	FREQUENCY
	ACCENTED
	TEST
	SCAN
	PFX
	SFX
	SEP
	TAB
	RET
	LBL
	TAG
	ATT
	ATR
	CLS
	SLF
	END
	CLR
	PFC
	DEQ
	PLG
	ELG
	FWD
	AWD
	WRP
	BKT
	ENC
	PKG
	RST
	DEF
	MSK
	REG
	EXP
	WITH
	GCODE
	FRAME0
	FRAME1
	COLOR
	POSITION
	SELECT
	IF
	UNLESS
	MATCH
	AVOID
	AND
	OR
	EQUALS
	CONTAINS
	INCLUDES
	EXCLUDES
	ISWITHIN
	STARTSWITH
	ENDSWITH
	ISNOT
	ISBEFORE
	ISAFTER
	MATCHES
	RESEMBLES
	ISEQUALTO
	DIFFERSFROM
	GT
	GE
	LT
	LE
	EQ
	NE
	NUM
	LEN
	SUM
	ACC
	MIN
	MAX
	INC
	DEC
	SUB
	AVG
	DEV
	MED
	MUL
	DIV
	MOD
	GEO
	HRM
	RMS
	SQT
	LG2
	LGE
	LOG
	BIN
	OCT
	HEX
	BIT
	PAD
	RAW
	ZEROBASED
	ONEBASED
	UCSCBASED
	NUCLEIC
	REVCOMP
	FASTA
	NCBI2NA
	NCBI4NA
	CDS2PROT
	PEPTS
	MOLWTX
	MOLWTM
	MOLWTF
	ACCESSION
	NUMERIC
	HGVS
	ELSE
	VARIABLE
	ACCUMULATOR
	CONVERTER
	VALUE
	QUESTION
	TILDE
	STAR
	DOT
	PRCNT
	DOLLAR
	ATSIGN
	COUNT
	LENGTH
	DEPTH
	INDEX
	INCR
	DECR
	UNRECOGNIZED
)

// ArgumentType is the integer type for argument classification
type ArgumentType int

// ArgumentType keys for argument classification
const (
	_ ArgumentType = iota
	EXPLORATION
	CONDITIONAL
	EXTRACTION
	CUSTOMIZATION
)

// RangeType is the integer type for element range choices
type RangeType int

// RangeType keys for element range choices
const (
	NORANGE RangeType = iota
	STRINGRANGE
	VARIABLERANGE
	INTEGERRANGE
)

// SeqEndType is used for -ucsc-based decisions
type SeqEndType int

// SeqEndType keys for -ucsc-based decisions
const (
	_ SeqEndType = iota
	ISSTART
	ISSTOP
	ISPOS
)

// SequenceType is used to record XML tag and position for -ucsc-based
type SequenceType struct {
	Based int
	Which SeqEndType
}

// MUTEXES

var hlock sync.Mutex

var slock sync.RWMutex

// ARGUMENT MAPS

var argTypeIs = map[string]ArgumentType{
	"-unit":         EXPLORATION,
	"-Unit":         EXPLORATION,
	"-subsetX":      EXPLORATION,
	"-SubsetX":      EXPLORATION,
	"-subset":       EXPLORATION,
	"-Subset":       EXPLORATION,
	"-sectionX":     EXPLORATION,
	"-SectionX":     EXPLORATION,
	"-section":      EXPLORATION,
	"-Section":      EXPLORATION,
	"-blockX":       EXPLORATION,
	"-BlockX":       EXPLORATION,
	"-block":        EXPLORATION,
	"-Block":        EXPLORATION,
	"-branchX":      EXPLORATION,
	"-BranchX":      EXPLORATION,
	"-branch":       EXPLORATION,
	"-Branch":       EXPLORATION,
	"-groupX":       EXPLORATION,
	"-GroupX":       EXPLORATION,
	"-group":        EXPLORATION,
	"-Group":        EXPLORATION,
	"-divisionX":    EXPLORATION,
	"-DivisionX":    EXPLORATION,
	"-division":     EXPLORATION,
	"-Division":     EXPLORATION,
	"-pathX":        EXPLORATION,
	"-PathX":        EXPLORATION,
	"-path":         EXPLORATION,
	"-Path":         EXPLORATION,
	"-pattern":      EXPLORATION,
	"-Pattern":      EXPLORATION,
	"-position":     CONDITIONAL,
	"-select":       CONDITIONAL,
	"-if":           CONDITIONAL,
	"-unless":       CONDITIONAL,
	"-match":        CONDITIONAL,
	"-avoid":        CONDITIONAL,
	"-and":          CONDITIONAL,
	"-or":           CONDITIONAL,
	"-equals":       CONDITIONAL,
	"-contains":     CONDITIONAL,
	"-includes":     CONDITIONAL,
	"-excludes":     CONDITIONAL,
	"-is-within":    CONDITIONAL,
	"-starts-with":  CONDITIONAL,
	"-ends-with":    CONDITIONAL,
	"-is-not":       CONDITIONAL,
	"-is-before":    CONDITIONAL,
	"-is-after":     CONDITIONAL,
	"-matches":      CONDITIONAL,
	"-resembles":    CONDITIONAL,
	"-is-equal-to":  CONDITIONAL,
	"-differs-from": CONDITIONAL,
	"-gt":           CONDITIONAL,
	"-ge":           CONDITIONAL,
	"-lt":           CONDITIONAL,
	"-le":           CONDITIONAL,
	"-eq":           CONDITIONAL,
	"-ne":           CONDITIONAL,
	"-element":      EXTRACTION,
	"-first":        EXTRACTION,
	"-last":         EXTRACTION,
	"-even":         EXTRACTION,
	"-odd":          EXTRACTION,
	"-backward":     EXTRACTION,
	"-encode":       EXTRACTION,
	"-decode":       EXTRACTION,
	"-upper":        EXTRACTION,
	"-lower":        EXTRACTION,
	"-chain":        EXTRACTION,
	"-title":        EXTRACTION,
	"-mirror":       EXTRACTION,
	"-alpha":        EXTRACTION,
	"-alnum":        EXTRACTION,
	"-basic":        EXTRACTION,
	"-plain":        EXTRACTION,
	"-simple":       EXTRACTION,
	"-author":       EXTRACTION,
	"-prose":        EXTRACTION,
	"-jour":         EXTRACTION,
	"-journal":      EXTRACTION,
	"-year":         EXTRACTION,
	"-month":        EXTRACTION,
	"-date":         EXTRACTION,
	"-page":         EXTRACTION,
	"-auth":         EXTRACTION,
	"-initials":     EXTRACTION,
	"-prop":         EXTRACTION,
	"-trim":         EXTRACTION,
	"-wct":          EXTRACTION,
	"-doi":          EXTRACTION,
	"-translate":    EXTRACTION,
	"-replace":      EXTRACTION,
	"-terms":        EXTRACTION,
	"-words":        EXTRACTION,
	"-pairs":        EXTRACTION,
	"-pairx":        EXTRACTION,
	"-split":        EXTRACTION,
	"-order":        EXTRACTION,
	"-reverse":      EXTRACTION,
	"-letters":      EXTRACTION,
	"-pentamers":    EXTRACTION,
	"-clauses":      EXTRACTION,
	"-indexer":      EXTRACTION,
	"-meshcode":     EXTRACTION,
	"-matrix":       EXTRACTION,
	"-classify":     EXTRACTION,
	"-histogram":    EXTRACTION,
	"-frequency":    EXTRACTION,
	"-accented":     EXTRACTION,
	"-test":         EXTRACTION,
	"-scan":         EXTRACTION,
	"-num":          EXTRACTION,
	"-len":          EXTRACTION,
	"-sum":          EXTRACTION,
	"-acc":          EXTRACTION,
	"-min":          EXTRACTION,
	"-max":          EXTRACTION,
	"-inc":          EXTRACTION,
	"-dec":          EXTRACTION,
	"-sub":          EXTRACTION,
	"-avg":          EXTRACTION,
	"-dev":          EXTRACTION,
	"-med":          EXTRACTION,
	"-mul":          EXTRACTION,
	"-div":          EXTRACTION,
	"-mod":          EXTRACTION,
	"-geo":          EXTRACTION,
	"-hrm":          EXTRACTION,
	"-rms":          EXTRACTION,
	"-sqt":          EXTRACTION,
	"-lg2":          EXTRACTION,
	"-lge":          EXTRACTION,
	"-log":          EXTRACTION,
	"-bin":          EXTRACTION,
	"-oct":          EXTRACTION,
	"-hex":          EXTRACTION,
	"-bit":          EXTRACTION,
	"-pad":          EXTRACTION,
	"-raw":          EXTRACTION,
	"-0-based":      EXTRACTION,
	"-zero-based":   EXTRACTION,
	"-1-based":      EXTRACTION,
	"-one-based":    EXTRACTION,
	"-ucsc":         EXTRACTION,
	"-ucsc-based":   EXTRACTION,
	"-ucsc-coords":  EXTRACTION,
	"-bed-based":    EXTRACTION,
	"-bed-coords":   EXTRACTION,
	"-nucleic":      EXTRACTION,
	"-revcomp":      EXTRACTION,
	"-fasta":        EXTRACTION,
	"-ncbi2na":      EXTRACTION,
	"-ncbi4na":      EXTRACTION,
	"-cds2prot":     EXTRACTION,
	"-pept":         EXTRACTION,
	"-molwt":        EXTRACTION,
	"-molwt-x":      EXTRACTION,
	"-molwt-m":      EXTRACTION,
	"-molwt-f":      EXTRACTION,
	"-accession":    EXTRACTION,
	"-numeric":      EXTRACTION,
	"-hgvs":         EXTRACTION,
	"-else":         EXTRACTION,
	"-pfx":          CUSTOMIZATION,
	"-sfx":          CUSTOMIZATION,
	"-sep":          CUSTOMIZATION,
	"-tab":          CUSTOMIZATION,
	"-ret":          CUSTOMIZATION,
	"-lbl":          CUSTOMIZATION,
	"-tag":          CUSTOMIZATION,
	"-att":          CUSTOMIZATION,
	"-atr":          CUSTOMIZATION,
	"-cls":          CUSTOMIZATION,
	"-slf":          CUSTOMIZATION,
	"-end":          CUSTOMIZATION,
	"-clr":          CUSTOMIZATION,
	"-pfc":          CUSTOMIZATION,
	"-deq":          CUSTOMIZATION,
	"-plg":          CUSTOMIZATION,
	"-elg":          CUSTOMIZATION,
	"-fwd":          CUSTOMIZATION,
	"-awd":          CUSTOMIZATION,
	"-wrp":          CUSTOMIZATION,
	"-bkt":          CUSTOMIZATION,
	"-enc":          CUSTOMIZATION,
	"-pkg":          CUSTOMIZATION,
	"-rst":          CUSTOMIZATION,
	"-def":          CUSTOMIZATION,
	"-mask":         CUSTOMIZATION,
	"-reg":          CUSTOMIZATION,
	"-exp":          CUSTOMIZATION,
	"-with":         CUSTOMIZATION,
	"-gcode":        CUSTOMIZATION,
	"-frame":        CUSTOMIZATION,
	"-frame0":       CUSTOMIZATION,
	"-frame-0":      CUSTOMIZATION,
	"-frame1":       CUSTOMIZATION,
	"-frame-1":      CUSTOMIZATION,
	"-color":        CUSTOMIZATION,
}

var levTypeIs = map[string]LevelType{
	"-unit":     UNIT,
	"-subset":   SUBSET,
	"-section":  SECTION,
	"-block":    BLOCK,
	"-branch":   BRANCH,
	"-group":    GROUP,
	"-division": DIVISION,
	"-path":     PATH,
	"-pattern":  PATTERN,
	"-record":   PATTERN,
}

var opTypeIs = map[string]OpType{
	"-element":      ELEMENT,
	"-first":        FIRST,
	"-last":         LAST,
	"-even":         EVEN,
	"-odd":          ODD,
	"-backward":     BACKWARD,
	"-encode":       ENCODE,
	"-decode":       DECODE,
	"-upper":        UPPER,
	"-lower":        LOWER,
	"-chain":        CHAIN,
	"-title":        TITLE,
	"-mirror":       MIRROR,
	"-alpha":        ALPHA,
	"-alnum":        ALNUM,
	"-basic":        BASIC,
	"-plain":        PLAIN,
	"-simple":       SIMPLE,
	"-author":       AUTHOR,
	"-prose":        PROSE,
	"-jour":         JOURNAL,
	"-journal":      JOURNAL,
	"-year":         YEAR,
	"-month":        MONTH,
	"-date":         DATE,
	"-page":         PAGE,
	"-auth":         AUTH,
	"-initials":     INITIALS,
	"-prop":         PROP,
	"-trim":         TRIM,
	"-wct":          WCT,
	"-doi":          DOI,
	"-translate":    TRANSLATE,
	"-replace":      REPLACE,
	"-terms":        TERMS,
	"-words":        WORDS,
	"-pairs":        PAIRS,
	"-pairx":        PAIRX,
	"-split":        SPLIT,
	"-order":        ORDER,
	"-reverse":      REVERSE,
	"-letters":      LETTERS,
	"-pentamers":    PENTAMERS,
	"-clauses":      CLAUSES,
	"-indexer":      INDEXER,
	"-meshcode":     MESHCODE,
	"-matrix":       MATRIX,
	"-classify":     CLASSIFY,
	"-histogram":    HISTOGRAM,
	"-frequency":    FREQUENCY,
	"-accented":     ACCENTED,
	"-test":         TEST,
	"-scan":         SCAN,
	"-pfx":          PFX,
	"-sfx":          SFX,
	"-sep":          SEP,
	"-tab":          TAB,
	"-ret":          RET,
	"-lbl":          LBL,
	"-tag":          TAG,
	"-att":          ATT,
	"-atr":          ATR,
	"-cls":          CLS,
	"-slf":          SLF,
	"-end":          END,
	"-clr":          CLR,
	"-pfc":          PFC,
	"-deq":          DEQ,
	"-plg":          PLG,
	"-elg":          ELG,
	"-fwd":          FWD,
	"-awd":          AWD,
	"-wrp":          WRP,
	"-bkt":          BKT,
	"-enc":          ENC,
	"-pkg":          PKG,
	"-rst":          RST,
	"-def":          DEF,
	"-mask":         MSK,
	"-reg":          REG,
	"-exp":          EXP,
	"-with":         WITH,
	"-gcode":        GCODE,
	"-frame":        FRAME1,
	"-frame0":       FRAME0,
	"-frame-0":      FRAME0,
	"-frame1":       FRAME1,
	"-frame-1":      FRAME1,
	"-color":        COLOR,
	"-position":     POSITION,
	"-select":       SELECT,
	"-if":           IF,
	"-unless":       UNLESS,
	"-match":        MATCH,
	"-avoid":        AVOID,
	"-and":          AND,
	"-or":           OR,
	"-equals":       EQUALS,
	"-contains":     CONTAINS,
	"-includes":     INCLUDES,
	"-excludes":     EXCLUDES,
	"-is-within":    ISWITHIN,
	"-starts-with":  STARTSWITH,
	"-ends-with":    ENDSWITH,
	"-is-not":       ISNOT,
	"-is-before":    ISBEFORE,
	"-is-after":     ISAFTER,
	"-matches":      MATCHES,
	"-resembles":    RESEMBLES,
	"-is-equal-to":  ISEQUALTO,
	"-differs-from": DIFFERSFROM,
	"-gt":           GT,
	"-ge":           GE,
	"-lt":           LT,
	"-le":           LE,
	"-eq":           EQ,
	"-ne":           NE,
	"-num":          NUM,
	"-len":          LEN,
	"-sum":          SUM,
	"-acc":          ACC,
	"-min":          MIN,
	"-max":          MAX,
	"-inc":          INC,
	"-dec":          DEC,
	"-sub":          SUB,
	"-avg":          AVG,
	"-dev":          DEV,
	"-med":          MED,
	"-mul":          MUL,
	"-div":          DIV,
	"-mod":          MOD,
	"-geo":          GEO,
	"-hrm":          HRM,
	"-rms":          RMS,
	"-sqt":          SQT,
	"-lg2":          LG2,
	"-lge":          LGE,
	"-log":          LOG,
	"-bin":          BIN,
	"-oct":          OCT,
	"-hex":          HEX,
	"-bit":          BIT,
	"-pad":          PAD,
	"-raw":          RAW,
	"-0-based":      ZEROBASED,
	"-zero-based":   ZEROBASED,
	"-1-based":      ONEBASED,
	"-one-based":    ONEBASED,
	"-ucsc":         UCSCBASED,
	"-ucsc-based":   UCSCBASED,
	"-ucsc-coords":  UCSCBASED,
	"-bed-based":    UCSCBASED,
	"-bed-coords":   UCSCBASED,
	"-nucleic":      NUCLEIC,
	"-revcomp":      REVCOMP,
	"-fasta":        FASTA,
	"-ncbi2na":      NCBI2NA,
	"-ncbi4na":      NCBI4NA,
	"-cds2prot":     CDS2PROT,
	"-pept":         PEPTS,
	"-molwt":        MOLWTX,
	"-molwt-x":      MOLWTX,
	"-molwt-m":      MOLWTM,
	"-molwt-f":      MOLWTF,
	"-accession":    ACCESSION,
	"-numeric":      NUMERIC,
	"-hgvs":         HGVS,
	"-else":         ELSE,
}

var sequenceTypeIs = map[string]SequenceType{
	"INSDSeq:INSDInterval_from":       {1, ISSTART},
	"INSDSeq:INSDInterval_to":         {1, ISSTOP},
	"DocumentSummary:ChrStart":        {0, ISSTART},
	"DocumentSummary:ChrStop":         {0, ISSTOP},
	"DocumentSummary:Chr_start":       {1, ISSTART},
	"DocumentSummary:Chr_end":         {1, ISSTOP},
	"DocumentSummary:Chr_inner_start": {1, ISSTART},
	"DocumentSummary:Chr_inner_end":   {1, ISSTOP},
	"DocumentSummary:Chr_outer_start": {1, ISSTART},
	"DocumentSummary:Chr_outer_end":   {1, ISSTOP},
	"DocumentSummary:start":           {1, ISSTART},
	"DocumentSummary:stop":            {1, ISSTOP},
	"DocumentSummary:display_start":   {1, ISSTART},
	"DocumentSummary:display_stop":    {1, ISSTOP},
	"Entrezgene:Seq-interval_from":    {0, ISSTART},
	"Entrezgene:Seq-interval_to":      {0, ISSTOP},
	"GenomicInfoType:ChrStart":        {0, ISSTART},
	"GenomicInfoType:ChrStop":         {0, ISSTOP},
	"RS:position":                     {0, ISPOS},
	"RS:@asnFrom":                     {0, ISSTART},
	"RS:@asnTo":                       {0, ISSTOP},
	"RS:@end":                         {0, ISSTOP},
	"RS:@leftContigNeighborPos":       {0, ISSTART},
	"RS:@physMapInt":                  {0, ISPOS},
	"RS:@protLoc":                     {0, ISPOS},
	"RS:@rightContigNeighborPos":      {0, ISSTOP},
	"RS:@start":                       {0, ISSTART},
	"RS:@structLoc":                   {0, ISPOS},
}

var monthTable = map[string]int{
	"jan":       1,
	"january":   1,
	"feb":       2,
	"february":  2,
	"mar":       3,
	"march":     3,
	"apr":       4,
	"april":     4,
	"may":       5,
	"jun":       6,
	"june":      6,
	"jul":       7,
	"july":      7,
	"aug":       8,
	"august":    8,
	"sep":       9,
	"september": 9,
	"oct":       10,
	"october":   10,
	"nov":       11,
	"november":  11,
	"dec":       12,
	"december":  12,
}

var propertyTable = map[string]string{
	"AssociatedDataset":           "Associated Dataset",
	"AssociatedPublication":       "Associated Publication",
	"CommentIn":                   "Comment In",
	"CommentOn":                   "Comment On",
	"ErratumFor":                  "Erratum For",
	"ErratumIn":                   "Erratum In",
	"ExpressionOfConcernFor":      "Expression Of Concern For",
	"ExpressionOfConcernIn":       "Expression Of Concern In",
	"OriginalReportIn":            "Original Report In",
	"ReprintIn":                   "Reprint In",
	"ReprintOf":                   "Reprint Of",
	"RepublishedFrom":             "Republished From",
	"RepublishedIn":               "Republished In",
	"RetractedandRepublishedFrom": "Retracted And Republished From",
	"RetractedandRepublishedIn":   "Retracted And Republished In",
	"RetractionIn":                "Retraction In",
	"RetractionOf":                "Retraction Of",
	"SummaryForPatientsIn":        "Summary For Patients In",
	"UpdateIn":                    "Update In",
	"UpdateOf":                    "Update Of",
	"aheadofprint":                "Ahead Of Print",
	"epublish":                    "Electronically Published",
	"ppublish":                    "Published In Print",
}

// DATA OBJECTS

// Step contains parameters for executing a single command step
type Step struct {
	Type   OpType
	Value  string
	Parent string
	Match  string
	Attrib string
	TypL   RangeType
	StrL   string
	IntL   int
	TypR   RangeType
	StrR   string
	IntR   int
	Norm   bool
	Wild   bool
	Unesc  bool
}

// Operation breaks commands into sequential steps
type Operation struct {
	Type   OpType
	Value  string
	Stages []*Step
}

// Block contains nested instructions for executing commands
type Block struct {
	Visit      string
	Parent     string
	Match      string
	Path       []string
	Working    []string
	Parsed     []string
	Position   string
	Foreword   string
	Afterword  string
	Conditions []*Operation
	Commands   []*Operation
	Failure    []*Operation
	Subtasks   []*Block
}

// Limiter is used for collecting specific nodes (e.g., first and last)
type Limiter struct {
	Obj *XMLNode
	Idx int
	Lvl int
}

// DebugBlock examines structure of parsed arguments (undocumented)
/*
func DebugBlock(blk *Block, depth int) {

	doIndent := func(indt int) {
		for i := 1; i < indt; i++ {
			fmt.Fprintf(os.Stderr, "  ")
		}
	}

	doIndent(depth)

	if blk.Visit != "" {
		doIndent(depth + 1)
		fmt.Fprintf(os.Stderr, "<Visit> %s </Visit>\n", blk.Visit)
	}
	if len(blk.Parsed) > 0 {
		doIndent(depth + 1)
		fmt.Fprintf(os.Stderr, "<Parsed>")
		for _, str := range blk.Parsed {
			fmt.Fprintf(os.Stderr, " %s", str)
		}
		fmt.Fprintf(os.Stderr, " </Parsed>\n")
	}

	if len(blk.Subtasks) > 0 {
		for _, sub := range blk.Subtasks {
			DebugBlock(sub, depth+1)
		}
	}
}
*/

// PARSE COMMAND-LINE ARGUMENTS

// ParseArguments parses nested exploration instruction from command-line arguments
func ParseArguments(cmdargs []string, pttrn string) *Block {

	// different names of exploration control arguments allow multiple levels of nested "for" loops in a linear command line
	// (capitalized versions for backward-compatibility with original Perl implementation handling of recursive definitions)
	var (
		lcname = []string{
			"",
			"-unit",
			"-subsetX",
			"-subset",
			"-sectionX",
			"-section",
			"-blockX",
			"-block",
			"-branchX",
			"-branch",
			"-groupX",
			"-group",
			"-divisionX",
			"-division",
			"-pathX",
			"-path",
			"-pattern",
		}

		ucname = []string{
			"",
			"-Unit",
			"-SubsetX",
			"-Subset",
			"-SectionX",
			"-Section",
			"-BlockX",
			"-Block",
			"-BranchX",
			"-Branch",
			"-GroupX",
			"-Group",
			"-DivisionX",
			"-Division",
			"-PathX",
			"-Path",
			"-Pattern",
		}
	)

	parseFlag := func(str string, rest []string) (OpType, bool) {

		ch, ok := HasLeadingUnicodeDash(str)
		if ok {
			DisplayError("Unicode dash %d replaced expected ASCII hyphen in '%s'", ch, str)
			os.Exit(1)
		}

		op, ok := opTypeIs[str]
		if ok {
			if argTypeIs[str] == EXTRACTION {
				return op, true
			}
			return op, false
		}

		// check for user-defined variable - one to three dashes followed by all capital letters or digits

		if len(str) > 1 && str[0] == '-' && IsAllCapsOrDigits(str[1:]) {
			if len(rest) > 0 {
				next := rest[0]
				// if next argument is a command
				if strings.HasPrefix(next, "-") {
					// then no need for explicit --- prefix to indicate CONVERTER variable
					return CONVERTER, true
				}
			}
			return VARIABLE, true
		}

		if len(str) > 2 && strings.HasPrefix(str, "--") && IsAllCapsOrDigits(str[2:]) {
			return ACCUMULATOR, true
		}

		if len(str) > 3 && strings.HasPrefix(str, "---") && IsAllCapsOrDigits(str[3:]) {
			// original explicit triple dash prefix for CONVERTER variable development (undocumented)
			return CONVERTER, true
		}

		// any other argument starting with a dash is an unrecognized command

		if len(str) > 0 && str[0] == '-' {
			return UNRECOGNIZED, false
		}

		return UNSET, false
	}

	// parseCommands recursive definition
	var parseCommands func(parent *Block, startLevel LevelType)

	// parseCommands does initial parsing of exploration command structure
	parseCommands = func(parent *Block, startLevel LevelType) {

		// find next highest level exploration argument
		findNextLevel := func(args []string, level LevelType) (LevelType, string, string) {

			if len(args) > 1 {

				for {

					if level < UNIT {
						break
					}

					lctag := lcname[level]
					uctag := ucname[level]

					for _, txt := range args {
						if txt == lctag {
							return level, lctag, uctag
						}
						if txt == uctag {
							DisplayWarning("Upper-case '%s' exploration command is deprecated, use lower-case '%s' instead", uctag, lctag)
							return level, lctag, uctag
						}
					}

					level--
				}
			}

			return 0, "", ""
		}

		arguments := parent.Working

		level, lctag, uctag := findNextLevel(arguments, startLevel)

		if level < UNIT {

			// break recursion
			return
		}

		// group arguments at a given exploration level
		subsetCommands := func(args []string) *Block {

			max := len(args)

			visit := ""

			// extract name of object to visit
			if max > 1 {
				visit = args[1]
				args = args[2:]
				max -= 2
			}

			partition := 0
			for cur, str := range args {

				// record point of next exploration command
				partition = cur + 1

				// skip if not a command
				if len(str) < 1 || str[0] != '-' {
					continue
				}

				if argTypeIs[str] == EXPLORATION {
					partition = cur
					break
				}
			}

			// convert slashes (e.g., parent/child construct) to periods (e.g., dotted exploration path)
			if strings.Contains(visit, "/") {
				if !strings.Contains(visit, ".") {
					visit = strings.Replace(visit, "/", ".", -1)
				}
			}

			// parse parent.child or dotted path construct
			// colon indicates a namespace prefix in any or all of the components
			prnt, rmdr := SplitInTwoRight(visit, ".")
			match, rest := SplitInTwoLeft(rmdr, ".")

			if rest != "" {

				// exploration match on first component, then search remainder one level at a time with subsequent components
				dirs := strings.Split(rmdr, ".")

				// signal with "path" position
				return &Block{Visit: visit, Parent: "", Match: prnt, Path: dirs, Position: "path", Parsed: args[0:partition], Working: args[partition:]}
			}

			// promote arguments parsed at this level
			return &Block{Visit: visit, Parent: prnt, Match: match, Parsed: args[0:partition], Working: args[partition:]}
		}

		cur := 0

		// search for positions of current exploration command

		for idx, txt := range arguments {
			if txt == lctag || txt == uctag {
				if idx == 0 {
					continue
				}

				blk := subsetCommands(arguments[cur:idx])
				parseCommands(blk, level-1)
				parent.Subtasks = append(parent.Subtasks, blk)

				cur = idx
			}
		}

		if cur < len(arguments) {
			blk := subsetCommands(arguments[cur:])
			parseCommands(blk, level-1)
			parent.Subtasks = append(parent.Subtasks, blk)
		}

		// clear execution arguments from parent after subsetting
		parent.Working = nil
	}

	// parse optional [min:max], [&VAR:&VAR], or [after|before] range specification
	parseRange := func(item, rnge string) (typL RangeType, strL string, intL int, typR RangeType, strR string, intR int) {

		typL = NORANGE
		typR = NORANGE
		strL = ""
		strR = ""
		intL = 0
		intR = 0

		if rnge == "" {
			// no range specification, return default values
			return
		}

		// check if last character is right square bracket
		if !strings.HasSuffix(rnge, "]") {
			DisplayError("Unrecognized range %s", rnge)
			os.Exit(1)
		}

		rnge = strings.TrimSuffix(rnge, "]")

		if rnge == "" {
			DisplayError("Empty range %s[]", item)
			os.Exit(1)
		}

		// check for caret [after^before] variant that allows vertical bar in contents
		if strings.Contains(rnge, "^") {

			strL, strR = SplitInTwoLeft(rnge, "^")
			// spacing matters, so do not call TrimSpace

			if strL == "" && strR == "" {
				DisplayError("Empty range %s[^]", item)
				os.Exit(1)
			}

			typL = STRINGRANGE
			typR = STRINGRANGE

			// return statement returns named variables
			return
		}

		// next check for original vertical bar [after|before] variant
		if strings.Contains(rnge, "|") {

			strL, strR = SplitInTwoLeft(rnge, "|")
			// spacing matters, so do not call TrimSpace

			if strL == "" && strR == "" {
				DisplayError("Empty range %s[|]", item)
				os.Exit(1)
			}

			typL = STRINGRANGE
			typR = STRINGRANGE

			// return statement returns named variables
			return
		}

		// otherwise must have colon and integers [from:to] within brackets
		if !strings.Contains(rnge, ":") {
			DisplayError("Colon missing in range %s[%s]", item, rnge)
			os.Exit(1)
		}

		// split at colon
		lft, rgt := SplitInTwoLeft(rnge, ":")

		lft = strings.TrimSpace(lft)
		rgt = strings.TrimSpace(rgt)

		if lft == "" && rgt == "" {
			DisplayError("Empty range %s[:]", item)
			os.Exit(1)
		}

		// for variable, parse optional +/- offset suffix
		parseOffset := func(str string) (string, int) {

			if str == "" || str[0] == ' ' {
				DisplayError("Unrecognized variable '&%s'", str)
				os.Exit(1)
			}

			pls := ""
			mns := ""

			ofs := 0

			// check for &VAR+1 or &VAR-1 integer adjustment
			str, pls = SplitInTwoLeft(str, "+")
			str, mns = SplitInTwoLeft(str, "-")

			if pls != "" {
				val, err := strconv.Atoi(pls)
				if err != nil {
					DisplayError("Unrecognized range adjustment &%s+%s", str, pls)
					os.Exit(1)
				}
				ofs = val
			} else if mns != "" {
				val, err := strconv.Atoi(mns)
				if err != nil {
					DisplayError("Unrecognized range adjustment &%s-%s", str, mns)
					os.Exit(1)
				}
				ofs = -val
			}

			return str, ofs
		}

		// parse integer position, 1-based coordinate must be greater than 0
		parseInteger := func(str string, mustBePositive bool) int {
			if str == "" {
				return 0
			}

			val, err := strconv.Atoi(str)
			if err != nil {
				DisplayError("Unrecognized range component %s[%s:]", item, str)
				os.Exit(1)
			}
			if mustBePositive {
				if val < 1 {
					DisplayError("Range component %s[%s:] must be positive", item, str)
					os.Exit(1)
				}
			} else {
				if val == 0 {
					DisplayError("Range component %s[%s:] must not be zero", item, str)
					os.Exit(1)
				}
			}

			return val
		}

		if lft != "" {
			if lft[0] == '&' {
				lft = lft[1:]
				strL, intL = parseOffset(lft)
				typL = VARIABLERANGE
			} else {
				intL = parseInteger(lft, true)
				typL = INTEGERRANGE
			}
		}

		if rgt != "" {
			if rgt[0] == '&' {
				rgt = rgt[1:]
				strR, intR = parseOffset(rgt)
				typR = VARIABLERANGE
			} else {
				intR = parseInteger(rgt, false)
				typR = INTEGERRANGE
			}
		}

		// return statement required to return named variables
		return
	}

	parseConditionals := func(cmds *Block, arguments []string) []*Operation {

		max := len(arguments)
		if max < 1 {
			return nil
		}

		// check for missing condition command
		txt := arguments[0]

		ch, ok := HasLeadingUnicodeDash(txt)
		if ok {
			DisplayError("Unicode dash %d replaced expected ASCII hyphen in '%s'", ch, txt)
			os.Exit(1)
		}

		if txt != "-if" && txt != "-unless" && txt != "-select" && txt != "-match" && txt != "-avoid" && txt != "-position" {
			DisplayError("Missing -if command before '%s'", txt)
			os.Exit(1)
		}
		if txt == "-position" && max > 2 {
			DisplayError("Cannot combine -position with -if or -unless commands")
			os.Exit(1)
		}
		// check for missing argument after last condition
		txt = arguments[max-1]
		if len(txt) > 0 && txt[0] == '-' {
			DisplayError("Item missing after %s command", txt)
			os.Exit(1)
		}

		cond := make([]*Operation, 0, max)

		// parse conditional clause into execution step
		parseStep := func(op *Operation, elementColonValue bool) {

			if op == nil {
				return
			}

			str := op.Value

			status := ELEMENT

			// isolate and parse optional [min:max], [&VAR:&VAR], or [after|before] range specification
			str, rnge := SplitInTwoLeft(str, "[")

			str = strings.TrimSpace(str)
			rnge = strings.TrimSpace(rnge)

			if str == "" && rnge != "" {
				// rnge should already end with right square bracket
				DisplayError("Variable missing in range specification [%s", rnge)
				os.Exit(1)
			}

			typL, strL, intL, typR, strR, intR := parseRange(str, rnge)

			// check for pound, percent, or caret character at beginning of name
			if len(str) > 1 {
				switch str[0] {
				case '&':
					if IsAllCapsOrDigits(str[1:]) {
						status = VARIABLE
						str = str[1:]
					} else if strings.Contains(str, ":") {
						DisplayError("Unsupported construct '%s', use -if &VARIABLE -equals VALUE instead", str)
						os.Exit(1)
					} else {
						DisplayError("Unrecognized variable '%s'", str)
						os.Exit(1)
					}
				case '#':
					status = COUNT
					str = str[1:]
				case '%':
					status = LENGTH
					str = str[1:]
				case '^':
					status = DEPTH
					str = str[1:]
				default:
				}
			} else if str == "+" {
				status = INDEX
			}

			// parse parent/element@attribute construct
			// colon indicates a namespace prefix in any or all of the components
			prnt, match := SplitInTwoRight(str, "/")
			match, attrib := SplitInTwoLeft(match, "@")
			val := ""

			// leading colon indicates namespace prefix wildcard
			wildcard := false
			if strings.HasPrefix(prnt, ":") || strings.HasPrefix(match, ":") || strings.HasPrefix(attrib, ":") {
				wildcard = true
			}

			if elementColonValue {

				// allow parent/element@attribute:value construct for deprecated -match and -avoid, and for subsequent -and and -or commands
				match, val = SplitInTwoLeft(str, ":")
				prnt, match = SplitInTwoRight(match, "/")
				match, attrib = SplitInTwoLeft(match, "@")
			}

			norm := true
			if rnge != "" {
				if typL != NORANGE || typR != NORANGE || strL != "" || strR != "" || intL != 0 || intR != 0 {
					norm = false
				}
			}

			tsk := &Step{Type: status, Value: str, Parent: prnt, Match: match, Attrib: attrib,
				TypL: typL, StrL: strL, IntL: intL, TypR: typR, StrR: strR, IntR: intR,
				Norm: norm, Wild: wildcard}

			op.Stages = append(op.Stages, tsk)

			// transform old -match "element:value" to -match element -equals value
			if val != "" {
				tsk := &Step{Type: EQUALS, Value: val}
				op.Stages = append(op.Stages, tsk)
			}
		}

		idx := 0

		// conditionals should alternate between command and object/value
		expectDash := true
		last := ""

		var op *Operation

		// flag to allow element-colon-value for deprecated -match and -avoid commands, otherwise colon is for namespace prefixes
		elementColonValue := false

		status := UNSET

		numIf := 0
		numUnless := 0
		lastCond := ""

		// parse command strings into operation structure
		for idx < max {
			str := arguments[idx]
			idx++

			// conditionals should alternate between command and object/value
			if expectDash {

				ch, ok := HasLeadingUnicodeDash(str)
				if ok {
					DisplayError("Unicode dash %d replaced expected ASCII hyphen in '%s'", ch, str)
					os.Exit(1)
				}

				if len(str) < 1 || str[0] != '-' {
					DisplayError("Unexpected '%s' argument after '%s'", str, last)
					os.Exit(1)
				}
				expectDash = false

			} else {

				if len(str) > 0 && str[0] == '-' {
					DisplayError("Unexpected '%s' command after '%s'", str, last)
					os.Exit(1)
				}
				expectDash = true
			}
			last = str

			switch status {
			case UNSET:
				status, _ = parseFlag(str, arguments[idx:])
			case POSITION:
				if cmds.Position != "" {
					DisplayError("-position '%s' conflicts with existing '%s'", str, cmds.Position)
					os.Exit(1)
				}
				cmds.Position = str
				status = UNSET
			case MATCH, AVOID:
				elementColonValue = true
				fallthrough
			case IF:
				numIf++
				if numIf > 1 || numUnless > 1 || numIf > 0 && numUnless > 0 {
					DisplayError("Unexpected '-if %s' after '%s'", str, lastCond)
					os.Exit(1)
				}
				lastCond = "-if " + str
				op = &Operation{Type: status, Value: str}
				cond = append(cond, op)
				parseStep(op, elementColonValue)
				status = UNSET
			case UNLESS:
				numUnless++
				if numIf > 1 || numUnless > 1 || numIf > 0 && numUnless > 0 {
					DisplayError("Unexpected '-unless %s' after '%s'", str, lastCond)
					os.Exit(1)
				}
				lastCond = "-unless " + str
				op = &Operation{Type: status, Value: str}
				cond = append(cond, op)
				parseStep(op, elementColonValue)
				status = UNSET
			case SELECT, AND, OR:
				op = &Operation{Type: status, Value: str}
				cond = append(cond, op)
				parseStep(op, elementColonValue)
				status = UNSET
			case EQUALS, CONTAINS, INCLUDES, EXCLUDES, ISWITHIN, STARTSWITH, ENDSWITH, ISNOT, ISBEFORE, ISAFTER:
				if op != nil {
					if len(str) > 1 && str[0] == '\\' {
						// first character may be backslash protecting dash (undocumented)
						str = str[1:]
					}
					tsk := &Step{Type: status, Value: str}
					op.Stages = append(op.Stages, tsk)
					op = nil
				} else {
					DisplayError("Unexpected adjacent string match constraints")
					os.Exit(1)
				}
				status = UNSET
			case MATCHES:
				if op != nil {
					if len(str) > 1 && str[0] == '\\' {
						// first character may be backslash protecting dash (undocumented)
						str = str[1:]
					}
					str = RemoveCommaOrSemicolon(str)
					tsk := &Step{Type: status, Value: str}
					op.Stages = append(op.Stages, tsk)
					op = nil
				} else {
					DisplayError("Unexpected adjacent string match constraints")
					os.Exit(1)
				}
				status = UNSET
			case RESEMBLES:
				if op != nil {
					if len(str) > 1 && str[0] == '\\' {
						// first character may be backslash protecting dash (undocumented)
						str = str[1:]
					}
					str = SortStringByWords(str)
					tsk := &Step{Type: status, Value: str}
					op.Stages = append(op.Stages, tsk)
					op = nil
				} else {
					DisplayError("Unexpected adjacent string match constraints")
					os.Exit(1)
				}
				status = UNSET
			case ISEQUALTO, DIFFERSFROM:
				if op != nil {
					if len(str) < 1 {
						DisplayError("Empty conditional argument")
						os.Exit(1)
					}
					ch := str[0]
					// uses element as second argument
					orig := str
					if ch == '#' || ch == '%' || ch == '^' {
						// check for pound, percent, or caret character at beginning of element (undocumented)
						str = str[1:]
						if len(str) < 1 {
							DisplayError("Unexpected conditional constraints")
							os.Exit(1)
						}
						ch = str[0]
					}
					if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
						prnt, match := SplitInTwoRight(str, "/")
						match, attrib := SplitInTwoLeft(match, "@")
						wildcard := false
						if strings.HasPrefix(prnt, ":") || strings.HasPrefix(match, ":") || strings.HasPrefix(attrib, ":") {
							wildcard = true
						}
						tsk := &Step{Type: status, Value: orig, Parent: prnt, Match: match, Attrib: attrib, Wild: wildcard}
						op.Stages = append(op.Stages, tsk)
					} else {
						DisplayError("Unexpected conditional constraints")
						os.Exit(1)
					}
					op = nil
				}
				status = UNSET
			case GT, GE, LT, LE, EQ, NE:
				if op != nil {
					if len(str) > 1 && str[0] == '\\' {
						// first character may be backslash protecting minus sign (undocumented)
						str = str[1:]
					}
					if len(str) < 1 {
						DisplayError("Empty numeric match constraints")
						os.Exit(1)
					}
					ch := str[0]
					if (ch >= '0' && ch <= '9') || ch == '-' || ch == '+' {
						// literal numeric constant
						tsk := &Step{Type: status, Value: str}
						op.Stages = append(op.Stages, tsk)
					} else {
						// numeric test allows element as second argument
						orig := str
						if ch == '#' || ch == '%' || ch == '^' {
							// check for pound, percent, or caret character at beginning of element (undocumented)
							str = str[1:]
							if len(str) < 1 {
								DisplayError("Unexpected numeric match constraints")
								os.Exit(1)
							}
							ch = str[0]
						}
						if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '&' {
							prnt, match := SplitInTwoRight(str, "/")
							match, attrib := SplitInTwoLeft(match, "@")
							wildcard := false
							if strings.HasPrefix(prnt, ":") || strings.HasPrefix(match, ":") || strings.HasPrefix(attrib, ":") {
								wildcard = true
							}
							tsk := &Step{Type: status, Value: orig, Parent: prnt, Match: match, Attrib: attrib, Wild: wildcard}
							op.Stages = append(op.Stages, tsk)
						} else {
							DisplayError("Unexpected numeric match constraints")
							os.Exit(1)
						}
					}
					op = nil
				} else {
					DisplayError("Unexpected adjacent numeric match constraints")
					os.Exit(1)
				}
				status = UNSET
			case UNRECOGNIZED:
				DisplayError("Unrecognized argument '%s'", str)
				os.Exit(1)
			default:
				DisplayError("Unexpected argument '%s'", str)
				os.Exit(1)
			}
		}

		return cond
	}

	parseExtractions := func(cmds *Block, arguments []string) []*Operation {

		max := len(arguments)
		if max < 1 {
			return nil
		}

		// check for missing -element (or -first, etc.) command
		txt := arguments[0]

		ch, ok := HasLeadingUnicodeDash(txt)
		if ok {
			DisplayError("Unicode dash %d replaced expected ASCII hyphen in '%s'", ch, txt)
			os.Exit(1)
		}

		if len(txt) < 1 || txt[0] != '-' {
			DisplayError("Missing -element command before '%s'", txt)
			os.Exit(1)
		}
		// check for missing argument after last -element (or -first, etc.) command
		txt = arguments[max-1]
		if len(txt) > 0 && txt[0] == '-' {
			if txt == "-rst" {
				DisplayError("Unexpected position for %s command", txt)
				os.Exit(1)
			} else if txt == "-clr" {
				// main loop runs out after trailing -clr, add another one so this one will be executed
				arguments = append(arguments, "-clr")
				max++
			} else if txt == "-cls" || txt == "-slf" {
				// okay at end
			} else if max < 2 || arguments[max-2] != "-lbl" {
				DisplayError("Item missing after %s command", txt)
				os.Exit(1)
			} else if max < 3 || (arguments[max-3] != "-att" && arguments[max-3] != "-atr") {
				DisplayError("Item missing after %s command", txt)
				os.Exit(1)
			}
		}

		comm := make([]*Operation, 0, max)

		removeLeadingHyphens := func(str string) string {

			for strings.HasPrefix(str, "-") {
				str = strings.TrimPrefix(str, "-")
			}

			ch, ok := HasLeadingUnicodeDash(str)
			if ok {
				DisplayError("Unicode dash %d replaced expected ASCII hyphen in '%s'", ch, str)
				os.Exit(1)
			}

			return str
		}

		// parse next argument
		nextStatus := func(str string, rest []string) (OpType, bool) {

			status, isExtraction := parseFlag(str, rest)

			// no-argument flags must be handled here to prevent subsequent "No -element before" error
			switch status {
			case VARIABLE:
				op := &Operation{Type: status, Value: removeLeadingHyphens(str)}
				comm = append(comm, op)
				status = VALUE
			case ACCUMULATOR:
				op := &Operation{Type: status, Value: removeLeadingHyphens(str)}
				comm = append(comm, op)
				status = VALUE
			case CONVERTER:
				op := &Operation{Type: status, Value: removeLeadingHyphens(str)}
				comm = append(comm, op)
				// CONVERTER sets next status to UNSET, not VALUE
				status = UNSET
			case CLR, RST:
				op := &Operation{Type: status, Value: ""}
				comm = append(comm, op)
				status = UNSET
			case ELEMENT:
			case TAB, RET, PFX, SFX, SEP, LBL, TAG, ATT, ATR, END, PFC, DEQ, PLG, ELG, WRP, BKT, ENC, DEF, MSK, REG, EXP, WITH, GCODE, FRAME0, FRAME1, COLOR:
			case CLS:
				op := &Operation{Type: LBL, Value: ">"}
				comm = append(comm, op)
				status = UNSET
			case SLF:
				op := &Operation{Type: LBL, Value: " />"}
				comm = append(comm, op)
				status = UNSET
			case FWD, AWD, PKG:
			case UNSET:
				DisplayError("No -element before '%s'", str)
				os.Exit(1)
			case UNRECOGNIZED:
				DisplayError("Unrecognized argument '%s'", str)
				os.Exit(1)
			default:
				if !isExtraction {
					// not ELEMENT through HGVS
					DisplayError("Misplaced %s command", str)
					os.Exit(1)
				}
			}

			return status, isExtraction
		}

		// parse extraction clause into individual steps
		parseSteps := func(op *Operation, pttrn string) {

			if op == nil {
				return
			}

			stat := op.Type
			str := op.Value

			// element names combined with commas are treated as a prefix-separator-suffix group
			comma := strings.Split(str, ",")

			rnge := ""
			for _, item := range comma {
				status := stat

				// isolate and parse optional [min:max], [&VAR:&VAR], or [after|before] range specification
				item, rnge = SplitInTwoLeft(item, "[")

				item = strings.TrimSpace(item)
				rnge = strings.TrimSpace(rnge)

				if item == "" && rnge != "" {
					// rnge should already end with right square bracket
					DisplayError("Variable missing in range specification [%s", rnge)
					os.Exit(1)
				}

				typL, strL, intL, typR, strR, intR := parseRange(item, rnge)

				// check for special character at beginning of name
				if len(item) > 1 {
					switch item[0] {
					case '&':
						if IsAllCapsOrDigits(item[1:]) {
							status = VARIABLE
							item = item[1:]
						} else {
							DisplayError("Unrecognized variable '%s'", item)
							os.Exit(1)
						}
					case '#':
						status = COUNT
						item = item[1:]
					case '%':
						status = LENGTH
						item = item[1:]
					case '^':
						status = DEPTH
						item = item[1:]
					case '*':
						for _, ch := range item {
							if ch != '*' {
								break
							}
						}
						status = STAR
					default:
					}
				} else {
					switch item {
					case "?":
						status = QUESTION
					case "~":
						status = TILDE
					case ".":
						status = DOT
					case "%":
						status = PRCNT
					case "*":
						status = STAR
					case "$":
						status = DOLLAR
					case "@":
						status = ATSIGN
					case "+":
						status = INDEX
					default:
					}
				}

				// parse parent/element@attribute construct
				// colon indicates a namespace prefix in any or all of the components
				prnt, match := SplitInTwoRight(item, "/")
				match, attrib := SplitInTwoLeft(match, "@")

				// leading colon indicates namespace prefix wildcard
				wildcard := false
				if strings.HasPrefix(prnt, ":") || strings.HasPrefix(match, ":") || strings.HasPrefix(attrib, ":") {
					wildcard = true
				}

				// sequence coordinate adjustments
				switch status {
				case ZEROBASED, ONEBASED, UCSCBASED:
					seq := pttrn + ":"
					if attrib != "" {
						seq += "@"
						seq += attrib
					} else if match != "" {
						seq += match
					}
					// confirm -0-based or -1-based arguments are known sequence position elements or attributes
					slock.RLock()
					seqtype, ok := sequenceTypeIs[seq]
					slock.RUnlock()
					if !ok {
						DisplayError("Element '%s' is not suitable for sequence coordinate conversion", item)
						os.Exit(1)
					}
					switch status {
					case ZEROBASED:
						status = ELEMENT
						// if 1-based coordinates, decrement to get 0-based value
						if seqtype.Based == 1 {
							status = DECR
						}
					case ONEBASED:
						status = ELEMENT
						// if 0-based coordinates, increment to get 1-based value
						if seqtype.Based == 0 {
							status = INCR
						}
					case UCSCBASED:
						status = ELEMENT
						// half-open intervals, start is 0-based, stop is 1-based
						if seqtype.Based == 0 && seqtype.Which == ISSTOP {
							status = INCR
						} else if seqtype.Based == 1 && seqtype.Which == ISSTART {
							status = DECR
						}
					default:
						status = ELEMENT
					}
				default:
				}

				norm := true
				if rnge != "" {
					if typL != NORANGE || typR != NORANGE || strL != "" || strR != "" || intL != 0 || intR != 0 {
						norm = false
					}
				}

				unescape := (status != INDEXER && status != RAW)

				tsk := &Step{Type: status, Value: item, Parent: prnt, Match: match, Attrib: attrib,
					TypL: typL, StrL: strL, IntL: intL, TypR: typR, StrR: strR, IntR: intR,
					Norm: norm, Wild: wildcard, Unesc: unescape}

				op.Stages = append(op.Stages, tsk)
			}
		}

		idx := 0

		status := UNSET
		isExtraction := false

		// parse command strings into operation structure
		for idx < max {
			str := arguments[idx]
			idx++

			if argTypeIs[str] == CONDITIONAL {
				DisplayError("Misplaced %s command", str)
				os.Exit(1)
			}

			switch status {
			case UNSET:
				status, isExtraction = nextStatus(str, arguments[idx:])
			case TAB, RET, PFX, SFX, SEP, LBL, CLS, SLF, PFC, DEQ, PLG, ELG, WRP, BKT, ENC, DEF, MSK, REG, EXP, WITH, GCODE, FRAME0, FRAME1, COLOR:
				op := &Operation{Type: status, Value: ConvertSlash(str)}
				comm = append(comm, op)
				status = UNSET
			case TAG:
				// when starting to construct XML tag and attributes from components, first clear -tab and -sep values
				op := &Operation{Type: TAB, Value: ""}
				comm = append(comm, op)
				op = &Operation{Type: SEP, Value: ""}
				comm = append(comm, op)
				// TAG variant of LBL sets wrp flag for automatic content reencoding
				op = &Operation{Type: TAG, Value: "<" + ConvertSlash(str)}
				comm = append(comm, op)
				status = UNSET
			case ATT:
				if idx < max {
					// -att takes key and literal string value
					val := arguments[idx]
					idx++
					if val != "" {
						op := &Operation{Type: LBL, Value: " " + ConvertSlash(str) + "=" + "\"" + ConvertSlash(val) + "\""}
						comm = append(comm, op)
					}
				}
				status = UNSET
			case ATR:
				if idx < max {
					// -atr takes key and object or &variable name
					val := arguments[idx]
					idx++
					if val != "" {
						op := &Operation{Type: LBL, Value: " " + ConvertSlash(str) + "=" + "\""}
						comm = append(comm, op)
						op = &Operation{Type: ELEMENT, Value: val}
						comm = append(comm, op)
						parseSteps(op, pttrn)
						op = &Operation{Type: LBL, Value: "\""}
						comm = append(comm, op)
					}
				}
				status = UNSET
			case END:
				op := &Operation{Type: LBL, Value: "</" + ConvertSlash(str) + ">"}
				comm = append(comm, op)
				status = UNSET
			case FWD:
				cmds.Foreword = ConvertSlash(str)
				status = UNSET
			case AWD:
				cmds.Afterword = ConvertSlash(str)
				status = UNSET
			case PKG:
				pkg := ConvertSlash(str)
				cmds.Foreword = ""
				cmds.Afterword = ""
				if pkg != "" && pkg != "-" {
					items := strings.Split(pkg, "/")
					for i := range len(items) {
						cmds.Foreword += "<" + items[i] + ">"
					}
					for i := len(items) - 1; i >= 0; i-- {
						cmds.Afterword += "</" + items[i] + ">"
					}
				}
				status = UNSET
			case VARIABLE:
				op := &Operation{Type: status, Value: removeLeadingHyphens(str)}
				comm = append(comm, op)
				status = VALUE
			case ACCUMULATOR:
				op := &Operation{Type: status, Value: removeLeadingHyphens(str)}
				comm = append(comm, op)
				status = VALUE
			case CONVERTER:
				op := &Operation{Type: status, Value: removeLeadingHyphens(str)}
				comm = append(comm, op)
				// CONVERTER sets next status to UNSET, not VALUE
				status = UNSET
			case VALUE:
				op := &Operation{Type: status, Value: str}
				comm = append(comm, op)
				parseSteps(op, pttrn)
				status = UNSET
			case UNRECOGNIZED:
				DisplayError("Unrecognized argument '%s'", str)
				os.Exit(1)
			default:
				if isExtraction {
					// ELEMENT through HGVS
					for !strings.HasPrefix(str, "-") {
						// create one operation per argument, even if under a single -element statement
						op := &Operation{Type: status, Value: str}
						comm = append(comm, op)
						parseSteps(op, pttrn)
						if idx >= max {
							break
						}
						str = arguments[idx]
						idx++
					}
					status = UNSET
					if idx < max {
						status, isExtraction = nextStatus(str, arguments[idx:])
					}
				}
			}
		}

		return comm
	}

	// parseOperations recursive definition
	var parseOperations func(parent *Block)

	// parseOperations converts parsed arguments to operations lists
	parseOperations = func(parent *Block) {

		args := parent.Parsed

		partition := 0
		for cur, str := range args {

			// record junction between conditional and extraction commands
			partition = cur + 1

			// skip if not a command
			if len(str) < 1 || str[0] != '-' {
				continue
			}

			if argTypeIs[str] != CONDITIONAL {
				partition = cur
				break
			}
		}

		// split arguments into conditional tests and extraction or customization commands
		conditionals := args[0:partition]
		args = args[partition:]

		partition = 0
		foundElse := false
		for cur, str := range args {

			// record junction at -else command
			partition = cur + 1

			// skip if not a command
			if len(str) < 1 || str[0] != '-' {
				continue
			}

			if str == "-else" {
				partition = cur
				foundElse = true
				break
			}
		}

		extractions := args[0:partition]
		alternative := args[partition:]

		if len(alternative) > 0 && alternative[0] == "-else" {
			alternative = alternative[1:]
		}

		// validate argument structure and convert to operations lists
		parent.Conditions = parseConditionals(parent, conditionals)
		parent.Commands = parseExtractions(parent, extractions)
		parent.Failure = parseExtractions(parent, alternative)

		// reality checks on placement of -else command
		if foundElse {
			if len(conditionals) < 1 {
				DisplayError("Misplaced -else command")
				os.Exit(1)
			}
			if len(alternative) < 1 {
				DisplayError("Misplaced -else command")
				os.Exit(1)
			}
			if len(parent.Subtasks) > 0 {
				DisplayError("Misplaced -else command")
				os.Exit(1)
			}
		}

		for _, sub := range parent.Subtasks {
			parseOperations(sub)
		}
	}

	// preprocess arguments to work around -pkg artifact

	var expargs []string

	lastLevel := ""
	lastVisit := ""
	isExplore := false

	// add second exploration level to "-block INSDFeature -pkg feat_intervals"
	// resulting in "-block INSDFeature -blockX INSDFeature -pkg feat_intervals"
	for _, txt := range cmdargs {

		if argTypeIs[txt] == EXPLORATION && txt != "-pattern" && txt != "-Pattern" {

			// extra exploration arguments ending in X are for internal use only
			if strings.HasSuffix(txt, "X") {
				DisplayError("Unrecognized argument '%s'", txt)
				os.Exit(1)
			}

			expargs = append(expargs, txt)

			lastLevel = txt
			lastVisit = ""
			isExplore = true

			continue
		}

		if isExplore {

			expargs = append(expargs, txt)

			// get last component of parent/child or multi-level exploration path
			if strings.Contains(txt, "/") && !strings.Contains(txt, ".") {
				txt = strings.Replace(txt, "/", ".", -1)
			}
			lstidx := strings.LastIndex(txt, ".")
			if lstidx >= 0 {
				txt = txt[lstidx+1:]
			}

			lastVisit = txt
			isExplore = false

			continue
		}

		// -pkg adds additional exploration level
		if txt == "-pkg" && lastLevel != "" && lastVisit != "" && lastVisit != "*" {
			expargs = append(expargs, lastLevel+"X")
			expargs = append(expargs, lastVisit)
		}

		expargs = append(expargs, txt)
	}

	// ParseArguments

	head := &Block{}

	for _, txt := range expargs {
		head.Working = append(head.Working, txt)
	}

	// initial parsing of exploration command structure
	parseCommands(head, PATTERN)

	if len(head.Subtasks) != 1 {
		return nil
	}

	// skip past empty placeholder
	head = head.Subtasks[0]

	// convert command strings to array of operations for faster processing
	parseOperations(head)

	// check for no -element or multiple -pattern commands
	noElement := true
	noClose := true
	numPatterns := 0
	for _, txt := range expargs {
		if argTypeIs[txt] == EXTRACTION {
			noElement = false
		}
		if txt == "-pattern" || txt == "-Pattern" {
			numPatterns++
		} else if txt == "-select" {
			noElement = false
			head.Position = "select"
		} else if txt == "-cls" || txt == "-slf" {
			noClose = false
		}
	}

	if numPatterns < 1 {
		DisplayError("No -pattern in command-line arguments")
		os.Exit(1)
	}

	if numPatterns > 1 {
		DisplayError("Only one -pattern command is permitted")
		os.Exit(1)
	}

	if noElement && noClose {
		DisplayError("No -element statement in argument list")
		os.Exit(1)
	}

	return head
}

// PrettyArguments indents xtract arguments by exploration level
func PrettyArguments(cmdargs []string) {

	levels := make(map[LevelType]bool)

	hasLeading := false
	isFirst := true

	for _, str := range cmdargs {

		ch, ok := HasLeadingUnicodeDash(str)
		if ok {
			DisplayError("Unicode dash %d replaced expected ASCII hyphen in '%s'", ch, str)
			os.Exit(1)
		}

		lev, ok := levTypeIs[str]
		if ok {
			levels[lev] = true
		} else if isFirst {
			hasLeading = true
		}

		isFirst = false
	}

	indents := make(map[LevelType]string)

	lev := PATTERN
	spaces := "                                        "
	indent := 0
	if hasLeading {
		indent += 2
	}

	for {
		if lev < UNIT {
			break
		}

		if levels[lev] {
			indents[lev] = spaces[0:indent]
			indent += 2
		}

		lev--
	}

	isFirst = true
	quote := "\""

	needsQuotes := func(str string) bool {

		for _, ch := range str {
			if ch == ' ' || ch == '*' || ch == '&' || ch == '\\' {
				return true
			}
		}

		return false
	}

	fmt.Fprintf(os.Stdout, "\nxtract")
	for _, str := range cmdargs {

		lev, ok := levTypeIs[str]
		if ok {
			if isFirst && lev == PATTERN {
				fmt.Fprintf(os.Stdout, " %s", str)
			} else {
				spc := indents[lev]
				fmt.Fprintf(os.Stdout, " \\\n%s%s", spc, str)
			}
		} else if needsQuotes(str) {
			fmt.Fprintf(os.Stdout, " %s", quote+str+quote)
		} else {
			fmt.Fprintf(os.Stdout, " %s", str)
		}

		isFirst = false
	}
	fmt.Fprintf(os.Stdout, "\n\n")
}

// printXMLtree supports XML compression styles selected by -element "*" through "****"
func printXMLtree(node *XMLNode, style IndentType, mask string, printAttrs bool, proc func(string)) {

	if node == nil || proc == nil {
		return
	}

	// WRAPPED is SUBTREE plus each attribute on its own line
	wrapped := false
	if style == WRAPPED {
		style = SUBTREE
		wrapped = true
	}

	// INDENT is offset by two spaces to allow for parent tag, SUBTREE is not offset
	initial := 1
	if style == SUBTREE {
		style = INDENT
		initial = 0
	}

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
			proc("                    ")
			i -= 10
		}
		if i < 0 {
			return
		}
		proc(indentSpaces[i])
	}

	// xtract -mixed -pattern article -mask "table-wrap,alternatives,inline-formula" -element "*"
	masked := make(map[string]bool)

	comma := strings.Split(mask, ",")
	for _, msk := range comma {
		masked[msk] = true
	}

	// doSubtree recursive definition
	var doSubtree func(*XMLNode, int)

	doSubtree = func(curr *XMLNode, depth int) {

		// suppress if it would be an empty self-closing tag
		if !IsNotJustWhitespace(curr.Attributes) && curr.Contents == "" && curr.Children == nil {
			return
		}

		if style == INDENT {
			doIndent(depth)
		}

		if curr.Name != "" {
			proc("<")
			proc(curr.Name)

			if printAttrs {

				attr := strings.TrimSpace(curr.Attributes)
				attr = CompressRunsOfSpaces(attr)

				if attr != "" {

					if wrapped {

						start := 0
						idx := 0

						attlen := len(attr)

						for idx < attlen {
							ch := attr[idx]
							if ch == '=' {
								str := attr[start:idx]
								proc("\n")
								doIndent(depth)
								proc(" ")
								proc(str)
								// skip past equal sign and leading double quote
								idx += 2
								start = idx
							} else if ch == '"' || ch == '\'' {
								str := attr[start:idx]
								proc("=\"")
								proc(str)
								proc("\"")
								// skip past trailing double quote and (possible) space
								idx += 2
								start = idx
							} else {
								idx++
							}
						}

						proc("\n")
						doIndent(depth)

					} else {

						proc(" ")
						proc(attr)
					}
				}
			}

			// see if suitable for for self-closing tag
			if curr.Contents == "" && curr.Children == nil {
				proc("/>")
				if style != COMPACT {
					proc("\n")
				}
				return
			}

			proc(">")
		}

		if curr.Contents != "" {

			proc(curr.Contents[:])

		} else {

			if style != COMPACT {
				proc("\n")
			}

			for chld := curr.Children; chld != nil; chld = chld.Next {
				// use -mask to selectively skip descent into container
				if mask != "" && masked[chld.Name] {
					continue
				}
				doSubtree(chld, depth+1)
			}

			if style == INDENT {
				i := depth
				for i > 9 {
					proc("                    ")
					i -= 10
				}
				proc(indentSpaces[i])
			}
		}

		if curr.Name != "" {
			proc("<")
			proc("/")
			proc(curr.Name)
			proc(">")
		}

		if style != COMPACT {
			proc("\n")
		}
	}

	doSubtree(node, initial)
}

// printASNtree prints ASN.1 selected by -element "."
func printASNtree(node *XMLNode, proc func(string)) {

	if node == nil || proc == nil {
		return
	}

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
			proc("                    ")
			i -= 10
		}
		if i < 0 {
			return
		}
		proc(indentSpaces[i])
	}

	afix := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
		"&apos;", "'",
		"&#39;", "'",
		"&quot;", "'",
		"&#34;", "'",
		"\"", "'",
	)

	// doASNtree recursive definition
	var doASNtree func(*XMLNode, int, bool)

	doASNtree = func(curr *XMLNode, depth int, comma bool) {

		// suppress if it would be an empty self-closing tag
		if !IsNotJustWhitespace(curr.Attributes) && curr.Contents == "" && curr.Children == nil {
			return
		}

		name := curr.Name
		if name == "" {
			name = "_"
		}

		// just a hyphen   - unnamed braces (element of SEQUENCE OF or SET OF structured objects)
		// trailing hyphen - unquoted value
		// internal hyphen - convert to space
		show := true
		quot := true
		if name == "_" {
			show = false
		} else if strings.HasPrefix(name, "_") {
			// if leading hyphen, ignore remainder of name
			show = false
		} else if strings.HasSuffix(name, "_") {
			name = strings.TrimSuffix(name, "_")
			quot = false
		}
		name = strings.Replace(name, "_", " ", -1)
		name = strings.TrimSpace(name)

		if curr.Contents != "" {

			doIndent(depth)
			proc(name)
			proc(" ")

			if quot {
				proc("\"")
			}

			str := curr.Contents[:]
			if HasBadSpace(str) {
				str = CleanupBadSpaces(str)
			}
			if IsNotASCII(str) {
				str = TransformAccents(str, false, false)
			}
			if HasAdjacentSpaces(str) {
				str = CompressRunsOfSpaces(str)
			}
			str = afix.Replace(str)
			proc(str)

			if quot {
				proc("\"")
			}

		} else {

			doIndent(depth)
			if show {
				proc(name)
				proc(" ")
			}
			if depth == 0 {
				proc("::= ")
			}
			proc("{\n")

			for chld := curr.Children; chld != nil; chld = chld.Next {
				// do not print comma after last child object in chain
				doASNtree(chld, depth+1, (chld.Next != nil))
			}

			doIndent(depth)
			proc("}")
		}

		if comma {
			proc(",")
		}
		proc("\n")
	}

	doASNtree(node, 0, false)
}

// printJSONtree prints JSON selected by -element "%"
func printJSONtree(node *XMLNode, proc func(string)) {

	// COPIED FROM printASNtree, MODIFICATIONS NOT YET COMPLETE

	if node == nil || proc == nil {
		return
	}

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
			proc("                    ")
			i -= 10
		}
		if i < 0 {
			return
		}
		proc(indentSpaces[i])
	}

	// doJSONtree recursive definition
	var doJSONtree func(*XMLNode, int, bool)

	doJSONtree = func(curr *XMLNode, depth int, comma bool) {

		// suppress if it would be an empty self-closing tag
		if !IsNotJustWhitespace(curr.Attributes) && curr.Contents == "" && curr.Children == nil {
			return
		}

		name := curr.Name
		if name == "" {
			name = "_"
		}

		// just a hyphen   - unnamed brackets
		// leading hyphen  - array instead of object
		// trailing hyphen - unquoted value
		show := true
		array := false
		quot := true
		if name == "_" {
			show = false
		} else if strings.HasPrefix(name, "_") {
			array = true
		} else if strings.HasSuffix(name, "_") {
			name = strings.TrimSuffix(name, "_")
			quot = false
		}
		name = strings.Replace(name, "_", " ", -1)
		name = strings.TrimSpace(name)

		if curr.Contents != "" {

			doIndent(depth)
			proc("\"")
			proc(name)
			proc("\": ")

			if quot {
				proc("\"")
			}

			str := curr.Contents[:]
			if HasBadSpace(str) {
				str = CleanupBadSpaces(str)
			}
			if IsNotASCII(str) {
				str = TransformAccents(str, false, false)
			}
			if HasAdjacentSpaces(str) {
				str = CompressRunsOfSpaces(str)
			}
			proc(str)

			if quot {
				proc("\"")
			}

		} else {

			doIndent(depth)
			if show && depth > 0 {
				proc("\"")
				proc(name)
				proc("\": ")
			}
			if array {
				proc("[")
			} else {
				proc("{")
			}
			proc("\n")

			for chld := curr.Children; chld != nil; chld = chld.Next {
				// do not print comma after last child object in chain
				doJSONtree(chld, depth+1, (chld.Next != nil))
			}

			doIndent(depth)
			if array {
				proc("]")
			} else {
				proc("}")
			}
		}

		if comma {
			proc(",")
		}
		proc("\n")
	}

	doJSONtree(node, 0, false)
}
