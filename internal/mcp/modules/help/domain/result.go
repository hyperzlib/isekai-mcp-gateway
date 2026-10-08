package domain

type Result struct {
	Topic               string   `json:"topic"`
	URI                 string   `json:"uri"`
	Content             string   `json:"content"`
	AvailableOperations []string `json:"available_operations"`
	EnabledFeatures     []string `json:"enabled_features"`
}
