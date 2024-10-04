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
// File Name:  trie.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"path/filepath"
	"strings"
	"unicode"
)

// LinkLen controls the number of directory levels for link terms
const LinkLen = 4

// TrieLen directory depth parameters are based on the observed size distribution of PubMed indices
var TrieLen = map[string]int{
	"17": 4,
	"18": 4,
	"19": 4,
	"20": 4,
	"a1": 3,
	"ab": 3,
	"ac": 4,
	"ad": 3,
	"af": 4,
	"ag": 3,
	"al": 3,
	"an": 4,
	"ap": 4,
	"ar": 3,
	"as": 4,
	"b0": 3,
	"ba": 4,
	"be": 4,
	"bi": 3,
	"br": 3,
	"c0": 3,
	"c1": 3,
	"ca": 4,
	"ce": 4,
	"ch": 4,
	"cl": 4,
	"co": 4,
	"cr": 3,
	"cy": 3,
	"d0": 4,
	"d1": 4,
	"d2": 3,
	"da": 4,
	"de": 4,
	"di": 4,
	"do": 3,
	"dr": 3,
	"e0": 3,
	"ef": 4,
	"el": 4,
	"en": 4,
	"ev": 3,
	"ex": 4,
	"fa": 3,
	"fe": 4,
	"fi": 3,
	"fo": 4,
	"fr": 4,
	"fu": 4,
	"g0": 3,
	"ge": 4,
	"gr": 4,
	"he": 4,
	"hi": 4,
	"ho": 4,
	"hu": 4,
	"hy": 4,
	"jo": 4,
	"im": 3,
	"in": 4,
	"la": 3,
	"le": 3,
	"li": 3,
	"lo": 3,
	"ma": 3,
	"me": 4,
	"mi": 3,
	"mo": 4,
	"mu": 3,
	"mz": 3,
	"n0": 3,
	"ne": 3,
	"no": 4,
	"ob": 3,
	"on": 3,
	"oz": 3,
	"pa": 4,
	"pe": 4,
	"ph": 3,
	"pl": 4,
	"po": 4,
	"pr": 4,
	"pu": 4,
	"ra": 3,
	"re": 4,
	"ri": 3,
	"ro": 4,
	"rz": 3,
	"sa": 4,
	"sc": 4,
	"se": 3,
	"si": 4,
	"so": 4,
	"sp": 4,
	"st": 4,
	"su": 4,
	"sy": 4,
	"te": 3,
	"th": 3,
	"ti": 3,
	"to": 4,
	"tr": 4,
	"tw": 4,
	"un": 3,
	"va": 3,
	"ve": 3,
	"vi": 3,
	"we": 3,
	"wh": 3,
}

// MergLen directory depth parameters are based on the observed size distribution of PubMed indices
var MergLen = map[string]int{
	"ana": 4,
	"app": 4,
	"ass": 4,
	"can": 4,
	"cas": 4,
	"cha": 4,
	"cli": 4,
	"com": 4,
	"con": 4,
	"d00": 4,
	"d01": 4,
	"d02": 4,
	"d12": 4,
	"dam": 4,
	"dat": 4,
	"dec": 4,
	"ded": 4,
	"del": 4,
	"dem": 4,
	"dep": 4,
	"des": 4,
	"det": 4,
	"dev": 4,
	"dif": 4,
	"dis": 4,
	"eff": 4,
	"enf": 4,
	"exp": 4,
	"for": 4,
	"gen": 4,
	"gro": 4,
	"hea": 4,
	"hig": 4,
	"hum": 4,
	"inc": 4,
	"ind": 4,
	"int": 4,
	"inv": 4,
	"met": 4,
	"mod": 4,
	"pat": 4,
	"per": 4,
	"pre": 4,
	"pro": 4,
	"pub": 4,
	"rel": 4,
	"rep": 4,
	"res": 4,
	"sig": 4,
	"sta": 4,
	"str": 4,
	"stu": 4,
	"tre": 4,
}

