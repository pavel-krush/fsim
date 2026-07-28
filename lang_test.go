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

// Moving the interface text into JSON gave up the compiler's help: a mistyped
// key is no longer a build error. These tests take that job over. Between them
// they catch a key used but not defined, a key defined in one language but not
// another, and a key left behind after the call site went away.

// keyRefs are the ways the source refers to a locale key: a direct lookup, and
// a key stored in a widget option to be looked up later. Anything else that
// starts holding keys has to be added here, or its keys will look unused and a
// typo in one will go unnoticed.
var keyRefs = []*regexp.Regexp{
	regexp.MustCompile(`\bT\("([^"]+)"\)`),
	regexp.MustCompile(`\bInfo:\s*"([^"]+)"`),
}

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

// keysUsedInSource scans every Go file for T("...") and returns the keys.
func keysUsedInSource(t *testing.T) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range keyRefs {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				used[m[1]] = append(used[m[1]], name)
			}
		}
	}
	if len(used) == 0 {
		t.Fatal("found no key references at all — the scanner is broken, not the locales")
	}
	return used
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

// Every key the code asks for must exist. This is the check that replaces the
// compile error a mistyped identifier used to produce.
func TestEveryUsedKeyExists(t *testing.T) {
	ru := loadLocale(t, "ru")
	for key, files := range keysUsedInSource(t) {
		if _, ok := ru[key]; !ok {
			t.Errorf("key %q is used in %s but defined nowhere", key, strings.Join(files, ", "))
		}
	}
}

// Keys nobody asks for are dead weight and, worse, make the locale files look
// more complete than they are.
func TestNoUnusedKeys(t *testing.T) {
	used := keysUsedInSource(t)
	for key := range loadLocale(t, "ru") {
		if _, ok := used[key]; !ok {
			t.Errorf("key %q is defined but never used", key)
		}
	}
}

// Format strings must take the same verbs in every language. Translating
// "%s at %s km" into a variant with one %s would panic at print time, and only
// on the screen that happens to use it.
func TestFormatVerbsMatchAcrossLocales(t *testing.T) {
	verb := regexp.MustCompile(`%[a-zA-Z]`)
	ru := loadLocale(t, "ru")
	en := loadLocale(t, "en")

	for k, rv := range ru {
		ev, ok := en[k]
		if !ok {
			continue // reported by TestLocalesHaveTheSameKeys
		}
		rverbs := verb.FindAllString(rv, -1)
		everbs := verb.FindAllString(ev, -1)
		if !slices.Equal(rverbs, everbs) {
			t.Errorf("key %q: ru has verbs %v, en has %v", k, rverbs, everbs)
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
