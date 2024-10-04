package eutils

import "testing"

func stringTestTransmute(t *testing.T, name string, proc func(string) string, input, expected string) {

	actual := proc(input)
	if actual != expected {
		t.Errorf("%s(%s) = %s, expected %s", name, input, actual, expected)
	}
}

func TestJSONtoXML(t *testing.T) {

	stringTestTransmute(t, "JSONtoXML,",
		func(str string) string { return JSONtoXML(str, "", "Rec", "") },
		`{"download":"*","collection":"pathway","where":{"ands":[{"geneid":"1956"}]},"order":["taxname,asc"],
		  "start":1,"limit":10000000,"downloadfilename":"GeneID_1956_pathway"}`,
		`<Rec>
  <download>*</download>
  <collection>pathway</collection>
  <where>
    <ands>
      <geneid>1956</geneid>
    </ands>
  </where>
  <order>taxname,asc</order>
  <start>1</start>
  <limit>10000000</limit>
  <downloadfilename>GeneID_1956_pathway</downloadfilename>
</Rec>
`,
	)
}

func TestASNtoXML(t *testing.T) {

	stringTestTransmute(t, "ASNtoXML,",
		func(str string) string { return ASNtoXML(str, "", "") },
		`Seq-align ::= {
  segs denseg {
    numseg 3,
    ids { genbank { accession "AY046051", version 1 } },
    starts { 5613, 842, -1, 961, 5599, 962 },
    lens { 119, 1, 14 },
    strands { minus, plus, minus, plus, minus, plus }
  }
}
`,
		`<Seq-align>
  <segs>
    <denseg>
      <numseg>3</numseg>
      <ids>
        <genbank>
          <accession>AY046051</accession>
          <version>1</version>
        </genbank>
      </ids>
      <starts>
        <starts_E>5613</starts_E>
        <starts_E>842</starts_E>
        <starts_E>-1</starts_E>
        <starts_E>961</starts_E>
        <starts_E>5599</starts_E>
        <starts_E>962</starts_E>
      </starts>
      <lens>
        <lens_E>119</lens_E>
        <lens_E>1</lens_E>
        <lens_E>14</lens_E>
      </lens>
      <strands>
        <strands_E>minus</strands_E>
        <strands_E>plus</strands_E>
        <strands_E>minus</strands_E>
        <strands_E>plus</strands_E>
        <strands_E>minus</strands_E>
        <strands_E>plus</strands_E>
      </strands>
    </denseg>
  </segs>
</Seq-align>
`,
	)
}

func TestINItoXML(t *testing.T) {

	stringTestTransmute(t, "INItoXML,",
		func(str string) string { return INItoXML(str) },
		`[section1]
key1 = value1
key2 = value2

[section2]
key3 = value3

[.sub1]
key4 = value4

[section3.sub1]
key5 = value5

[section3.sub2]
key6 = value6
`,
		`<ConfigFile>
  <section1>
    <key1>value1</key1>
    <key2>value2</key2>
  </section1>
  <section2>
    <key3>value3</key3>
    <sub1>
      <key4>value4</key4>
    </sub1>
  </section2>
  <section3>
    <sub1>
      <key5>value5</key5>
    </sub1>
    <sub2>
      <key6>value6</key6>
    </sub2>
  </section3>
</ConfigFile>
`,
	)
}

func TestTOMLtoXML(t *testing.T) {

	stringTestTransmute(t, "TOMLtoXML,",
		func(str string) string { return TOMLtoXML(str) },
		`[section1]
key1 = "value1"
key2 = "value2"

[section2]
key3 = "value3"

[section2.sub1]
key4 = "value4"

[section3.sub1]
key5 = "value5"

[section3.sub2]
key6 = "value6"
`,
		`<ConfigFile>
  <section1>
    <key1>value1</key1>
    <key2>value2</key2>
  </section1>
  <section2>
    <key3>value3</key3>
    <sub1>
      <key4>value4</key4>
    </sub1>
  </section2>
  <section3>
    <sub1>
      <key5>value5</key5>
    </sub1>
    <sub2>
      <key6>value6</key6>
    </sub2>
  </section3>
</ConfigFile>
`,
	)
}

func TestYAMLtoXML(t *testing.T) {

	stringTestTransmute(t, "YAMLtoXML,",
		func(str string) string { return YAMLtoXML(str) },
		`section1:
    key1: value1
    key2: value2
section2:
    key3: value3
`,
		`<ConfigFile>
  <section1>
    <key1>value1</key1>
    <key2>value2</key2>
  </section1>
  <section2>
    <key3>value3</key3>
  </section2>
</ConfigFile>
`,
	)
}

func TestNormalizeXML(t *testing.T) {

	stringTestTransmute(t, "NormalizeXML,",
		func(str string) string {
			chn := StringToChan(str)
			rdr := CreateXMLStreamer(nil, chn)
			nrm := NormalizeXML(rdr, "pubmed")
			return ChanToString(nrm)
		},
		`<DocumentSummary uid="2539356">
  <PubDate>1989 Apr</PubDate>
  <Source>J Bacteriol</Source>
</DocumentSummary>
`,
		`<DocumentSummary>
<Id>
2539356
</Id>
<PubDate>
1989 Apr
</PubDate>
<Source>
J Bacteriol
</Source>
</DocumentSummary>
`,
	)
}

func TestFormatXML(t *testing.T) {

	stringTestTransmute(t, "FormatXML,",
		func(str string) string {
			chn := StringToChan(str)
			rdr := CreateXMLStreamer(nil, chn)
			tknq := CreateTokenizer(rdr)
			frgs := FormatArgs{Format: "indent", XML: "", Doctype: ""}
			frm := FormatTokens(tknq, frgs)
			return ChanToString(frm)
		},
		`<DocumentSummary>
<Id>
2539356
</Id>
<PubDate>
1989 Apr
</PubDate>
<Source>
J Bacteriol
</Source>
</DocumentSummary>
`,
		`<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE DocumentSummary>
<DocumentSummary>
  <Id>2539356</Id>
  <PubDate>1989 Apr</PubDate>
  <Source>J Bacteriol</Source>
</DocumentSummary>
`,
	)
}