func getPrefixAndPadID(id string) (string, string) {

	// "2539356"

	if len(id) > 64 {
		return "", id
	}

	str := id

	if IsAllDigits(str) {

		// pad numeric identifier to 8 characters with leading zeros
		ln := len(str)
		if ln < 8 {
			zeros := "00000000"
			str = zeros[ln:] + str
		}
	}

	// "02539356"

	if IsAllDigitsOrPeriod(str) {

		// limit trie to first 6 characters
		if len(str) > 6 {
			str = str[:6]
		}
	}

	// "025393"

	max := 4
	k := 0
	for _, ch := range str {
		if unicode.IsLetter(ch) {
			k++
			continue
		}
		if ch == '_' {
			k++
			max = 6
		}
		break
	}

	// prefix is up to three letters if followed by digits,
	// or up to four letters if followed by an underscore
	pfx := str[:k]
	if len(pfx) < max {
		str = str[k:]
	} else {
		pfx = ""
	}

	// "", "025393"

	return pfx, str
}

// PadNumericID returns 8-character leading zero-padded numeric identifier
func PadNumericID(id string) string {

	// "2539356"

	if len(id) > 64 {
		return id
	}

	str := id

	if IsAllDigits(str) {

		// pad numeric identifier to 8 characters with leading zeros
		ln := len(str)
		if ln < 8 {
			zeros := "00000000"
			str = zeros[ln:] + str
		}
	}

	// "02539356"

	return str
}

// ArchiveTrie allows a short prefix of letters with an optional underscore,
// and splits the remainder into character pairs
func ArchiveTrie(id string) (string, string) {

	// "2539356"

	if len(id) > 64 {
		return "", ""
	}

	pfx, str := getPrefixAndPadID(id)

	var arry [132]rune

	i := 0

	if pfx != "" {
		for _, ch := range pfx {
			arry[i] = ch
			i++
		}
		arry[i] = '/'
		i++
	}

	between := 0
	doSlash := false

	// remainder is divided in character pairs, e.g., NP_/06/00/51 for NP_060051.2
	for _, ch := range str {
		// break at period separating accession from version
		if ch == '.' {
			break
		}
		if doSlash {
			arry[i] = '/'
			i++
			doSlash = false
		}
		if ch == ' ' {
			ch = '_'
		}
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			ch = '_'
		}
		arry[i] = ch
		i++
		between++
		if between > 1 {
			doSlash = true
			between = 0
		}
	}

	res := string(arry[:i])

	if !strings.HasSuffix(res, "/") {
		arry[i] = '/'
		i++
		res = string(arry[:i])
	}

	// "02/53/93/", "2539356"

	return strings.ToUpper(res), id
}

// IndexTrie generates the path and file name for first-level incremental index files
func IndexTrie(id string) (string, string) {

	// "2539356"

	if len(id) > 64 {
		return "", ""
	}

	pfx, str := getPrefixAndPadID(id)

	// "02539356"

	var arry [132]rune

	i := 0

	if pfx != "" {
		for _, ch := range pfx {
			arry[i] = ch
			i++
		}
		arry[i] = '/'
		i++
	}

	between := 0
	doSlash := false

	// remainder is divided in character pairs, e.g., NP_/06/00/51 for NP_060051.2
	for _, ch := range str {
		// break at period separating accession from version
		if ch == '.' {
			break
		}
		if doSlash {
			arry[i] = '/'
			i++
			doSlash = false
		}
		if ch == ' ' {
			ch = '_'
		}
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			ch = '_'
		}
		arry[i] = ch
		i++
		between++
		if between > 1 {
			doSlash = true
			between = 0
		}
	}

	// trim back one subdirectory level from Archive for Index
	i -= 3

	res := string(arry[:i])

	if !strings.HasSuffix(res, "/") {
		arry[i] = '/'
		i++
		res = string(arry[:i])
	}

	// return the 6-digit .e2x file name for precomputed incremental
	// index cached for all .xml records in 10 adjacent leaf folders
	if IsAllDigits(id) {

		// pad numeric identifier to 8 characters with leading zeros
		ln := len(id)
		if ln < 8 {
			zeros := "00000000"
			id = zeros[ln:] + id
		}
	}

	idx := id

	if IsAllDigitsOrPeriod(id) {

		// limit index trie to first 6 characters
		if len(idx) > 6 {
			idx = idx[:6]
		}
	}

	// "02/53/", "025393"

	return strings.ToUpper(res), idx
}

