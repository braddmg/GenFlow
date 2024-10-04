// prep-taxoninfo.go

// Public domain notice for all NCBI EDirect scripts is located at:
// https://www.ncbi.nlm.nih.gov/books/NBK179288/#chapter6.Public_Domain_Notice

package main

import (
	"bufio"
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// copied from EDirect's eutils library

// Adding "replace eutils => ../eutils" to go.mod works for a ".go" program compiled
// with "go build", but not when called with "go run" as if it were a script, so these
// helpers for converting external data formats duplicate a few EDirect functions

var greekRunes = map[rune]string{
	0x0190: "epsilon",
	0x025B: "epsilon",
	0x0391: "alpha",
	0x0392: "beta",
	0x0393: "gamma",
	0x0394: "delta",
	0x0395: "epsilon",
	0x0396: "zeta",
	0x0397: "eta",
	0x0398: "theta",
	0x0399: "iota",
	0x039A: "kappa",
	0x039B: "lambda",
	0x039C: "mu",
	0x039D: "nu",
	0x039E: "xi",
	0x039F: "omicron",
	0x03A0: "pi",
	0x03A1: "rho",
	0x03A3: "sigma",
	0x03A4: "tau",
	0x03A5: "upsilon",
	0x03A6: "phi",
	0x03A7: "chi",
	0x03A8: "psi",
	0x03A9: "omega",
	0x03B1: "alpha",
	0x03B2: "beta",
	0x03B3: "gamma",
	0x03B4: "delta",
	0x03B5: "epsilon",
	0x03B6: "zeta",
	0x03B7: "eta",
	0x03B8: "theta",
	0x03B9: "iota",
	0x03BA: "kappa",
	0x03BB: "lambda",
	0x03BC: "mu",
	0x03BD: "nu",
	0x03BE: "xi",
	0x03BF: "omicron",
	0x03C0: "pi",
	0x03C1: "rho",
	0x03C2: "sigma",
	0x03C3: "sigma",
	0x03C4: "tau",
	0x03C5: "upsilon",
	0x03C6: "phi",
	0x03C7: "chi",
	0x03C8: "psi",
	0x03C9: "omega",
	0x03D0: "beta",
	0x03D1: "theta",
	0x03D5: "phi",
	0x03D6: "pi",
	0x03F0: "kappa",
	0x03F1: "rho",
	0x03F5: "epsilon",
	0x1D5D: "beta",
	0x1D66: "beta",
}

// rune table from data file
var asciiRunes map[rune]string

// parser character type lookup tables
var inBlank [256]bool

// reencodes < and > to &lt and &gt, and & to &amp
var rfix *strings.Replacer

// reencodes < and > to &lt and &gt, and & to &amp, and converts ' and " to space
var tfix *strings.Replacer

// ANSI escape codes for terminal color, highlight, and reverse
const (
	RED  = "\033[31m"
	BLUE = "\033[34m"
	BOLD = "\033[1m"
	RVRS = "\033[7m"
	INIT = "\033[0m"
	LOUD = INIT + RED + BOLD
	INVT = LOUD + RVRS
)

func displayError(format string, params ...any) {

	str := fmt.Sprintf(format, params...)
	fmt.Fprintf(os.Stderr, "\n%s ERROR: %s %s%s\n", INVT, LOUD, str, INIT)
}

func displayWarning(format string, params ...any) {

	str := fmt.Sprintf(format, params...)
	fmt.Fprintf(os.Stderr, "\n%s WARNING: %s %s%s\n", INVT, LOUD, str, INIT)
}

func isAllDigits(str string) bool {

	for _, ch := range str {
		if !unicode.IsDigit(ch) {
			return false
		}
	}

	return true
}

func compressRunsOfSpaces(str string) string {

	whiteSpace := false
	var buffer strings.Builder

	for _, ch := range str {
		if ch < 127 && inBlank[ch] {
			if !whiteSpace {
				buffer.WriteRune(' ')
			}
			whiteSpace = true
		} else {
			buffer.WriteRune(ch)
			whiteSpace = false
		}
	}

	return buffer.String()
}

func loadRuneTable(rt map[rune]string, dataPath, fileName string) bool {

	loaded := false

	fpath := filepath.Join(dataPath, fileName)
	file, ferr := os.Open(fpath)

	if file != nil && ferr == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {

			str := scanner.Text()
			if str == "" {
				continue
			}
			cols := strings.SplitN(str, "\t", 3)
			if len(cols) < 2 {
				continue
			}
			n, err := strconv.ParseUint(cols[0], 16, 32)
			if err != nil {
				continue
			}

			ch := rune(n)
			st := cols[1]
			rt[ch] = st

			loaded = true
		}
	}
	file.Close()

	if !loaded {
		displayWarning("Unable to load %s", fileName)
	}

	return loaded
}

