package wooh

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site              string      `yaml:"site"`
	OpenAIKey         string      `yaml:"openai_key"`
	WpUser            string      `yaml:"wp_user"`
	WpKey             string      `yaml:"wp_key"`
	WooConsumerKey    string      `yaml:"consumer_key"`
	WooConsumerSecret string      `yaml:"consumer_secret"`
	CacheFilename     string      `yaml:"cache_filename"`
	TrackerFilename   string      `yaml:"tracker_filename"`
	ProductMeta       ProductMeta `yaml:"product_meta"`
}
type ProductCache struct {
	Products   []map[string]interface{} `json:"products"`
	LastUpdate time.Time                `json:"last_update"`
	mu         sync.Mutex               // to guard concurrent access (if needed)
}
type TrackerUpdate struct {
	UpdatedIDs map[int]bool `json:"updated_ids"`
	mu         sync.Mutex
}

func TrackerLoad(filename string) (*TrackerUpdate, error) {
	t := &TrackerUpdate{UpdatedIDs: make(map[int]bool)}
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, err
	}
	return t, nil
}
func (t *TrackerUpdate) save(trackerFilepath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(t)
	ErrChk(err)
	return os.WriteFile(trackerFilepath, data, 0644)
}
func (pc *ProductCache) FetchFromCache(cacheFilePath string, maxAge time.Duration) ([]map[string]interface{}, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}
	if err := json.Unmarshal(data, pc); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}
	if time.Since(pc.LastUpdate) <= maxAge {
		log.Println("Returning products from cache...")
		return pc.Products, nil
	}
	return nil, nil
}
func (pc *ProductCache) SaveToCache(cacheFilePath string, products []WooProduct) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	var productMaps []map[string]interface{}
	for _, p := range products {
		productMap := map[string]interface{}{
			"id":                p.ID,
			"name":              p.Name,
			"description":       p.Description,
			"short_description": p.ShortDescription,
			"categories":        p.Categories,
			"meta_data":         p.MetaData,
		}
		productMaps = append(productMaps, productMap)
	}

	pc.Products = productMaps
	pc.LastUpdate = time.Now()

	data, err := json.Marshal(pc)
	if err != nil {
		log.Printf("Warning: could not marshal cache: %v", err)
		return
	}

	dir := filepath.Dir(cacheFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Warning: could not create directory for cache file: %v", err)
		return
	}

	if err := os.WriteFile(cacheFilePath, data, 0644); err != nil {
		log.Printf("Warning: could not save cache file: %v", err)
	}
}

func Contains(strRange []string, pattern string) bool {
	for _, val := range strRange {
		match, _ := regexp.MatchString(pattern, val)
		return match
	}

	return false
}
func ErrChk(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func Filter(arr []string, cond func(string) bool) []string {
	result := []string{}
	for i := range arr {
		if cond(arr[i]) {
			result = append(result, arr[i])
		}
	}
	return result
}
func GetConfig(configPath string) (*Config, error) {
	defaultConfig := &Config{
		Site:              "domain.com",
		WpUser:            "user",
		WpKey:             "",
		WooConsumerKey:    "woo_consumer_key",
		WooConsumerSecret: "woo_consumer_secret",
		TrackerFilename:   "tracker-state.json",
		CacheFilename:     "products-cache.json",
		ProductMeta: ProductMeta{
			Type:             "simple",
			RegularPrice:     "0.00",
			Description:      "Product description",
			ShortDescription: "Short Product Description",
			Categories: []interface{}{
				1, // Using integer for the default category
			},
		},
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := WriteDefaultConfig(configPath, defaultConfig); err != nil {
			return nil, err
		}
		return defaultConfig, nil
	}

	return ReadConfig(configPath)
}
func PathExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}
func ReadConfig(configPath string) (*Config, error) {
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(configFile, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	return config, nil
}
func WriteDefaultConfig(configPath string, defaultConfig *Config) error {
	yamlData, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(configPath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Config file created at %s\n", configPath)
	return nil
}