// InvertTrie generates the path and file name for second-level inverted index files
func InvertTrie(id string) (string, string) {

	if len(id) > 64 {
		return "", ""
	}

	// "2539356"

	dir, inv := IndexTrie(id)

	// "02/53/", "025393"

	// limit invert directory to first 3 characters
	if len(dir) > 3 {
		dir = dir[:3]
	}
	// limit invert trie to first 4 characters
	if len(inv) > 4 {
		inv = inv[:4]
	}

	// "02/", "0253"

	return dir, inv
}

// PostingDir returns directory trie (without slashes) for location of indices for a given term
func PostingDir(term string) string {

	if len(term) < 3 {
		return term
	}

	key := term[:2]

	num, ok := TrieLen[key]
	if ok && len(term) >= num {
		return term[:num]
	}

	switch term[0] {
	case 'u', 'v', 'w', 'x', 'y', 'z':
		return term[:2]
	}

	return term[:3]
}

// IdentifierKey cleans up a term then returns the posting directory
func IdentifierKey(term string) string {

	// remove punctuation from term
	key := strings.Map(func(c rune) rune {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != ' ' && c != '-' && c != '_' {
			return -1
		}
		return c
	}, term)

	key = strings.Replace(key, " ", "_", -1)
	key = strings.Replace(key, "-", "_", -1)

	// use first 2, 3, or 4 characters of identifier for directory
	key = PostingDir(key)

	return key
}

// PostingsTrie splits a string into characters, separated by path delimiting slashes
func PostingsTrie(term string) (string, string) {

	// "cancer"

	if len(term) > 256 {
		return "", term
	}

	// use first few characters of identifier for directory
	key := IdentifierKey(term)

	str := key

	var arry [516]rune

	if IsNotASCII(str) {
		// expand Greek letters, anglicize characters in other alphabets
		str = TransformAccents(str, true, true)
	}
	if HasAdjacentSpaces(str) {
		str = CompressRunsOfSpaces(str)
	}
	str = strings.TrimSpace(str)

	i := 0
	doSlash := false

	for _, ch := range str {
		if doSlash {
			arry[i] = '/'
			i++
		}
		if ch == ' ' {
			ch = '_'
		}
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			ch = '_'
		}
		arry[i] = ch
		i++
		doSlash = true
	}

	// "c/a/n/c", "canc"

	return strings.ToLower(string(arry[:i])), key
}

// PostingPath constructs a Postings directory subpath for a given term prefix
func PostingPath(prom, field, term string, isLink bool) (string, string) {

	// "/Volumes/archive/pubmed/Postings", "TIAB", "cancer"

	if isLink {
		dir, _ := LinksTrie(term, false)
		if dir == "" {
			return "", ""
		}
		dpath := filepath.Join(prom, field, dir)

		return dpath, term
	}

	// use first few characters of identifier for directory
	key := IdentifierKey(term)

	dir, _ := PostingsTrie(term)
	if dir == "" {
		return "", ""
	}

	dpath := filepath.Join(prom, field, dir)

	// "/Volumes/archive/pubmed/Postings/TIAB/c/a/n/c", "canc"

	return dpath, key
}

// LinksTrie generates the path and file name for link inverted index files
func LinksTrie(id string, pad bool) (string, string) {

	// "2539356"

	if len(id) > 64 {
		return "", id
	}

	str := id

	if pad {

		// true from ProcessLinks, false from PostingPath (indexed link terms are already zero-padded)

		str = PadNumericID(id)

		// "02539356"
	}

	// links always use 4 directory levels of padded identifiers, grouped numerically,
	// so ProcessLinks may be able to get multiple nearby links with fewer reads
	if len(str) > LinkLen {
		str = str[:LinkLen]
	}

	// "0253"

	var arry [132]rune

	i := 0
	doSlash := false

	for _, ch := range str {
		if doSlash {
			arry[i] = '/'
			i++
		}
		if ch == ' ' {
			ch = '_'
		}
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			ch = '_'
		}
		arry[i] = ch
		i++
		doSlash = true
	}

	res := string(arry[:i])

	if !strings.HasSuffix(res, "/") {
		arry[i] = '/'
		i++
		res = string(arry[:i])
	}

	// "0/2/5/3/", "0253" (pad true)

	return strings.ToUpper(res), str
}