func transformAccents(str string, spellGreek bool) string {

	var arry []string

	for _, ch := range str {

		st := ""
		ok := false

		if ch < 128 {
			// add printable 7-bit ASCII character directly
			if ch > 31 {
				arry = append(arry, string(ch))
			}
			continue
		}

		// leading and trailing spaces, if needed, are in the xxxRunes maps

		if spellGreek {
			// spells Greek letters (e.g., alpha, beta) for easier searching,
			// handles glyph variants, treats Latin letter open E as Greek epsilon
			st, ok = greekRunes[ch]
			if ok {
				arry = append(arry, st)
				continue
			}
		}

		// lookup remaining characters in asciiRunes table
		st, ok = asciiRunes[ch]
		if ok {
			arry = append(arry, st)
		}
	}

	str = strings.Join(arry, "")

	return str
}

// initialize maps, tables, and variables
func init() {

	asciiRunes = make(map[rune]string)

	for i := range inBlank {
		inBlank[i] = false
	}
	inBlank[' '] = true
	inBlank['\t'] = true
	inBlank['\n'] = true
	inBlank['\r'] = true
	inBlank['\f'] = true

	rfix = strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"&", "&amp;",
	)

	tfix = strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"&", "&amp;",
		"'", " ",
		"\"", " ",
	)
}

// TaxCodes holds nuclear, mitochondrial, plastid, and hydrogenosome genetic codes
type TaxCodes struct {
	Nuclear       string
	Mitochondrial string
	Plastid       string
	Hydrogenosome string
}

// TaxNames has several kinds of alternative names for a given node
type TaxNames struct {
	Common     []string
	GenBank    []string
	Synonym    []string
	Authority  []string
	Equivalent []string
	Includes   []string
	Acronym    []string
	Other      []string
}

// TaxLevels has fields for well-established taxonomic ranks
type TaxLevels struct {
	Species string
	Genus   string
	Family  string
	Order   string
	Class   string
	Phylum  string
	Kingdom string
	Domain  string // superkingdom
}

// TaxMods has fields for Org-ref.OrgName.OrgMod qualifiers
type TaxMods struct {
	Clade      string
	Strain     string
	Subspecies string
	Substrain  string
	Serovar    string
	Note       string
}

// TaxType allows recording of type materials, etc. (see PMC4383940)
type TaxType struct {
	Type string
	Name string
}

// TaxonInfo is the master structure for archived, indexed taxonomy record
type TaxonInfo struct {
	TaxID      string
	Scientific string
	Rank       string
	Division   string
	Lineage    string
	ParentID   string
	Codes      TaxCodes
	Names      TaxNames
	Levels     TaxLevels
	Mods       TaxMods
	Specimens  []TaxType
	Children   []string
}

