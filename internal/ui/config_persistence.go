package ui

import (
	"yt-tui/internal/config"
	"yt-tui/internal/filepicker"
)

// Remember navigation without freezing sort defaults merely because the picker
// was opened. Empty preference strings use the picker's built-in defaults.
func saveFilepickerPreferences(cfg *config.Config, picker filepicker.Model) error {
	previousBy := filepicker.SortByName
	if cfg.FilepickerSortBy == "Datetime" {
		previousBy = filepicker.SortByDatetime
	}
	previousOrder := filepicker.SortAscending
	if cfg.FilepickerSortOrder == "desc" {
		previousOrder = filepicker.SortDescending
	}
	if picker.SortBy != previousBy {
		cfg.FilepickerSortBy = picker.SortBy.String()
	}
	if picker.SortOrder != previousOrder {
		cfg.FilepickerSortOrder = picker.SortOrder.String()
	}
	cfg.FilepickerLastDir = picker.CurrentDirectory
	return config.SaveConfig(cfg, "filepicker_last_dir")
}
