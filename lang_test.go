package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// These check the locale files against each other, and nothing else. Mistakes
// inside a single entry — a mistyped key, a format string that lost its %s —
// show up on screen as soon as that screen is opened, which turned out to be
// a better deal than the machinery it took to catch them ahead of time.

func loadLocale(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("assets", "locale", name+".json"))
	if err != nil {
		t.Fatalf("read locale %s: %v", name, err)
	}
	var table map[string]string
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatalf("parse locale %s: %v", name, err)
	}
	return table
}

// Every language must define exactly the same keys. A key present in one file
// and missing from another is a gap that only shows up after switching
// language, which is exactly when nobody is looking for it.
func TestLocalesHaveTheSameKeys(t *testing.T) {
	ru := loadLocale(t, "ru")
	en := loadLocale(t, "en")

	for k := range ru {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q is in ru.json but not in en.json", k)
		}
	}
	for k := range en {
		if _, ok := ru[k]; !ok {
			t.Errorf("key %q is in en.json but not in ru.json", k)
		}
	}
}

// No entry may be blank: an empty string renders as nothing at all, which
// looks like a layout bug rather than a missing translation.
func TestLocalesHaveNoEmptyValues(t *testing.T) {
	for _, name := range []string{"ru", "en"} {
		for k, v := range loadLocale(t, name) {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s.json: key %q is empty", name, k)
			}
		}
	}
}

// The same key has to take the same substitutions in every language. A format
// string that lost a verb in translation does not fail to compile and does not
// fail to render — it prints "%!(EXTRA string=11.0)" into the middle of the
// interface, and only in the language nobody was testing in.
//
// This is a locale-against-locale check, which is the only kind kept here: the
// source scanner that used to look for missing keys could not tell code from
// comments and cost more than it caught.
func TestLocalesTakeTheSameSubstitutions(t *testing.T) {
	verbs := regexp.MustCompile(`%[-+ #0-9.*]*[a-zA-Z]`)
	ru := loadLocale(t, "ru")
	en := loadLocale(t, "en")

	for k, e := range en {
		r, ok := ru[k]
		if !ok {
			continue // the missing-key test says so already
		}
		a, b := verbs.FindAllString(e, -1), verbs.FindAllString(r, -1)
		if !slices.Equal(a, b) {
			t.Errorf("key %q takes %v in en.json and %v in ru.json", k, a, b)
		}
	}
}

// The lookup has to survive a key that does not exist, and say so visibly.
func TestMissingKeyIsVisible(t *testing.T) {
	loadLocales()
	got := T("no.such.key")
	if !strings.Contains(got, "no.such.key") {
		t.Errorf("T on a missing key returned %q, which hides the mistake", got)
	}
}

// Every language the -lang flag accepts must actually have a file to load,
// and the default has to be one of them.
func TestLanguagesAreLoadable(t *testing.T) {
	loadLocales()
	for code, l := range localeCode {
		if _, ok := localeFile[l]; !ok {
			t.Errorf("-lang %s selects a language with no locale file", code)
		}
		if len(locales[l]) == 0 {
			t.Errorf("-lang %s selects a language that loaded no text", code)
		}
	}
	if len(locales[defaultLang]) == 0 {
		t.Error("the default language loaded no text")
	}
}