// createTAXONINFO reads taxonomy files and create TaxonInfo records
func createTAXONINFO(verboseAncestors, verboseChildren bool) int {

	recordCount := 0

	// master map of taxon objects
	taxonInfoMap := make(map[string]*TaxonInfo)

	// GenBank division code lookup
	divCodes := make(map[string]string)

	// common function to handle reading of tab-delimited lines from taxonomy release files
	processTableFile := func(fname string, expected int, proc func([]string)) {

		if proc == nil {
			return
		}

		inFile, err := os.Open(fname)
		if err != nil {
			displayError("Unable to open file %s - %s", fname, err.Error())
			os.Exit(1)
		}

		defer inFile.Close()

		scanr := bufio.NewScanner(inFile)

		lineNum := 0

		for scanr.Scan() {

			lineNum++
			line := scanr.Text()
			if line == "" {
				continue
			}

			cols := strings.Split(line, "\t")
			if len(cols) != expected {
				displayError("Found %d columns in line %d of file %s", len(cols), lineNum, fname)
				continue
			}

			// send columns to indicated callback
			proc(cols)
		}
	}

	readDivisions := func(cols []string) {

		divID, divCode, divName := cols[0], cols[2], cols[4]

		if divID == "" || divCode == "" || divName == "" {
			return
		}

		// looks up 3-letter division abbreviation from 1-digit number
		divCodes[divID] = divCode
		// overload to look up division name from 3-letter abbreviation
		divCodes[divCode] = divName
	}

	readNameTable := func(cols []string) {

		taxID, taxName, nameClass := cols[0], cols[2], cols[6]

		if taxID == "" || taxName == "" || nameClass == "" {
			return
		}

		tn, ok := taxonInfoMap[taxID]
		if !ok {
			// composite literal creates new node, initializes with taxonomy UID
			tn = &TaxonInfo{TaxID: taxID}
			// save new node in map
			taxonInfoMap[taxID] = tn
		}
		if tn == nil {
			return
		}

		if nameClass == "scientific name" {
			// majority of lines are scientific name
			tn.Scientific = taxName
			return
		}

		// take address of composite structure in order to modify data in map
		nam := &tn.Names

		switch nameClass {
		case "common name":
			nam.Common = append(nam.Common, taxName)
		case "genbank common name":
			nam.GenBank = append(nam.GenBank, taxName)
		case "synonym":
			nam.Synonym = append(nam.Synonym, taxName)
		case "equivalent name":
			nam.Equivalent = append(nam.Equivalent, taxName)
		case "includes":
			nam.Includes = append(nam.Includes, taxName)
		case "authority":
			nam.Authority = append(nam.Authority, taxName)
		case "acronym", "genbank acronym":
			nam.Acronym = append(nam.Acronym, taxName)
		default:
			// "in-part" and "blast name"
			nam.Other = append(nam.Other, taxName)
		}
	}

	readFullLineage := func(cols []string) {

		taxID, taxName, lineage := cols[0], cols[2], cols[4]

		if taxID == "" || taxName == "" || lineage == "" {
			return
		}

		tn, ok := taxonInfoMap[taxID]
		if !ok {
			return
		}

		sci := strings.ToLower(tn.Scientific)
		sci = transformAccents(sci, true)
		txn := strings.ToLower(taxName)
		txn = transformAccents(txn, true)

		if sci != txn {
			displayError("FullTax Mismatch in TaxID %s\n%s - %s", tn.TaxID, sci, txn)
			return
		}

		lineage = strings.TrimSuffix(lineage, "; ")
		tn.Lineage = lineage
	}

	readRankedLineage := func(cols []string) {

		taxID, taxName, species, genus, family := cols[0], cols[2], cols[4], cols[6], cols[8]
		order, class, phylum, kingdom, domain := cols[10], cols[12], cols[14], cols[16], cols[18]

		if taxID == "" || taxName == "" {
			return
		}

		tn, ok := taxonInfoMap[taxID]
		if !ok {
			return
		}

		sci := strings.ToLower(tn.Scientific)
		sci = transformAccents(sci, true)
		txn := strings.ToLower(taxName)
		txn = transformAccents(txn, true)
		if sci != txn {
			displayError("RankTax Mismatch in TaxID %s\n%s - %s", tn.TaxID, sci, txn)
			return
		}

		// species adjustments
		if species == "" && genus != "" && strings.HasPrefix(tn.Scientific, genus+" ") {

			// e.g., genus "Homo", scientific name "Homo sapiens", construct species - 9606
			species = tn.Scientific
			species = strings.TrimPrefix(species, genus+" ")

		} else if species != "" && genus != "" && strings.HasPrefix(species, genus+" ") {

			// e.g., species "Homo sapiens", genus "Homo", remove prefix from species - 63221, 741158
			species = strings.TrimPrefix(species, genus+" ")
		}
		species = strings.TrimSuffix(species, " environmental sample")
		species = strings.TrimSpace(species)

		// take address to modify data in map
		rnk := &tn.Levels

		rnk.Species = species
		rnk.Genus = genus
		rnk.Family = family
		rnk.Order = order
		rnk.Class = class
		rnk.Phylum = phylum
		rnk.Kingdom = kingdom
		rnk.Domain = domain
	}

	readNodeTable := func(cols []string) {

		taxID, parentID, rank, divID := cols[0], cols[2], cols[4], cols[8]
		nucCode, mitoCode, plastCode, hydroCode := cols[12], cols[16], cols[26], cols[32]

		if taxID == "" {
			return
		}

		tn, ok := taxonInfoMap[taxID]
		if !ok {
			return
		}

		tn.ParentID = parentID

		tn.Rank = rank

		pn, ok := taxonInfoMap[parentID]
		if ok {
			pn.Children = append(pn.Children, taxID)
		}

		// division code 0 is Bacteria, only skip on empty string
		if divID != "" {
			code, ok := divCodes[divID]
			if ok {
				tn.Division = code
			}
		}

		// normalize 0 to empty string for genetic codes
		if nucCode == "0" {
			nucCode = ""
		}
		if mitoCode == "0" {
			mitoCode = ""
		}
		if plastCode == "0" {
			plastCode = ""
		}
		if hydroCode == "0" {
			hydroCode = ""
		}

		// take address to modify data in map
		gcs := &tn.Codes

		gcs.Nuclear = nucCode
		gcs.Mitochondrial = mitoCode
		gcs.Plastid = plastCode
		gcs.Hydrogenosome = hydroCode

		// take address to modify data in map
		rnk := &tn.Levels

		// fill in level of current rank for [TREE] construction
		switch tn.Rank {
		case "species":
			if rnk.Species == "" {
				rnk.Species = tn.Scientific
			}
		case "genus":
			if rnk.Genus == "" {
				rnk.Genus = tn.Scientific
			}
		case "family":
			if rnk.Family == "" {
				rnk.Family = tn.Scientific
			}
		case "order":
			if rnk.Order == "" {
				rnk.Order = tn.Scientific
			}
		case "class":
			if rnk.Class == "" {
				rnk.Class = tn.Scientific
			}
		case "phylum":
			if rnk.Phylum == "" {
				rnk.Phylum = tn.Scientific
			}
		case "kingdom":
			if rnk.Kingdom == "" {
				rnk.Kingdom = tn.Scientific
			}
		}

		trimFlanks := func(str, pfx string) string {

			/*
				if strings.HasPrefix(str, "'") && strings.HasSuffix(str, "'") {
					str = strings.TrimPrefix(str, "'")
					str = strings.TrimSuffix(str, "'")
				}
				if strings.HasPrefix(str, "(") && strings.HasSuffix(str, ")") {
					str = strings.TrimPrefix(str, "(")
					str = strings.TrimSuffix(str, ")")
				}
			*/
			if pfx != "" && strings.HasPrefix(str, pfx) {
				str = strings.TrimPrefix(str, pfx)
			}
			str = strings.TrimSpace(str)

			return str
		}

		mds := &tn.Mods

		// subspecies construction
		if tn.Rank == "subspecies" && rnk.Genus != "" && rnk.Species != "" {

			// e.g., scientific name "Homo sapiens neanderthalensis", genus "Homo", species "sapiens" - 63221
			subspecies := strings.TrimPrefix(tn.Scientific, rnk.Genus+" ")
			subspecies = strings.TrimPrefix(subspecies, rnk.Species+" ")
			subspecies = strings.TrimPrefix(subspecies, "subsp. ")
			subspecies = strings.TrimSpace(subspecies)

			mds.Subspecies = trimFlanks(subspecies, "subspecies ")
		}

		// parse additional modifiers, e.g.,
		// <Scientific>Prochlorococcus marinus subsp. pastoris str. NATL2 substr. M98C2B</Scientific> - 170609
		// <Scientific>Salmonella enterica subsp. enterica serovar Typhi str. E98-2068</Scientific> - 496068

		// tags are flanked by spaces to ensure correct matches
		ptrns := []string{" clade ", " serovar ", " sp. ", " str. ", " strain ", " subsp. ", " substr. "}

		var hits []int

		modname := tn.Scientific
		modname = strings.Replace(modname, "'", " ", -1)
		modname = strings.Replace(modname, "(", " ", -1)
		modname = strings.Replace(modname, ")", " ", -1)
		if rnk.Genus != "" && rnk.Species != "" {
			modname = strings.TrimPrefix(modname, rnk.Genus+" ")
			modname = strings.TrimPrefix(modname, rnk.Species+" ")
		}

		// record location of specific qualifier tags
		for _, md := range ptrns {
			pos := strings.Index(modname, md)
			if pos < 0 {
				continue
			}
			hits = append(hits, pos)
		}

		// sort by position in scientific name string
		slices.Sort(hits)

		var clauses []string

		// partition string into clauses
		lst := 0
		for _, pos := range hits {
			cls := modname[lst:pos]
			cls = strings.TrimSpace(cls)
			if cls != "" {
				clauses = append(clauses, cls)
			}
			lst = pos
		}
		// add clause after last matched position
		cls := modname[lst:]
		cls = strings.TrimSpace(cls)
		if cls != "" {
			clauses = append(clauses, cls)
		}

		clade, serovar, strain, subspecies, substrain, note := "", "", "", "", "", ""

		// set specific modifiers based on qualifier tag
		for _, cls := range clauses {

			lft, rgt, found := strings.Cut(cls, " ")
			if !found {
				continue
			}
			rgt = strings.TrimSpace(rgt)
			if rgt == "" {
				continue
			}

			switch lft {
			case "clade":
				clade = rgt
			case "serovar":
				serovar = rgt
			case "str.", "strain":
				strain = rgt
			case "subsp.":
				subspecies = rgt
			case "substr.":
				substrain = rgt
			case "sp.":
				// absorb
			default:
				note = rgt
			}
		}

		subspecies = strings.TrimPrefix(subspecies, "subsp ")
		subspecies = strings.TrimPrefix(subspecies, "subs ")

		if mds.Clade == "" {
			mds.Clade = trimFlanks(clade, "clade ")
		}
		if mds.Subspecies == "" {
			mds.Subspecies = trimFlanks(subspecies, "subspecies ")
		}
		if mds.Serovar == "" {
			mds.Serovar = trimFlanks(serovar, "serovar ")
		}
		if mds.Strain == "" {
			mds.Strain = trimFlanks(strain, "strain ")
		}
		if mds.Substrain == "" {
			mds.Substrain = trimFlanks(substrain, "substrain ")
		}

		// strain construction
		if tn.Rank == "strain" && mds.Strain == "" && rnk.Genus != "" && rnk.Species != "" {

			// e.g., scientific name "Escherichia coli K-12", genus "Escherichia", species "coli" - 83333
			strain := strings.TrimPrefix(modname, "str. ")
			strain = strings.TrimSpace(strain)

			mds.Strain = trimFlanks(strain, "strain ")
		}

		if mds.Note == "" {
			mds.Note = trimFlanks(note, "")
		}
	}

	readTypeMaterial := func(cols []string) {

		taxID, taxName, typ, ident := cols[0], cols[2], cols[4], cols[6]

		if taxID == "" || taxName == "" {
			return
		}

		tn, ok := taxonInfoMap[taxID]
		if !ok {
			return
		}

		mat := TaxType{Type: typ, Name: ident}
		tn.Specimens = append(tn.Specimens, mat)
	}

	uniqueStringArray := func(strs []string) []string {

		rs := make([]string, 0, len(strs))
		mp := make(map[string]bool)

		for _, val := range strs {
			// store lower-case version in map
			lc := strings.ToLower(val)
			_, ok := mp[lc]
			if !ok {
				mp[lc] = true
				rs = append(rs, val)
			}
		}

		return rs
	}

	// main body of function

	// perform taxonomy file processing steps
	processTableFile("division.dmp", 8, readDivisions)
	processTableFile("names.dmp", 8, readNameTable)
	processTableFile("fullnamelineage.dmp", 6, readFullLineage)
	processTableFile("rankedlineage.dmp", 20, readRankedLineage)
	processTableFile("nodes.dmp", 36, readNodeTable)
	processTableFile("typematerial.dmp", 8, readTypeMaterial)

	keys := slices.SortedFunc(maps.Keys(taxonInfoMap),
		func(i, j string) int {
			if isAllDigits(i) && isAllDigits(j) {
				// numeric sort on strings checks lengths first
				lni, lnj := len(i), len(j)
				// shorter string is numerically lower, assuming no leading zeros
				if lni < lnj {
					return -1
				}
				if lni > lnj {
					return 1
				}
				// same length allows comparison of numeric values without
				// needing expensive string-to-integer conversion
			}

			// same length or non-numeric, can now do string comparison on contents
			return cmp.Compare(i, j)
		})

	// buffer writes to stdout in chunks for efficiency
	var buffer strings.Builder
	count := 0
	okay := false

	wrtr := bufio.NewWriter(os.Stdout)

	// variables to control printing of top-level set tags flanking all records
	printSetHead := true
	printSetTail := false

	// variables to control printing of section tags flanking various content objects
	head := ""
	spaces := "  "
	fst := true

	printHead := func(name, tag, val string) {

		head = "  <" + name
		if tag != "" && val != "" {
			head += " " + tag + "=\"" + val + "\""
		}
		head += ">\n"
		// extra indentation of components
		spaces = "    "
		fst = true
	}

	printTail := func(name string) {

		if !fst {
			// print closing section tag
			tail := "  </" + name + ">\n"
			buffer.WriteString(tail)
		}
		head = ""
		// restore indentation
		spaces = "  "
	}

	// use of attributes is not unreasonable for showing information about ancestor IDs, e.g.,
	// <AnID name="Primates" rank="order">9443</AnID>

	printWithAttribs := func(name, tag1, val1, tag2, val2, value string) {

		if value != "" {

			if strings.ContainsAny(value, "<>&") {
				// reencodes <, >, and & to avoid breaking XML - 2137958, 2590565
				value = rfix.Replace(value)
			}

			if val1 != "" && strings.ContainsAny(val1, "<>&'\"") {
				// reencodes <, >, and &, and converts ' and " to space, to avoid breaking XML attributes
				val1 = tfix.Replace(val1)
				// then remove excess spaces
				val1 = compressRunsOfSpaces(val1)
				val1 = strings.TrimSpace(val1)
			}
			if val2 != "" && strings.ContainsAny(val2, "<>&'\"") {
				val1 = tfix.Replace(val2)
				val2 = compressRunsOfSpaces(val2)
				val2 = strings.TrimSpace(val2)
			}

			if fst {
				// print opening section tag
				buffer.WriteString(head)
				fst = false
			}

			// indent appropriate number of spaces
			buffer.WriteString(spaces)

			buffer.WriteString("<")
			buffer.WriteString(name)
			// print up to two optional attributes
			if tag1 != "" && val1 != "" {
				buffer.WriteString(" ")
				buffer.WriteString(tag1)
				buffer.WriteString("=\"")
				buffer.WriteString(val1)
				buffer.WriteString("\"")
			}
			if tag2 != "" && val2 != "" {
				buffer.WriteString(" ")
				buffer.WriteString(tag2)
				buffer.WriteString("=\"")
				buffer.WriteString(val2)
				buffer.WriteString("\"")
			}
			buffer.WriteString(">")

			buffer.WriteString(value)

			buffer.WriteString("</")
			buffer.WriteString(name)
			buffer.WriteString(">\n")
		}
	}

	printOne := func(name, value string) {

		printWithAttribs(name, "", "", "", "", value)
	}

	printMany := func(name string, values []string) {

		values = uniqueStringArray(values)
		for _, str := range values {
			printOne(name, str)
		}
	}

	// supports searching the taxonomy hierarchy index term list using wildcard truncation on a
	// lineage composed of the principal taxonomic ranks, e.g.,
	// phrase-search -db taxonomy -query "Eukaryota Metazoa Chordata Mammalia Primates * [TREE]"

	printTree := func(values []string) {

		var arry []string

		for _, str := range values {
			if str != "" {
				arry = append(arry, str)
			}
		}
		tree := strings.Join(arry, "_")
		if tree != "" {
			printOne("Tree", tree)
		}
	}

	// taxoninfo.xml output can be piped to xtract for analysis or record selection, e.g.,
	// xtract -pattern TaxonInfo -select Genus -equals Homo -and Species -equals sapiens
	// xtract -pattern TaxonInfo -select TaxID -equals 9606 -or TaxID -equals 63221

	for _, key := range keys {

		if printSetHead {
			// print opening set tag before first record
			buffer.WriteString("<TaxonInfoSet>\n")
			printSetHead = false
			printSetTail = true
		}

		// ensure indentation controls are reset
		head = ""
		spaces = "  "
		fst = true

		buffer.WriteString("<TaxonInfo>\n")

		tn := taxonInfoMap[key]

		gcs := tn.Codes
		nam := &tn.Names
		rnk := tn.Levels
		mds := tn.Mods

		printOne("TaxID", tn.TaxID)
		printOne("Rank", tn.Rank)
		printOne("Scientific", tn.Scientific)

		if rnk.Genus != "" && rnk.Species != "" {
			if tn.Division == "VRL" && strings.Index(rnk.Species, " ") > 0 {
				printOne("Binomial", rnk.Species)
			} else {
				printOne("Binomial", rnk.Genus+" "+rnk.Species)
			}
		}

		fullDiv, ok := divCodes[tn.Division]
		if ok {
			printWithAttribs("Division", "code", tn.Division, "", "", fullDiv)
		} else {
			printOne("Division", tn.Division)
		}

		printOne("Lineage", tn.Lineage)

		if rnk.Domain != "" {
			printTree([]string{
				rnk.Domain, rnk.Kingdom, rnk.Phylum, rnk.Class,
				rnk.Order, rnk.Family, rnk.Genus, rnk.Species,
			})
		}

		printOne("ParentID", tn.ParentID)

		printHead("Codes", "", "")
		printOne("Nuclear", gcs.Nuclear)
		printOne("Mitochondrial", gcs.Mitochondrial)
		printOne("Plastid", gcs.Plastid)
		printOne("Hydrogenosome", gcs.Hydrogenosome)
		printTail("Codes")

		printHead("Names", "", "")
		printMany("Common", nam.Common)
		printMany("GenBank", nam.GenBank)
		printMany("Synonym", nam.Synonym)
		printMany("Equivalent", nam.Equivalent)
		printMany("Includes", nam.Includes)
		printMany("Authority", nam.Authority)
		printMany("Acronym", nam.Acronym)
		printMany("Other", nam.Other)
		printTail("Names")

		printHead("Levels", "", "")
		printOne("Species", rnk.Species)
		printOne("Genus", rnk.Genus)
		printOne("Family", rnk.Family)
		printOne("Order", rnk.Order)
		printOne("Class", rnk.Class)
		printOne("Phylum", rnk.Phylum)
		printOne("Kingdom", rnk.Kingdom)
		printOne("Domain", rnk.Domain)
		printTail("Levels")

		printHead("Modifiers", "", "")
		printOne("Subspecies", mds.Subspecies)
		printOne("Serovar", mds.Serovar)
		printOne("Strain", mds.Strain)
		printOne("Substrain", mds.Substrain)
		printOne("Clade", mds.Clade)
		printOne("Note", mds.Note)
		printTail("Modifiers")

		if len(tn.Specimens) > 0 {
			// length of local slice is cached, no overhead to call again
			printHead("Specimens", "count", strconv.Itoa(len(tn.Specimens)))
			prev := ""
			for _, mat := range tn.Specimens {
				lc := strings.ToLower(mat.Name)
				if lc == prev {
					continue
				}

				printWithAttribs("SpID", "type", mat.Type, "", "", mat.Name)

				prev = lc
			}
			printTail("Specimens")
		}

		if tn.TaxID != "1" {
			printHead("Ancestors", "", "")
			pn := tn
			for {
				pID := pn.ParentID
				pn = taxonInfoMap[pID]
				if verboseAncestors {
					printWithAttribs("AnID", "name", pn.Scientific, "rank", pn.Rank, pID)
				} else {
					printOne("AnID", pID)
				}
				if pID == "1" {
					break
				}
			}
			printTail("Ancestors")
		}

		if len(tn.Children) > 0 {
			printHead("Children", "count", strconv.Itoa(len(tn.Children)))
			if verboseChildren {
				children := uniqueStringArray(tn.Children)
				for _, str := range children {
					cn := taxonInfoMap[str]
					printWithAttribs("ChID", "name", cn.Scientific, "rank", cn.Rank, cn.TaxID)
				}
			} else {
				printMany("ChID", tn.Children)
			}
			printTail("Children")
		}

		buffer.WriteString("</TaxonInfo>\n")

		recordCount++
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

		okay = true
	}

	if okay {
		if printSetTail {
			// print closing set tag after last record
			buffer.WriteString("</TaxonInfoSet>\n")
		}
		txt := buffer.String()
		if txt != "" {
			// print final buffer
			wrtr.WriteString(txt[:])
		}
	}
	buffer.Reset()

	wrtr.Flush()

	return recordCount
}

func main() {

	// skip past executable name
	args := os.Args[1:]

	if len(args) < 1 {
		displayError("Insufficient arguments for -taxon")
		os.Exit(1)
	}

	// path to Unicode converters cannot be derived from program path, since
	// using "go run" compiles a temporary executable in an unrelated area
	dataPath := args[0]

	// bulk of Unicode to ASCII mapping data is in a large external file
	loadRuneTable(asciiRunes, dataPath, "unicode-ascii.txt")

	createTAXONINFO(true, true)
}
