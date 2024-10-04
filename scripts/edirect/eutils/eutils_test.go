package eutils

import "testing"

type stringTable struct {
	input    string
	expected string
}

func stringTestMatch(t *testing.T, name string, proc func(str string) string, data []stringTable) {

	for _, test := range data {
		actual := proc(test.input)
		if actual != test.expected {
			t.Errorf("%s(%s) = %s, expected %s", name, test.input, actual, test.expected)
		}
	}
}

func TestCleanupAuthor(t *testing.T) {

	stringTestMatch(t, "CleanupAuthor,",
		func(str string) string { return CleanupAuthor(str, false) },
		[]stringTable{
			{"Savel&#39;ev", "Savel'ev"},
			{"O&#39;Mullane", "O'Mullane"},
			{"Rählmann Köhler Sebastião", "Rahlmann Kohler Sebastiao"},
			{"Jolín Høglend Çağıl Ertuğrul", "Jolin Hoglend Cagil Ertugrul"},
		})
}

func TestCleanupBadSpaces(t *testing.T) {

	stringTestMatch(t, "CleanupBadSpaces,",
		CleanupBadSpaces,
		[]stringTable{
			{"nonbreaking space", "nonbreaking space"},
		})
}

func TestCleanupSimple(t *testing.T) {

	stringTestMatch(t, "CleanupSimple,",
		func(str string) string { return CleanupSimple(str, false) },
		[]stringTable{
			{"lycopene β-cyclase", "lycopene beta-cyclase"},
			{"kinase cβ", "kinase cbeta"},
			{"NF-κB", "NF-kappaB"},
			{"Fbw7β", "Fbw7beta"},
			{"Behçet disease", "Behcet disease"},
			{"Montréal", "Montreal"},
			{"à la santé", "a la sante"},
			{"Möbius", "Mobius"},
			{"(32-35 °C)", "(32-35 degrees C)"},
			{"FRAX ™ tool", "FRAX (tm) tool"},
			{"ColoSure™ test", "ColoSure (tm) test"},
			{"Ciklavit®.", "Ciklavit (reg) ."},
			{"Privigen(®)", "Privigen((reg))"},
			{"(Tevagrastim®)", "(Tevagrastim (reg))"},
			{"D&lt;sub&gt;2&lt;/sub&gt;-like", "D2-like"},
			{"&lt;i&gt;DAPHNE GENKWA&lt;/i&gt;", "DAPHNE GENKWA"},
		})
}

func TestCompressRunsOfSpaces(t *testing.T) {

	stringTestMatch(t, "CompressRunsOfSpaces,",
		CompressRunsOfSpaces,
		[]stringTable{
			{"double  spaces", "double spaces"},
		})
}

func TestConvertSlash(t *testing.T) {

	stringTestMatch(t, "ConvertSlash,",
		ConvertSlash,
		[]stringTable{
			{"first\\tsecond\\n", "first\tsecond\n"},
		})
}

func TestDoTrimFlankingHTML(t *testing.T) {

	stringTestMatch(t, "DoTrimFlankingHTML,",
		DoTrimFlankingHTML,
		[]stringTable{
			{"</i>Escherichia coli<i>", "Escherichia coli"},
		})
}

func TestFixSpecialCases(t *testing.T) {

	stringTestMatch(t, "FixSpecialCases,",
		FixSpecialCases,
		[]stringTable{
			{"5'ATGTGA", "5_prime ATGTGA"},
		})
}

func TestGenBankToMedlineAuthors(t *testing.T) {

	stringTestMatch(t, "GenBankToMedlineAuthors,",
		GenBankToMedlineAuthors,
		[]stringTable{
			{"Smith-Jones,J.-P.", "Smith-Jones JP"},
		})
}

func TestNcbi2naToIupac(t *testing.T) {

	stringTestMatch(t, "Ncbi2naToIupac,",
		Ncbi2naToIupac,
		[]stringTable{
			{"6BE7EAFF30", "CGGTTGCTTGGGTTTTATAA"},
		})
}

func TestNcbi4naToIupac(t *testing.T) {

	stringTestMatch(t, "Ncbi4naToIupac,",
		Ncbi4naToIupac,
		[]stringTable{
			{"4418845814", "GGATTGRTAG"},
		})
}

func TestNormalizeAuthor(t *testing.T) {

	stringTestMatch(t, "NormalizeAuthor,",
		NormalizeAuthor,
		[]stringTable{
			{"Smith, PA", "Smith P"},
		})
}

