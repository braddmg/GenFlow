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
// File Name:  insd.go
//
// Author:  Jonathan Kans
//
// ==========================================================================

package eutils

import (
	"os"
	"strings"
)

// INSDSEQ EXTRACTION COMMAND GENERATOR

// e.g., xtract -insd complete mat_peptide "%peptide" product mol_wt peptide

// ProcessINSD generates extraction commands for GenBank/RefSeq records in INSDSet format
func ProcessINSD(args []string, isPipe, addDash, doIndex, makeXML bool) []string {

	// legal GenBank / GenPept / RefSeq features

	features := []string{
		"-10_signal",
		"-35_signal",
		"3'clip",
		"3'UTR",
		"5'clip",
		"5'UTR",
		"allele",
		"assembly_gap",
		"attenuator",
		"Bond",
		"C_region",
		"CAAT_signal",
		"CDS",
		"centromere",
		"conflict",
		"D_segment",
		"D-loop",
		"enhancer",
		"exon",
		"gap",
		"GC_signal",
		"gene",
		"iDNA",
		"intron",
		"J_segment",
		"LTR",
		"mat_peptide",
		"misc_binding",
		"misc_difference",
		"misc_feature",
		"misc_recomb",
		"misc_RNA",
		"misc_signal",
		"misc_structure",
		"mobile_element",
		"modified_base",
		"mRNA",
		"mutation",
		"N_region",
		"ncRNA",
		"old_sequence",
		"operon",
		"oriT",
		"polyA_signal",
		"polyA_site",
		"precursor_RNA",
		"prim_transcript",
		"primer_bind",
		"promoter",
		"proprotein",
		"protein_bind",
		"Protein",
		"RBS",
		"Region",
		"regulatory",
		"rep_origin",
		"repeat_region",
		"repeat_unit",
		"rRNA",
		"S_region",
		"satellite",
		"scRNA",
		"sig_peptide",
		"Site",
		"snoRNA",
		"snRNA",
		"source",
		"stem_loop",
		"STS",
		"TATA_signal",
		"telomere",
		"terminator",
		"tmRNA",
		"transit_peptide",
		"tRNA",
		"unsure",
		"V_region",
		"V_segment",
		"variation",
	}

	// legal GenBank / GenPept / RefSeq qualifiers

	qualifiers := []string{
		"allele",
		"altitude",
		"anticodon",
		"artificial_location",
		"bio_material",
		"bond_type",
		"bound_moiety",
		"breed",
		"calculated_mol_wt",
		"cell_line",
		"cell_type",
		"chloroplast",
		"chromoplast",
		"chromosome",
		"circular_RNA",
		"citation",
		"clone_lib",
		"clone",
		"coded_by",
		"codon_start",
		"codon",
		"collected_by",
		"collection_date",
		"compare",
		"cons_splice",
		"country",
		"cultivar",
		"culture_collection",
		"cyanelle",
		"db_xref",
		"derived_from",
		"dev_stage",
		"direction",
		"EC_number",
		"ecotype",
		"encodes",
		"endogenous_virus",
		"environmental_sample",
		"estimated_length",
		"evidence",
		"exception",
		"experiment",
		"focus",
		"frequency",
		"function",
		"gap_type",
		"gdb_xref",
		"gene_synonym",
		"gene",
		"geo_loc_name",
		"germline",
		"GO_component",
		"GO_function",
		"GO_process",
		"haplogroup",
		"haplotype",
		"host",
		"identified_by",
		"inference",
		"insertion_seq",
		"isolate",
		"isolation_source",
		"kinetoplast",
		"lab_host",
		"label",
		"lat_lon",
		"linkage_evidence",
		"locus_tag",
		"macronuclear",
		"map",
		"mating_type",
		"metagenome_source",
		"metagenomic",
		"mitochondrion",
		"mobile_element_type",
		"mobile_element",
		"mod_base",
		"mol_type",
		"name",
		"nat_host",
		"ncRNA_class",
		"non_functional",
		"note",
		"number",
		"old_locus_tag",
		"operon",
		"organelle",
		"organism",
		"partial",
		"PCR_conditions",
		"PCR_primers",
		"peptide",
		"phenotype",
		"plasmid",
		"pop_variant",
		"product",
		"protein_id",
		"proviral",
		"pseudo",
		"pseudogene",
		"rearranged",
		"recombination_class",
		"region_name",
		"regulatory_class",
		"replace",
		"ribosomal_slippage",
		"rpt_family",
		"rpt_type",
		"rpt_unit_range",
		"rpt_unit_seq",
		"rpt_unit",
		"satellite",
		"segment",
		"sequenced_mol",
		"serotype",
		"serovar",
		"sex",
		"site_type",
		"specific_host",
		"specimen_voucher",
		"standard_name",
		"strain",
		"structural_class",
		"sub_clone",
		"sub_species",
		"sub_strain",
		"submitter_seqid",
		"tag_peptide",
		"tissue_lib",
		"tissue_type",
		"trans_splicing",
		"transcript_id",
		"transcription",
		"transgenic",
		"transl_except",
		"transl_table",
		"translation",
		"transposon",
		"type_material",
		"UniProtKB_evidence",
		"usedin",
		"variety",
		"virion",
	}

	// legal INSDSeq XML fields

	insdtags := []string{
		"INSDAltSeqData_items",
		"INSDAltSeqData",
		"INSDAltSeqItem_first-accn",
		"INSDAltSeqItem_gap-comment",
		"INSDAltSeqItem_gap-length",
		"INSDAltSeqItem_gap-linkage",
		"INSDAltSeqItem_gap-type",
		"INSDAltSeqItem_interval",
		"INSDAltSeqItem_isgap",
		"INSDAltSeqItem_isgap@value",
		"INSDAltSeqItem_last-accn",
		"INSDAltSeqItem_value",
		"INSDAltSeqItem",
		"INSDAuthor",
		"INSDComment_paragraphs",
		"INSDComment_type",
		"INSDComment",
		"INSDCommentParagraph",
		"INSDFeature_intervals",
		"INSDFeature_key",
		"INSDFeature_location",
		"INSDFeature_operator",
		"INSDFeature_partial3",
		"INSDFeature_partial3@value",
		"INSDFeature_partial5",
		"INSDFeature_partial5@value",
		"INSDFeature_quals",
		"INSDFeature_xrefs",
		"INSDFeature",
		"INSDFeatureSet_annot-source",
		"INSDFeatureSet_features",
		"INSDFeatureSet",
		"INSDInterval_accession",
		"INSDInterval_from",
		"INSDInterval_interbp",
		"INSDInterval_interbp@value",
		"INSDInterval_iscomp",
		"INSDInterval_iscomp@value",
		"INSDInterval_point",
		"INSDInterval_to",
		"INSDInterval",
		"INSDKeyword",
		"INSDQualifier_name",
		"INSDQualifier_value",
		"INSDQualifier",
		"INSDReference_authors",
		"INSDReference_consortium",
		"INSDReference_journal",
		"INSDReference_position",
		"INSDReference_pubmed",
		"INSDReference_reference",
		"INSDReference_remark",
		"INSDReference_title",
		"INSDReference_xref",
		"INSDReference",
		"INSDSecondary-accn",
		"INSDSeq_accession-version",
		"INSDSeq_alt-seq",
		"INSDSeq_comment-set",
		"INSDSeq_comment",
		"INSDSeq_contig",
		"INSDSeq_create-date",
		"INSDSeq_create-release",
		"INSDSeq_database-reference",
		"INSDSeq_definition",
		"INSDSeq_division",
		"INSDSeq_entry-version",
		"INSDSeq_feature-set",
		"INSDSeq_feature-table",
		"INSDSeq_keywords",
		"INSDSeq_length",
		"INSDSeq_locus",
		"INSDSeq_moltype",
		"INSDSeq_organism",
		"INSDSeq_other-seqids",
		"INSDSeq_primary-accession",
		"INSDSeq_primary",
		"INSDSeq_project",
		"INSDSeq_references",
		"INSDSeq_secondary-accessions",
		"INSDSeq_segment",
		"INSDSeq_sequence",
		"INSDSeq_source-db",
		"INSDSeq_source",
		"INSDSeq_strandedness",
		"INSDSeq_struc-comments",
		"INSDSeq_taxonomy",
		"INSDSeq_topology",
		"INSDSeq_update-date",
		"INSDSeq_update-release",
		"INSDSeq_xrefs",
		"INSDSeq",
		"INSDSeqid",
		"INSDSet",
		"INSDStrucComment_items",
		"INSDStrucComment_name",
		"INSDStrucComment",
		"INSDStrucCommentItem_tag",
		"INSDStrucCommentItem_url",
		"INSDStrucCommentItem_value",
		"INSDStrucCommentItem",
		"INSDXref_dbname",
		"INSDXref_id",
		"INSDXref",
	}

	checkAgainstVocabulary := func(str, objtype string, arry []string) {

		if str == "" || arry == nil {
			return
		}

		// skip past pound, percent, or caret character at beginning of string
		if len(str) > 1 {
			switch str[0] {
			case '#', '%', '^':
				str = str[1:]
			default:
			}
		}

		for _, txt := range arry {
			if str == txt {
				return
			}
			if strings.ToUpper(str) == strings.ToUpper(txt) {
				DisplayError("Incorrect capitalization of '%s' %s, change to '%s'", str, objtype, txt)
				os.Exit(1)
			}
		}

		DisplayError("Item '%s' is not a legal -insd %s", str, objtype)
		os.Exit(1)
	}

	var acc []string

	max := len(args)
	if max < 1 {
		DisplayError("Insufficient command-line arguments supplied to xtract -insd")
		os.Exit(1)
	}

	quote := ""
	retrn := "\n"
	if !isPipe {
		quote = "\""
		retrn = "\\n"
	}

	// record accession and sequence

	if doIndex {
		acc = append(acc, "-head", quote+"<IdxDocumentSet>"+quote, "-tail", quote+"</IdxDocumentSet>"+quote)
		acc = append(acc, "-hd", quote+"  <IdxDocument>"+retrn+quote, "-tl", quote+"  </IdxDocument>"+quote)
		acc = append(acc, "-pattern", "INSDSeq", "-pfx", quote+"    <IdxUid>"+quote, "-sfx", quote+"</IdxUid>"+retrn+quote)
		acc = append(acc, "-element", "INSDSeq_accession-version", "-clr", "-rst", "-tab", retrn)
	} else {
		acc = append(acc, "-pattern", "INSDSeq", "-ACCN", "INSDSeq_accession-version")
		acc = append(acc, "-LCUS", "INSDSeq_locus", "-SEQ", "INSDSeq_sequence")
	}

	if doIndex {
		acc = append(acc, "-group", "INSDSeq", "-lbl", quote+"    <IdxSearchFields>"+retrn+quote)
	}

	printAccn := true

	// collect descriptors

	if strings.HasPrefix(args[0], "INSD") {

		if doIndex {
			acc = append(acc, "-clr", "-wrp", "TIAB", "-indexer")
		} else {
			acc = append(acc, "-clr", "-pfx", quote+"\\n"+quote, "-element", quote+"&ACCN"+quote)
			acc = append(acc, "-group", "INSDSeq", "-sep", quote+"|"+quote, "-element")
			printAccn = false
		}

		for {
			if len(args) < 1 {
				return acc
			}
			str := args[0]
			if !strings.HasPrefix(args[0], "INSD") {
				break
			}
			checkAgainstVocabulary(str, "element", insdtags)
			acc = append(acc, str)
			args = args[1:]
		}

	} else if strings.HasPrefix(strings.ToUpper(args[0]), "INSD") {

		// report capitalization or vocabulary failure
		checkAgainstVocabulary(args[0], "element", insdtags)

		// program should not get to this point, but warn and exit anyway
		DisplayError("Item '%s' is not a legal -insd %s", args[0], "element")
		os.Exit(1)
	}

	processOneFeature := func(ftargs []string) {

		// skip past -insd feature clause separator

		if ftargs[0] == "-insd" || ftargs[0] == "-insdx" {
			ftargs = ftargs[1:]
		}

		// collect qualifiers

		partial := false
		complete := false

		if ftargs[0] == "+" || ftargs[0] == "complete" {
			complete = true
			ftargs = ftargs[1:]
			max--
		} else if ftargs[0] == "-" || ftargs[0] == "partial" {
			partial = true
			ftargs = ftargs[1:]
			max--
		}

		if max < 1 {
			DisplayError("No feature key supplied to xtract -insd")
			os.Exit(1)
		}

		acc = append(acc, "-group", "INSDFeature")

		// limit to designated features

		feature := ftargs[0]

		fcmd := "-if"

		// can specify multiple features separated by plus sign (e.g., CDS+mRNA) or comma (e.g., CDS,mRNA)
		plus := strings.Split(feature, "+")
		for _, pls := range plus {
			comma := strings.Split(pls, ",")
			for _, cma := range comma {

				checkAgainstVocabulary(cma, "feature", features)
				acc = append(acc, fcmd, "INSDFeature_key", "-equals", cma)

				fcmd = "-or"
			}
		}

		if max < 2 {
			// still need at least one qualifier even on legal feature
			DisplayError("Feature '%s' must be followed by at least one qualifier", feature)
			os.Exit(1)
		}

		ftargs = ftargs[1:]

		if complete {
			acc = append(acc, "-branch", "INSDFeature", "-unless", "INSDFeature_partial5", "-or", "INSDFeature_partial3")
		} else if partial {
			acc = append(acc, "-branch", "INSDFeature", "-if", "INSDFeature_partial5", "-or", "INSDFeature_partial3")
		}

		if printAccn {
			if doIndex {
			} else {
				acc = append(acc, "-clr", "-pfx", quote+"\\n"+quote, "-first", quote+"&ACCN,&LCUS"+quote)
				printAccn = false
			}
		}

		if makeXML {
			acc = append(acc, "-block", "INSDFeature", "-element", "INSDFeature_key")
		}

		for _, str := range ftargs {

			alt := ""

			if str == "mol_wt" {
				str = "calculated_mol_wt"
			}

			// for the source feature, INSDC has replaced the country qualifier with geo_loc_name
			if feature == "source" {
				if str == "country" {
					str = "geo_loc_name"
					alt = "country"
				} else if str == "geo_loc_name" {
					alt = "country"
				}
			}

			if strings.HasPrefix(str, "INSD") {

				checkAgainstVocabulary(str, "element", insdtags)
				if doIndex {
					acc = append(acc, "-block", "INSDFeature", "-clr", "-wrp", "TIAB", "-indexer")
				} else {
					acc = append(acc, "-block", "INSDFeature", "-sep", quote+"|"+quote, "-element")
				}
				acc = append(acc, str)
				if addDash {
					acc = append(acc, "-block", "INSDFeature", "-unless", str)
					if strings.HasSuffix(str, "@value") {
						acc = append(acc, "-lbl", quote+"false"+quote)
					} else {
						acc = append(acc, "-lbl", quote+"\\-"+quote)
					}
				}

			} else if strings.HasPrefix(str, "#INSD") {

				checkAgainstVocabulary(str, "element", insdtags)
				if doIndex {
					acc = append(acc, "-block", "INSDFeature", "-clr", "-wrp", "TIAB", "-indexer")
				} else {
					acc = append(acc, "-block", "INSDFeature", "-sep", quote+"|"+quote, "-element")
					acc = append(acc, quote+str+quote)
				}

			} else if strings.HasPrefix(strings.ToUpper(str), "#INSD") {

				// report capitalization or vocabulary failure
				checkAgainstVocabulary(str, "element", insdtags)

			} else if str == "sub_sequence" {

				// special sub_sequence qualifier shows sequence under feature intervals
				acc = append(acc, "-block", "INSDFeature_intervals")

				acc = append(acc, "-subset", "INSDInterval", "-FR", "INSDInterval_from", "-TO", "INSDInterval_to")
				acc = append(acc, "-pfx", quote+""+quote, "-tab", quote+""+quote, "-nucleic", quote+"&SEQ[&FR:&TO]"+quote)

				acc = append(acc, "-subset", "INSDFeature_intervals", "-deq", quote+"\\t"+quote)

			} else if str == "feat_location" {

				// special feat_location qualifier shows feature intervals, in 1-based GenBank convention
				acc = append(acc, "-block", "INSDFeature_intervals")

				acc = append(acc, "-subset", "INSDInterval", "-FR", "INSDInterval_from", "-TO", "INSDInterval_to")
				acc = append(acc, "-pfx", quote+""+quote, "-tab", quote+".."+quote, "-element", quote+"&FR"+quote)
				acc = append(acc, "-pfx", quote+""+quote, "-tab", quote+","+quote, "-element", quote+"&TO"+quote)

				acc = append(acc, "-subset", "INSDFeature_intervals", "-deq", quote+"\\t"+quote)

			} else if str == "feat_intervals" {

				// special feat_intervals qualifier shows feature intervals, decremented to 0-based
				acc = append(acc, "-block", "INSDFeature_intervals")

				acc = append(acc, "-subset", "INSDInterval")
				acc = append(acc, "-pfx", quote+""+quote, "-tab", quote+".."+quote, "-dec", "INSDInterval_from")
				acc = append(acc, "-pfx", quote+""+quote, "-tab", quote+","+quote, "-dec", "INSDInterval_to")

				acc = append(acc, "-subset", "INSDFeature_intervals", "-deq", quote+"\\t"+quote)

			} else if str == "chloroplast" ||
				str == "chromoplast" ||
				str == "cyanelle" ||
				str == "environmental_sample" ||
				str == "focus" ||
				str == "germline" ||
				str == "kinetoplast" ||
				str == "macronuclear" ||
				str == "metagenomic" ||
				str == "mitochondrion" ||
				str == "partial" ||
				str == "proviral" ||
				str == "pseudo" ||
				str == "rearranged" ||
				str == "ribosomal_slippage" ||
				str == "trans_splicing" ||
				str == "transgenic" ||
				str == "virion" {

				acc = append(acc, "-block", "INSDQualifier")

				checkAgainstVocabulary(str, "qualifier", qualifiers)
				if doIndex {
					acc = append(acc, "-if", "INSDQualifier_name", "-equals", str)
					acc = append(acc, "-clr", "-wrp", "TIAB", "-indexer", "INSDQualifier_name")
				} else {
					acc = append(acc, "-if", "INSDQualifier_name", "-equals", str)
					acc = append(acc, "-lbl", str)
				}
				if addDash {
					acc = append(acc, "-block", "INSDFeature", "-unless", "INSDQualifier_name", "-equals", str)
					acc = append(acc, "-lbl", quote+"\\-"+quote)
				}

			} else {

				acc = append(acc, "-block", "INSDQualifier")

				isTaxID := false
				if feature == "source" && (str == "taxon" || str == "taxid") {
					// special taxid qualifier extracts number from taxon db_xref
					isTaxID = true
					str = "db_xref"
				} else {
					checkAgainstVocabulary(str, "qualifier", qualifiers)
				}

				if len(str) > 2 && str[0] == '%' {
					acc = append(acc, "-if", "INSDQualifier_name", "-equals", str[1:])
					if doIndex {
						acc = append(acc, "-clr", "-wrp", "TIAB", "-indexer", quote+"%INSDQualifier_value"+quote)
					} else {
						acc = append(acc, "-element", quote+"%INSDQualifier_value"+quote)
					}
					if addDash {
						acc = append(acc, "-block", "INSDFeature", "-unless", "INSDQualifier_name", "-equals", str[1:])
						acc = append(acc, "-lbl", quote+"\\-"+quote)
					}
				} else {
					if doIndex {
						acc = append(acc, "-if", "INSDQualifier_name", "-equals", str)
						acc = append(acc, "-clr", "-wrp", "TIAB", "-indexer", "INSDQualifier_value")
					} else if isTaxID {
						acc = append(acc, "-if", "INSDQualifier_name", "-equals", str)
						acc = append(acc, "-and", "INSDQualifier_value", "-starts-with", "taxon:")
						acc = append(acc, "-element", "INSDQualifier_value[taxon:|]")
					} else {
						acc = append(acc, "-if", "INSDQualifier_name", "-equals", str)
						if alt != "" {
							acc = append(acc, "-or", "INSDQualifier_name", "-equals", alt)
						}
						acc = append(acc, "-element", "INSDQualifier_value")
					}
					if addDash {
						if isTaxID {
							acc = append(acc, "-block", "INSDFeature", "-unless", "INSDQualifier_value", "-starts-with", "taxon:")
						} else {
							acc = append(acc, "-block", "INSDFeature", "-unless", "INSDQualifier_name", "-equals", str)
						}
						acc = append(acc, "-lbl", quote+"\\-"+quote)
					}
				}
			}
		}
	}

	// multiple feature clauses are separated by additional -insd arguments

	last := 0
	curr := 0
	nxt := ""

	for curr, nxt = range args {
		if nxt == "-insd" || nxt == "-insdx" {
			if last < curr {
				processOneFeature(args[last:curr])
				last = curr
			}
		}
	}

	if last < curr {
		processOneFeature(args[last:])
	}

	if doIndex {
		acc = append(acc, "-group", "INSDSeq", "-clr", "-lbl", quote+"    </IdxSearchFields>"+retrn+quote)
	}

	return acc
}

