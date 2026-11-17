package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// ConfigVersion identifies the sparse-override file format. Defaults always
// come from the current application; only personal choices belong in this file.
const ConfigVersion = 1

type configDocument map[string]json.RawMessage

// Snapshots contain encoded values, not aliases of mutable runtime maps/slices.
// Never change a tracking object after attaching it to a Config.
type configTracking struct {
	path     string
	baseline configDocument
}

// DefaultConfig returns independent effective defaults without writing a file.
func DefaultConfig() *Config {
	cfg := &Config{
		PageSize:         DefaultPageSize,
		MaxIssues:        DefaultMaxIssues,
		Fields:           slices.Clone(DefaultFields),
		CustomTypes:      CustomTypesMap{"default": slices.Clone(DefaultTypes)},
		CustomPriorities: CustomPrioritiesMap{"default": slices.Clone(DefaultPriorities)},
		CustomStates:     CustomStatesMap{"default": slices.Clone(DefaultStates)},
		WorkTypes:        slices.Clone(DefaultWorkTypes),
		ActivityFilters:  []string{"Comments"},
		RenderMarkdown:   true,
		BrowserCommand:   "xdg-open",
		Theme:            "catppuccin",
	}
	// Factory-created configs have a known starting point even before their
	// first load/save. Unlike ambiguous zero-value literals, changing true to
	// false or a positive limit to zero is therefore an observable user choice.
	baseline, _ := configValues(cfg)
	cfg.tracking = &configTracking{baseline: baseline}
	return cfg
}

// LoadConfig applies explicit user overrides to fresh defaults. Legacy files
// are migrated once, with an exact backup before any replacement. Missing keys
// and null inherit defaults; arrays replace them and map entries overlay them.
func LoadConfig() (*Config, error) {
	fallback := DefaultConfig()
	path, err := configurationPath()
	if err != nil {
		return fallback, err
	}
	doc, err := readDocument(path)
	if err != nil {
		return fallback, err
	}
	if doc == nil {
		doc = configDocument{"config_version": json.RawMessage("1")}
		if err := writeDocument(path, doc); err != nil {
			return fallback, err
		}
	}
	cfg, err := effectiveConfig(doc)
	if err != nil {
		return fallback, err
	}
	baseline, err := configValues(cfg)
	if err != nil {
		return fallback, err
	}
	cfg.tracking = &configTracking{path: path, baseline: baseline}
	return cfg, nil
}

// SaveConfig persists effective changes plus optional explicit top-level choices.
// Passing a field name also records a deliberate choice equal to today's default.
// Existing explicit choices remain explicit. Config setters mark nested map keys
// themselves, so an issue filter save does not pin unrelated inherited defaults.
func SaveConfig(cfg *Config, explicitFields ...string) error {
	if cfg == nil {
		return errors.New("cannot save a nil configuration")
	}
	path, err := configurationPath()
	if err != nil {
		return err
	}
	if cfg.tracking != nil && cfg.tracking.path != "" && cfg.tracking.path != path {
		return errors.New("configuration path changed; reload before saving")
	}
	current, err := configValues(cfg)
	if err != nil {
		return err
	}
	paths := slices.Clone(cfg.explicitPaths)
	for _, field := range explicitFields {
		if _, known := current[field]; !known {
			return fmt.Errorf("unknown configuration field %q", field)
		}
		paths = append(paths, []string{field})
	}
	// Merge into the current document, not the old loaded copy: unrelated edits
	// and an override removed in an editor must survive a subsequent UI save.
	doc, err := readDocument(path)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = configDocument{"config_version": json.RawMessage("1")}
	}
	var baseline configDocument
	if cfg.tracking != nil {
		baseline = cfg.tracking.baseline
	} else {
		// Config literals predate the persistence API. Treat nonzero fields as
		// supplied values; callers can explicitly name zero/false choices.
		baseline, err = configValues(DefaultConfig())
		if err != nil {
			return err
		}
		for key, value := range current {
			if isZeroJSON(value) {
				current[key] = baseline[key]
			}
		}
	}
	for key, value := range current {
		if jsonEqual(value, baseline[key]) {
			continue
		}
		override, remove := changedOverride(normalizeLegacyMap(key, doc[key]), baseline[key], value)
		if remove {
			delete(doc, key)
		} else {
			doc[key] = override
		}
	}
	// Use actual values for explicit choices, including zero values from literals.
	actual, err := configValues(cfg)
	if err != nil {
		return err
	}
	for _, path := range paths {
		value, exists := valueAt(actual, path)
		setOverride(doc, path, value, exists)
	}
	if _, err := effectiveConfig(doc); err != nil {
		return err
	}
	if err := writeDocument(path, doc); err != nil {
		return err
	}
	// Assign fresh tracking only after success. A detached draft shares no
	// mutable persistence state with the original Config, even on failed saves.
	cfg.tracking = &configTracking{path: path, baseline: actual}
	cfg.explicitPaths = nil
	return nil
}

func (cfg *Config) markOverride(path ...string) {
	for _, existing := range cfg.explicitPaths {
		if slices.Equal(existing, path) {
			return
		}
	}
	paths := slices.Clone(cfg.explicitPaths)
	cfg.explicitPaths = append(paths, slices.Clone(path))
}

// Include zero values even when the public JSON tag has omitempty: clearing a
// preference is an effective change, not an instruction to leave old data behind.
func configValues(cfg *Config) (configDocument, error) {
	result := make(configDocument)
	value := reflect.ValueOf(cfg).Elem()
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		encoded, err := json.Marshal(value.Field(i).Interface())
		if err != nil {
			return nil, err
		}
		result[name] = encoded
	}
	return result, nil
}

