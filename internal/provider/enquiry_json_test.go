package provider

import "testing"

func TestStripComputedJSON(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"nested machine translations": {
			input: `[{"id":1,"content":[{"type":"title","id":9007199254740993,"text":{"en":"Hi"},"automaticTranslatedText":{"sv":"Hej"}},
				{"type":"select","id":2,"allowsCustomInput":false,"options":[{"id":1,"title":{"en":"A"},"automaticTranslatedTitle":{"sv":"A"}}]}]}]`,
			want: `[{"content":[{"id":9007199254740993,"text":{"en":"Hi"},"type":"title"},{"allowsCustomInput":false,"id":2,"options":[{"id":1,"title":{"en":"A"}}],"type":"select"}],"id":1}]`,
		},
		"object":  {input: `{"cornerStyle":"rounded","font":{"type":"predefined","value":"default"}}`, want: `{"cornerStyle":"rounded","font":{"type":"predefined","value":"default"}}`},
		"missing": {input: ``, want: `null`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := stripComputedJSON([]byte(test.input))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("got  %s\nwant %s", got, test.want)
			}
		})
	}
}

func TestStripComputedJSONRejectsInvalidJSON(t *testing.T) {
	if _, err := stripComputedJSON([]byte(`{`)); err == nil {
		t.Error("expected an error")
	}
}

func TestDefaultsAreCanonicalJSON(t *testing.T) {
	for name, value := range map[string]string{
		"defaultTheme":          defaultTheme,
		"defaultSubmittedPages": defaultSubmittedPages,
		"builtInSubmittedPages": builtInSubmittedPages,
	} {
		canonical, err := stripComputedJSON([]byte(value))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if canonical != value {
			t.Errorf("%s must be written in the canonical form read back from the API, or imports show a diff:\n%s", name, canonical)
		}
	}
}
