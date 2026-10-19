package locales

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// catalogs are the app's languages: English is the source, whose catalog also
// describes each message, and Simplified Chinese its translation.
var catalogs = []string{"en.json", "zh-Hans.json"}

// messages reads a catalog and returns its message keys, leaving out @@locale
// and the @key entries: the source language's describes the messages.
func messages(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var catalog map[string]json.RawMessage
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var keys []string
	for key := range catalog {
		if len(key) > 0 && key[0] != '@' {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// TestCatalogsSayTheSameKeys pins the translations to the source language: a
// key added to en.json without a translation says English to a Chinese reader
// rather than nothing, and a key only zh-Hans has is a message the app never
// asks for. The generated code has both problems already (mygo i18n check says
// so); this test keeps go test reporting them.
func TestCatalogsSayTheSameKeys(t *testing.T) {
	source := messages(t, "en.json")
	for _, file := range catalogs[1:] {
		translated := messages(t, file)
		if !slices.Equal(source, translated) {
			t.Errorf("%s says %v, %s says %v", file, translated, catalogs[0], source)
		}
	}
}

// TestSourceCatalogDescribesEveryMessage keeps the descriptions translators
// read: en.json carries an @key entry per message, with the placeholders' types.
func TestSourceCatalogDescribesEveryMessage(t *testing.T) {
	data, err := os.ReadFile("en.json")
	if err != nil {
		t.Fatalf("read en.json: %v", err)
	}
	var catalog map[string]json.RawMessage
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse en.json: %v", err)
	}
	for _, key := range messages(t, "en.json") {
		if _, ok := catalog["@"+key]; !ok {
			t.Errorf("en.json has no @%s description", key)
		}
	}
}