func effectiveConfig(doc configDocument) (*Config, error) {
	defaults := DefaultConfig()
	base, err := configValues(defaults)
	if err != nil {
		return nil, err
	}
	for key, value := range doc {
		if _, known := base[key]; known {
			base[key] = overlayValue(base[key], normalizeLegacyMap(key, value))
		}
	}
	encoded, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(encoded, &cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.PageSize <= 0 {
		return nil, errors.New("page_size must be greater than zero")
	}
	if cfg.MaxIssues < 0 {
		return nil, errors.New("max_issues must be zero (unlimited) or greater")
	}
	return &cfg, nil
}

func overlayValue(base, override json.RawMessage) json.RawMessage {
	if isNull(override) {
		return base
	}
	if values, ok := jsonObject(override); ok {
		merged, _ := jsonObject(base)
		if merged == nil {
			merged = make(configDocument)
		}
		for key, value := range values {
			if resolved := overlayValue(merged[key], value); len(resolved) > 0 {
				merged[key] = resolved
			}
		}
		return encodeValue(merged)
	}
	return override
}

func changedOverride(override, before, after json.RawMessage) (json.RawMessage, bool) {
	oldMap, oldObject := jsonObject(before)
	newMap, newObject := jsonObject(after)
	if newObject && (oldObject || isNull(before)) {
		result, _ := jsonObject(override)
		if result == nil {
			result = make(configDocument)
		}
		for key := range oldMap {
			if _, exists := newMap[key]; !exists {
				delete(result, key)
			}
		}
		for key, value := range newMap {
			if jsonEqual(value, oldMap[key]) {
				continue
			}
			updated, remove := changedOverride(result[key], oldMap[key], value)
			if remove {
				delete(result, key)
			} else {
				result[key] = updated
			}
		}
		if len(result) == 0 {
			return nil, true
		}
		return encodeValue(result), false
	}
	return after, false
}

func valueAt(values configDocument, path []string) (json.RawMessage, bool) {
	for i, part := range path {
		value, found := values[part]
		if !found || i == len(path)-1 {
			return value, found
		}
		values, _ = jsonObject(value)
	}
	return nil, false
}

func setOverride(doc configDocument, path []string, value json.RawMessage, exists bool) {
	if len(path) == 1 {
		if exists {
			doc[path[0]] = value
		} else {
			delete(doc, path[0])
		}
		return
	}
	child, _ := jsonObject(normalizeLegacyMap(path[0], doc[path[0]]))
	if child == nil {
		child = make(configDocument)
	}
	setOverride(child, path[1:], value, exists)
	if len(child) == 0 {
		delete(doc, path[0])
	} else {
		doc[path[0]] = encodeValue(child)
	}
}

func normalizeLegacyMap(key string, raw json.RawMessage) json.RawMessage {
	switch key {
	case "custom_types", "custom_states", "custom_priorities", "filtered_states", "filtered_priorities", "actions":
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			return encodeValue(configDocument{"default": raw})
		}
	}
	return raw
}

func jsonObject(raw json.RawMessage) (configDocument, bool) {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, false
	}
	var result configDocument
	if json.Unmarshal(raw, &result) != nil {
		return nil, false
	}
	return result, true
}

func encodeValue(value any) json.RawMessage {
	encoded, _ := json.Marshal(value) // Internal values are already valid JSON.
	return encoded
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func jsonEqual(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

func isZeroJSON(raw json.RawMessage) bool {
	return isNull(raw) || jsonEqual(raw, json.RawMessage(`false`)) ||
		jsonEqual(raw, json.RawMessage(`0`)) || jsonEqual(raw, json.RawMessage(`""`))
}

func configurationPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".config", "yt-tui", "config.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", fmt.Errorf("resolve configuration symlink: %w", err)
		}
		info, err = os.Stat(path)
		if err != nil {
			return "", err
		}
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("configuration path must point to a regular file")
	}
	return path, nil
}

// Reading validates before migrating. A fallback Config returned with an error
// cannot later overwrite malformed or newer-version data through SaveConfig.
func readDocument(path string) (configDocument, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	doc, ok := jsonObject(data)
	if !ok {
		return nil, errors.New("configuration must be a valid JSON object")
	}
	version := 0
	if raw, present := doc["config_version"]; present {
		if err := json.Unmarshal(raw, &version); err != nil || isNull(raw) {
			return nil, errors.New("config_version must be an integer")
		}
	}
	if version < 0 || version > ConfigVersion {
		return nil, fmt.Errorf("unsupported config_version %d", version)
	}
	if version == ConfigVersion {
		if _, err := effectiveConfig(doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	migrated := migrateLegacy(doc)
	if _, err := effectiveConfig(migrated); err != nil {
		return nil, err
	}
	if err := backupLegacy(path, data); err != nil {
		return nil, err
	}
	if err := writeDocument(path, migrated); err != nil {
		return nil, err
	}
	return migrated, nil
}

func backupLegacy(path string, data []byte) error {
	backup, err := os.CreateTemp(filepath.Dir(path), "config.json.before-v1-*")
	if err != nil {
		return fmt.Errorf("back up legacy configuration: %w", err)
	}
	complete := false
	defer func() {
		backup.Close()
		if !complete {
			os.Remove(backup.Name())
		}
	}()
	if _, err := backup.Write(data); err != nil {
		return err
	}
	if err := backup.Sync(); err != nil {
		return err
	}
	if err := backup.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func writeDocument(path string, doc configDocument) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.json.tmp-*")
	if err != nil {
		return err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
