package wooh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	// imported as openai
	"github.com/go-resty/resty/v2"
	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Site              string      `yaml:"site"`
	OpenAIKey         string      `yaml:"openai_key"`
	WpUser            string      `yaml:"wp_user"`
	WpKey             string      `yaml:"wp_key"`
	WooConsumerKey    string      `yaml:"consumer_key"`
	WooConsumerSecret string      `yaml:"consumer_secret"`
	ProductMeta       ProductMeta `yaml:"product_meta"`
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
			ShortDescription: "Short Product Description",
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

type ProductMeta struct {
	Name             *string
	Type             string        `yaml:"type"`
	RegularPrice     string        `yaml:"regular_price"`
	Description      string        `yaml:"description"`
	ShortDescription string        `yaml:"short_description"`
	Categories       []interface{} `yaml:"categories"`
}

func cleanHTMLToMarkdown(html string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(html), &buf); err != nil {
		return "", fmt.Errorf("failed to convert HTML to Markdown: %w", err)
	}
	return buf.String(), nil
}

func getProductsEndpoint(conf *Config) string {
	return fmt.Sprintf(
		"https://%s/wp-json/wc/v3/products?consumer_key=%s&consumer_secret=%s",
		conf.Site, conf.WooConsumerKey, conf.WooConsumerSecret,
	)
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

func GetProducts(conf *Config) ([]map[string]interface{}, error) {
	client := resty.New()

	productsEndpoint := getProductsEndpoint(conf)

	resp, err := client.R().
		SetHeader("Accept", "application/json").
		Get(productsEndpoint)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch products: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("error fetching products: %s, %s", resp.Status(), resp.String())
	}

	var products []map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &products); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return products, nil
}

func OpenAIProcess(conf *Config, productName, shortDescription, description string, categories []interface{}) (string, string, error) {
	// client := openai.NewClient(
	// 	option.WithAPIKey(conf.OpenAIKey),
	// )

	// Prepare prompt for OpenAI
	// 	prompt := fmt.Sprintf(`Generate a meta title and meta description for a product with the following details:
	// - Product Name: %s
	// - Short Description: %s
	// - Description: %s
	// - Categories: %v

	// Respond with a JSON object containing "meta_title" and "meta_description".`, productName, shortDescription, description, categories)

	// chatCompletion, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
	// 	Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
	// 		openai.UserMessage(prompt),
	// 	}),
	// 	Model: openai.F(openai.ChatModelGPT4o),
	// })
	// if err != nil {
	// 	return "", "", fmt.Errorf("failed to process OpenAI API: %w", err)
	// }

	// var response map[string]string
	// if err := json.Unmarshal([]byte(chatCompletion.Choices[0].Message.Content), &response); err != nil {
	// 	return "", "", fmt.Errorf("failed to parse OpenAI response: %w", err)
	// }

	// return response["meta_title"], response["meta_description"], nil

	fmt.Println(productName, shortDescription, description, categories)
	return "", "", nil

}

// updateSEO updates the Yoast SEO meta fields for all WooCommerce products.
func UpdateSEO(conf *Config) error {
	// client := resty.New()

	products, err := GetProducts(conf)
	if err != nil {
		return fmt.Errorf("failed to fetch products: %w", err)
	}

	for _, product := range products {
		productID := product["id"]
		productName, _ := product["name"].(string)
		shortDescription, _ := product["short_description"].(string)
		description, _ := product["description"].(string)
		categories, _ := product["categories"].([]interface{})

		cleanedDescription, err := cleanHTMLToMarkdown(description)
		if err != nil {
			return fmt.Errorf("failed to clean description for product ID %v: %w", productID, err)
		}
		fmt.Println("-------------------------")
		fmt.Println(cleanedDescription)
		fmt.Println("-------------------------")

		metaTitle, metaDescription, err := OpenAIProcess(conf, productName, shortDescription, cleanedDescription, categories)
		if err != nil {
			return fmt.Errorf("failed to process OpenAI for product ID %v: %w", productID, err)
		}

		fmt.Println(metaTitle, metaDescription)

		// Update the product's Yoast SEO fields
		// updatePayload := map[string]interface{}{
		// 	"meta_data": []map[string]interface{}{
		// 		{
		// 			"key":   "yoast_head_json",
		// 			"value": map[string]string{"title": metaTitle, "og_description": metaDescription},
		// 		},
		// 	},
		// }

		// productEndpoint := fmt.Sprintf(
		// 	"https://%s/wp-json/wc/v3/products/%v?consumer_key=%s&consumer_secret=%s",
		// 	conf.Site, productID, conf.WooConsumerKey, conf.WooConsumerSecret,
		// )

		// resp, err := client.R().
		// 	SetHeader("Content-Type", "application/json").
		// 	SetBody(updatePayload).
		// 	Put(productEndpoint)

		// if err != nil {
		// 	log.Printf("Failed to update SEO for product ID %v: %v", productID, err)
		// 	continue
		// }

		// if resp.IsError() {
		// 	log.Printf("API error updating SEO for product ID %v: %s", productID, resp.String())
		// 	continue
		// }

		// log.Printf("Successfully updated SEO for product ID %v", productID)
	}

	return nil
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
