package eutils

import "testing"

func stringTestXtract(t *testing.T, name string, proc func(string, []string) string, commands []string, input, expected string) {

	actual := proc(input, commands)
	if actual != expected {
		t.Errorf("%s(%s) = %s, expected %s", name, input, actual, expected)
	}
}

func TestXMLtoData(t *testing.T) {

	stringTestXtract(t, "XMLtoData,",
		XMLtoData,
		[]string{"-pattern", "Rec", "-def", "-", "-upper", "Data", "-element", "ID", "-title", "State"},
		"<Rec><ID>93</ID><Data>blue</Data><State>arizona</State></Rec>",
		"BLUE\t93\tArizona\n",
	)
}