func TestNormalizeJournal(t *testing.T) {

	stringTestMatch(t, "NormalizeJournal,",
		NormalizeJournal,
		[]stringTable{
			{"Proc. natl. acad sci", "Proc Natl Acad Sci"},
			{"PNAS", "PNAS"},
			{"PLoS Biol", "PLoS Biol"},
			{"journal of immunology", "Journal of Immunology"},
		})
}

func TestNormalizePage(t *testing.T) {

	stringTestMatch(t, "NormalizePage,",
		NormalizePage,
		[]stringTable{
			{"1904-14", "1904"},
		})
}

func TestNormalizeTitle(t *testing.T) {

	stringTestMatch(t, "NormalizeTitle,",
		NormalizeTitle,
		[]stringTable{
			{"17β-estradiol", "17 beta -estradiol"},
			{"Charité Humboldt", "Charite Humboldt"},
		})
}

func TestRelaxString(t *testing.T) {

	stringTestMatch(t, "RelaxString,",
		RelaxString,
		[]stringTable{
			{"amber (UAG) codon", "amber UAG codon"},
		})
}

func TestRemoveEmbeddedMarkup(t *testing.T) {

	stringTestMatch(t, "RemoveEmbeddedMarkup,",
		RemoveEmbeddedMarkup,
		[]stringTable{
			{"using <i>Escherichia coli</i> bacteria", "using Escherichia coli bacteria"},
		})
}

func TestRemoveCommaOrSemicolon(t *testing.T) {

	stringTestMatch(t, "RemoveCommaOrSemicolon,",
		RemoveCommaOrSemicolon,
		[]stringTable{
			{"Hello, world", "hello world"},
		})
}

func TestRemoveExtraSpaces(t *testing.T) {

	stringTestMatch(t, "RemoveExtraSpaces,",
		RemoveExtraSpaces,
		[]stringTable{
			{"( south , east, north- west )", "(south, east, north-west)"},
		})
}

func TestRemoveHTMLDecorations(t *testing.T) {

	stringTestMatch(t, "RemoveHTMLDecorations,",
		RemoveHTMLDecorations,
		[]stringTable{
			{"&lt;b&gt;", ""},
			{"&#181;", "µ"},
			{"&#x3C;", "<"},
			{"R&#xe9;animation", "Réanimation"},
		})
}

func TestRepairEncodedMarkup(t *testing.T) {

	stringTestMatch(t, "RepairEncodedMarkup,",
		RepairEncodedMarkup,
		[]stringTable{
			{"&lt;sup&gt;", "<sup>"},
			{"&amp;#181;", "&#181;"},
			{"&amp;amp;amp;amp;amp;amp;amp;lt;", "&lt;"},
			{"CO</sub><sub>2", "CO2"},
		})
}

func TestSortStringByWords(t *testing.T) {

	stringTestMatch(t, "SortStringByWords",
		SortStringByWords,
		[]stringTable{
			{"now is the time for all good men", "all for good is men now the time"},
			{"one two three", "one three two"},
		})
}

func TestTightenParentheses(t *testing.T) {

	stringTestMatch(t, "TightenParentheses,",
		TightenParentheses,
		[]stringTable{
			{" ( before after ) ", " (before after) "},
		})
}

func TestTransformAccents(t *testing.T) {

	stringTestMatch(t, "TransformAccents,",
		func(str string) string { return TransformAccents(str, true, false) },
		[]stringTable{
			{"ván", "van"},
			{"nüt", "nut"},
			{"těc", "tec"},
			{"β-c", "beta-c"},
			{"8 > 5", "8 > 5"},
			{"8 &gt; 5", "8 &gt; 5"},
		})

	stringTestMatch(t, "TransformAccents,",
		func(str string) string { return TransformAccents(str, false, false) },
		[]stringTable{
			{"β-c", " beta -c"},
		})

	stringTestMatch(t, "TransformAccents,",
		func(str string) string { return TransformAccents(str, true, true) },
		[]stringTable{
			{"8 > 5", "8 &gt; 5"},
			{"8 &gt; 5", "8 &amp;gt; 5"},
		})
}

func TestUnicodeToASCII(t *testing.T) {

	stringTestMatch(t, "UnicodeToASCII,",
		UnicodeToASCII,
		[]stringTable{
			{"µ", "&#xB5;"},
		})
}

/*
func TestCleanCombiningAccents(t *testing.T) {

	type table struct {
		input    string
		expected string
	}

	combining := []table{
		{"R&#xe9;animation", "Reanimation"},
	}

	for _, test := range combining {
		actual := CleanCombiningAccent(test.input)
		if actual != test.expected {
			t.Errorf("TransformAccents(%s) = %s, expected %s", test.input, actual, test.expected)
		}
	}
}
*/
