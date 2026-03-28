package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"

	"github.com/rs/zerolog/log"
)

// Translator provides thread-safe access to translation strings loaded from
// JSON locale files. Keys are stored in flattened dot-notation
// (e.g. "common.save") and support Go template variable interpolation.
type Translator struct {
	translations map[string]map[string]interface{} // lang → flat key → value
	fallback     string
	mu           sync.RWMutex
}

// NewTranslator loads all *.json files from localesDir. Each file's basename
// (without extension) is treated as the language code (e.g. "en.json" → "en").
// fallbackLang is the language used when a key is missing in the requested language.
func NewTranslator(localesDir, fallbackLang string) (*Translator, error) {
	t := &Translator{
		translations: make(map[string]map[string]interface{}),
		fallback:     fallbackLang,
	}

	files, err := filepath.Glob(filepath.Join(localesDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("scan locale directory: %w", err)
	}
	if len(files) == 0 {
		log.Warn().Str("dir", localesDir).Msg("no locale files found")
	}

	for _, fpath := range files {
		lang := strings.TrimSuffix(filepath.Base(fpath), ".json")

		data, err := os.ReadFile(fpath)
		if err != nil {
			return nil, fmt.Errorf("read locale file %s: %w", fpath, err)
		}

		var nested map[string]interface{}
		if err := json.Unmarshal(data, &nested); err != nil {
			return nil, fmt.Errorf("parse locale file %s: %w", fpath, err)
		}

		flat := make(map[string]interface{})
		flattenMap("", nested, flat)
		t.translations[lang] = flat

		log.Info().Str("lang", lang).Int("keys", len(flat)).Msg("locale loaded")
	}

	if _, ok := t.translations[fallbackLang]; !ok && len(t.translations) > 0 {
		log.Warn().Str("fallback", fallbackLang).Msg("fallback language not found in loaded locales")
	}

	return t, nil
}

// T returns the translated string for the given key and language.
// If the key is not found in the requested language it falls back to the
// fallback language. If still not found it logs a warning and returns the key
// itself.
//
// Optional template variables can be passed to interpolate dynamic values:
//
//	t.T("en", "dashboard.hours_remaining", map[string]interface{}{"hours": 12})
func (t *Translator) T(lang, key string, vars ...map[string]interface{}) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	val := t.lookup(lang, key)
	if val == "" {
		val = t.lookup(t.fallback, key)
	}
	if val == "" {
		log.Warn().Str("lang", lang).Str("key", key).Msg("translation key not found")
		return key
	}

	// Render template variables if provided.
	if len(vars) > 0 && vars[0] != nil {
		rendered, err := renderTemplate(val, vars[0])
		if err != nil {
			log.Warn().Err(err).Str("key", key).Msg("template rendering failed")
			return val
		}
		return rendered
	}

	return val
}

// TPlural returns the singular or plural form based on count.
// keySingular and keyPlural are separate translation keys.
func (t *Translator) TPlural(lang, keySingular, keyPlural string, count int) string {
	vars := map[string]interface{}{"count": count}
	if count == 1 {
		return t.T(lang, keySingular, vars)
	}
	return t.T(lang, keyPlural, vars)
}

// AvailableLanguages returns a sorted list of loaded language codes.
func (t *Translator) AvailableLanguages() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	langs := make([]string, 0, len(t.translations))
	for lang := range t.translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// LoadLanguage loads (or reloads) a single language from raw JSON bytes.
// This is useful for hot-reloading or loading translations from a database.
func (t *Translator) LoadLanguage(lang string, data []byte) error {
	var nested map[string]interface{}
	if err := json.Unmarshal(data, &nested); err != nil {
		return fmt.Errorf("parse language data for %s: %w", lang, err)
	}

	flat := make(map[string]interface{})
	flattenMap("", nested, flat)

	t.mu.Lock()
	t.translations[lang] = flat
	t.mu.Unlock()

	log.Info().Str("lang", lang).Int("keys", len(flat)).Msg("locale loaded/reloaded")
	return nil
}

// HasKey reports whether a translation exists for the given key in the given
// language (without falling back).
func (t *Translator) HasKey(lang, key string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lookup(lang, key) != ""
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// lookup retrieves a single key from a specific language. Caller must hold at
// least a read lock.
func (t *Translator) lookup(lang, key string) string {
	langMap, ok := t.translations[lang]
	if !ok {
		return ""
	}
	val, ok := langMap[key]
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%v", val)
	}
	return s
}

// flattenMap recursively flattens a nested map into dot-notation keys.
//
//	{"common": {"save": "Save", "actions": {"delete": "Delete"}}}
//
// becomes:
//
//	{"common.save": "Save", "common.actions.delete": "Delete"}
func flattenMap(prefix string, src map[string]interface{}, dst map[string]interface{}) {
	for k, v := range src {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}

		switch child := v.(type) {
		case map[string]interface{}:
			flattenMap(fullKey, child, dst)
		default:
			dst[fullKey] = v
		}
	}
}

// renderTemplate renders a Go text/template string with the given variables.
func renderTemplate(tmplStr string, vars map[string]interface{}) (string, error) {
	// Use {{ }} delimiters.  Allow missing keys to render as "<no value>".
	tmpl, err := template.New("t").Option("missingkey=zero").Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}
