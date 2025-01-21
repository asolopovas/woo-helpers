package wooh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	// imported as openai
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-resty/resty/v2"             // imported as openai
	openai "github.com/sashabaranov/go-openai" // For OpenAI usage
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
	// Convert HTML to Markdown
	markdown, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		log.Fatal(err)
	}

	// Replace #### with ## for better readability
	markdown = strings.ReplaceAll(markdown, "####", "##")

	// Remove all images in the form ![](url)
	// Regex pattern to match images in Markdown
	imageRegex := regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	markdown = imageRegex.ReplaceAllString(markdown, "")

	// Ensure there's a maximum of one newline between lines
	// Replace multiple newlines (\n) with a single newline
	newlineRegex := regexp.MustCompile(`\n{2,}`)
	markdown = newlineRegex.ReplaceAllString(markdown, "\n")

	// Trim any leading or trailing whitespace or newlines
	markdown = strings.TrimSpace(markdown)

	return markdown, nil
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
	client := openai.NewClient(conf.OpenAIKey)
	prompt := fmt.Sprintf(`
You are an experienced SEO specialist.
I will provide a product’s name, a short description, a detailed description (in Markdown/HTML),
and a list of categories.

Your task:
1. Read and understand the product information.
2. Generate a concise and informative meta title (up to 60 characters).
3. Generate a meta description (up to 160 characters) that follows Google's best practices:
   - Accurately summarize the product.
   - Use natural, human-readable language.
   - Avoid keyword stuffing; keep it relevant and helpful.
   - Include key specs only if they add real value.
4. Format your response strictly as valid JSON in the form:

{
  "meta_title": "Your meta title here",
  "meta_description": "Your meta description here"
}

Important notes:
- Do not include anything except the JSON object in your response.
- Make sure the JSON is properly escaped and valid.

Here is the product information:

- Product Name: %s
- Short Description: %s
- Full Description (may include Markdown/HTML): %s
- Categories: %v
`, productName, shortDescription, description, categories)

	// Create the chat completion request
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.7,
		},
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to get chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", "", fmt.Errorf("no choices returned by OpenAI API")
	}

	// The model's reply should be valid JSON with "meta_title" and "meta_description"
	content := resp.Choices[0].Message.Content

	// Parse the JSON response
	var parsed map[string]string
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", "", fmt.Errorf("failed to parse JSON: %w; raw content: %s", err, content)
	}

	metaTitle, ok := parsed["meta_title"]
	if !ok {
		return "", "", fmt.Errorf(`JSON response did not include "meta_title"`)
	}

	metaDescription, ok := parsed["meta_description"]
	if !ok {
		return "", "", fmt.Errorf(`JSON response did not include "meta_description"`)
	}

	return metaTitle, metaDescription, nil
}

// updateSEO updates the Yoast SEO meta fields for all WooCommerce products.
func UpdateSEO(conf *Config) error {
	client := resty.New()

	products, err := GetProducts(conf)
	if err != nil {
		return fmt.Errorf("failed to fetch products: %w", err)
	}
	reader := bufio.NewReader(os.Stdin) // For user input

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

		// Constants for character limits
		const maxTitleLength = 60
		const maxDescriptionLength = 160

		var metaTitle, metaDescription string
		retries := 5

		// Retry loop for generating valid meta title and description
		for i := 0; i < retries; i++ {
			metaTitle, metaDescription, err = OpenAIProcess(conf, productName, shortDescription, cleanedDescription, categories)
			if err != nil {
				log.Printf("Error generating meta fields for product ID %v: %v", productID, err)
				continue
			}

			// Validate the length of the meta fields
			if len(metaTitle) <= maxTitleLength && len(metaDescription) <= maxDescriptionLength {
				break // Valid meta fields
			} else {
				log.Printf("Meta fields exceeded character limits for product ID %v (attempt %d/%d)", productID, i+1, retries)
			}
		}

		// Check if valid meta fields were generated after retries
		if len(metaTitle) > maxTitleLength || len(metaDescription) > maxDescriptionLength {
			log.Printf("Failed to generate valid meta fields for product ID %v after %d retries", productID, retries)
			continue
		}

		fmt.Println("Meta Title: " + metaTitle)
		fmt.Println("Meta Description: " + metaDescription)

		for {
			fmt.Println("Do you approve these values? (y/n): ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input) // Remove whitespace and newline characters

			if input == "y" {
				break // Proceed to the next product
			} else if input == "n" {
				fmt.Println("Skipping this product...")
				continue // Skip the update for this product
			} else {
				fmt.Println("Invalid input. Please enter 'y' for yes or 'n' for no.")
			}
		}

		// Update the product's Yoast SEO fields
		updatePayload := map[string]interface{}{
			"meta_data": []map[string]interface{}{
				{
					"key":   "yoast_head_json",
					"value": map[string]string{"title": metaTitle, "og_description": metaDescription},
				},
			},
		}

		productEndpoint := fmt.Sprintf(
			"https://%s/wp-json/wc/v3/products/%v?consumer_key=%s&consumer_secret=%s",
			conf.Site, productID, conf.WooConsumerKey, conf.WooConsumerSecret,
		)

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(updatePayload).
			Put(productEndpoint)

		if err != nil {
			log.Printf("Failed to update SEO for product ID %v: %v", productID, err)
			continue
		}

		if resp.IsError() {
			log.Printf("API error updating SEO for product ID %v: %s", productID, resp.String())
			continue
		}

		log.Printf("Successfully updated SEO for product ID %v", productID)
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
