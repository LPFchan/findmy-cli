package findmy

import "testing"

func TestLookupStringsNormalizesLocaleTags(t *testing.T) {
	tests := []struct {
		lang      string
		peopleTab string
	}{
		{"fr-FR", "Personnes"},
		{"pt-PT", "Pessoas"},
		{"es-419", "Personas"},
		{"zh-Hant-TW", "聯絡人"},
		{"zh-Hans-CN", "联系人"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := lookupStrings(tt.lang)
			if got.PeopleTab != tt.peopleTab {
				t.Fatalf("PeopleTab = %q, want %q", got.PeopleTab, tt.peopleTab)
			}
		})
	}
}

func TestSkipWordsIncludesLocalizedAndEnglishTabs(t *testing.T) {
	s := lookupStrings("fr-FR")
	skip := s.SkipWords()

	for _, word := range []string{"Personnes", "Appareils", "Objets", "Rechercher", "People", "Devices", "Items", "Search"} {
		if !skip[word] {
			t.Fatalf("SkipWords missing %q", word)
		}
	}
}

func TestLookupStringsHasItemsTab(t *testing.T) {
	tests := []string{"en", "fr-FR", "de-DE", "ja-JP", "pt-BR"}

	for _, lang := range tests {
		t.Run(lang, func(t *testing.T) {
			got := lookupStrings(lang)
			if got.ItemsTab == "" {
				t.Fatalf("ItemsTab is empty")
			}
		})
	}
}

func TestParseAppleLanguages(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wanted string
	}{
		{
			name:   "bare Korean first entry",
			input:  "(\n    ko,\n    \"zh-Hans\",\n    \"zh-Hant\"\n)\n",
			wanted: "ko",
		},
		{
			name:   "quoted region tag",
			input:  "(\n    \"fr-FR\",\n    en\n)\n",
			wanted: "fr-FR",
		},
		{
			name:   "whitespace and leading comma",
			input:  " (  , \n\t en-GB  , fr ) ",
			wanted: "en-GB",
		},
		{name: "empty", input: "", wanted: "en"},
		{name: "empty array", input: "(  )", wanted: "en"},
		{name: "missing closing paren", input: "(ko, en", wanted: "en"},
		{name: "unterminated quote", input: "(\"fr-FR)", wanted: "en"},
		{name: "invalid token", input: "(ko@KR, en)", wanted: "en"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseAppleLanguages(test.input); got != test.wanted {
				t.Fatalf("parseAppleLanguages() = %q, want %q", got, test.wanted)
			}
		})
	}
}
