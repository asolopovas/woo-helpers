package wooh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-resty/resty/v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Site              string      `yaml:"site"`
	WpUser            string      `yaml:"wp_user"`
	WpKey             string      `yaml:"wp_key"`
	WooConsumerKey    string      `yaml:"consumer_key"`
	WooConsumerSecret string      `yaml:"consumer_secret"`
	ProductMeta       ProductMeta `yaml:"product_meta"`
}

type ProductMeta struct {
	Name             *string
	Type             string        `yaml:"type"`
	RegularPrice     string        `yaml:"regular_price"`
	Description      string        `yaml:"description"`
	ShortDescription string        `yaml:"short_description"`
	Categories       []interface{} `yaml:"categories"`
}

type Category struct {
	ID string `yaml:"id"`
}

func GetConfig(configPath string) (*Config, error) {
	defaultConfig := &Config{
		Site:              "domain.com",
		WpUser:            "user",
		WpKey:             "",
		WooConsumerKey:    "woo_consumer_key",
		WooConsumerSecret: "woo_consumer_secret",
		ProductMeta: ProductMeta{
			Type:             "simple",
			RegularPrice:     "0.00",
			Description:      "Product description",
			ShortDescription: "Short Product Descriptio",
			Categories: []interface{}{
				1, // Using integer for the default category
			},
		},
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := writeDefaultConfig(configPath, defaultConfig); err != nil {
			return nil, err
		}
		return defaultConfig, nil
	}

	return readConfig(configPath)
}

func writeDefaultConfig(configPath string, defaultConfig *Config) error {
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

func readConfig(configPath string) (*Config, error) {
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

func UploadImageToWordPress(conf *Config, imageDirPath string) error {
	client := resty.New()

	files, err := os.ReadDir(imageDirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && Contains([]string{".jpg", ".jpeg", ".png", ".gif"}, filepath.Ext(file.Name())) {
			imagePath := filepath.Join(imageDirPath, file.Name())
			fileName := file.Name()
			productName := fileName[:len(fileName)-len(filepath.Ext(fileName))]

			uploadEndpoint := fmt.Sprintf("https://%s/wp-json/wp/v2/media", conf.Site)

			resp, err := client.R().
				SetBasicAuth(conf.WpUser, conf.WpKey).
				SetFile("file", imagePath).
				SetFormData(map[string]string{
					"title":   productName,
					"caption": conf.ProductMeta.Description,
				}).
				Post(uploadEndpoint)
			if err != nil {
				return fmt.Errorf("failed to upload image: %w", err)
			}

			if resp.IsError() {
				return fmt.Errorf("failed to upload image: %s, %s", resp.Status(), resp.String())
			}

			var result map[string]interface{}
			if err := json.Unmarshal(resp.Body(), &result); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}
			imageURL := result["source_url"].(string)
			imageID := result["id"].(float64)

			uploadedImages := []map[string]interface{}{
				{
					"id":  imageID,
					"src": imageURL,
				},
			}

			if len(uploadedImages) > 0 {
				productEndpoint := fmt.Sprintf(
					"https://%s/wp-json/wc/v3/products?consumer_key=%s&consumer_secret=%s",
					conf.Site, conf.WooConsumerKey, conf.WooConsumerSecret,
				)
				fmt.Println("Creating product: " + productName)

				var formattedCategories []map[string]interface{}
				for _, category := range conf.ProductMeta.Categories {
					switch v := category.(type) {
					case int:
						formattedCategories = append(formattedCategories, map[string]interface{}{"id": v})
					case string:
						formattedCategories = append(formattedCategories, map[string]interface{}{"id": v})
					}
				}

				body := map[string]interface{}{
					"name":              &productName,
					"type":              conf.ProductMeta.Type,
					"regular_price":     conf.ProductMeta.RegularPrice,
					"description":       conf.ProductMeta.Description,
					"short_description": conf.ProductMeta.ShortDescription,
					"categories":        formattedCategories,
					"images":            &uploadedImages,
				}

				productResp, err := client.R().
					SetHeader("Content-Type", "application/json").
					SetBody(body).
					Post(productEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create product: %w", err)
				}

				if productResp.IsError() {
					return fmt.Errorf("failed to create product: %s, %s", productResp.Status(), productResp.String())
				}

				fmt.Println("Product created")
			}
		}

	}

	return nil
}