// BIOTHINGS EXTRACTION COMMAND GENERATOR

// ProcessBiopath generates extraction commands for BioThings resources (undocumented)
func ProcessBiopath(args []string, isPipe bool) []string {

	// nquire -get "http://myvariant.info/v1/variant/chr6:g.26093141G>A" \
	//   -fields clinvar.rcv.conditions.identifiers \
	//   -always_list clinvar.rcv.conditions.identifiers |
	// transmute -j2x |
	// xtract -biopath opt clinvar.rcv.conditions.identifiers.omim

	var acc []string

	max := len(args)
	if max < 2 {
		DisplayError("Insufficient command-line arguments supplied to xtract -biopath")
		os.Exit(1)
	}

	obj := args[0]
	args = args[1:]

	acc = append(acc, "-pattern", obj)

	paths := args[0]

	items := strings.Split(paths, ",")

	for _, path := range items {

		dirs := strings.Split(path, ".")
		max = len(dirs)
		if max < 1 {
			DisplayError("Insufficient path arguments supplied to xtract -biopath")
			os.Exit(1)
		}
		if max > 7 {
			DisplayError("Too many nodes in argument supplied to xtract -biopath")
			os.Exit(1)
		}

		str := dirs[max-1]

		acc = append(acc, "-path")
		if isPipe {
			acc = append(acc, path)
			acc = append(acc, "-tab", "\\n")
			acc = append(acc, "-element", str)
		} else {
			acc = append(acc, "\""+path+"\"")
			acc = append(acc, "-tab", "\"\\n\"")
			acc = append(acc, "-element", "\""+str+"\"")
		}
	}

	return acc
}
