package wooh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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
type ProductMeta struct {
	Name             *string
	Type             string        `yaml:"type"`
	RegularPrice     string        `yaml:"regular_price"`
	Description      string        `yaml:"description"`
	ShortDescription string        `yaml:"short_description"`
	Categories       []interface{} `yaml:"categories"`
}

// -------------------------------------------------------------------
// File-based cache for products
// -------------------------------------------------------------------
type productCache struct {
	Products   []map[string]interface{} `json:"products"`
	LastUpdate time.Time                `json:"last_update"`
	mu         sync.Mutex               // to guard concurrent access (if needed)
}

func (pc *productCache) fetchFromCache(cacheFile string, maxAge time.Duration) ([]map[string]interface{}, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		// If file doesn't exist, it's not an error; just means no cache
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}
	if err := json.Unmarshal(data, pc); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}
	// Check cache freshness
	if time.Since(pc.LastUpdate) <= maxAge {
		log.Println("Returning products from cache...")
		return pc.Products, nil
	}
	// Cache is stale
	return nil, nil
}
func (pc *productCache) saveToCache(cacheFile string, products []map[string]interface{}) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.Products = products
	pc.LastUpdate = time.Now()

	data, err := json.Marshal(pc)
	if err != nil {
		log.Printf("Warning: could not marshal cache: %v", err)
		return
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		log.Printf("Warning: could not save cache file: %v", err)
	}
}

// -------------------------------------------------------------------
// Config Operations
// -------------------------------------------------------------------
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
		if err := WriteDefaultConfig(configPath, defaultConfig); err != nil {
			return nil, err
		}
		return defaultConfig, nil
	}

	return ReadConfig(configPath)
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

// -------------------------------------------------------------------
// Fetch WooCommerce products, with cache
// -------------------------------------------------------------------
func GetProducts(conf *Config, cacheFile string, maxCacheAge time.Duration) ([]map[string]interface{}, error) {
	var pc productCache

	// Try fetching from cache
	if cached, err := pc.fetchFromCache(cacheFile, maxCacheAge); err != nil {
		log.Printf("Cache read error: %v", err)
	} else if cached != nil {
		return cached, nil
	}

	// Cache is stale or missing, so fetch all pages from WooCommerce
	log.Println("Fetching all products from API (paginated)...")

	client := resty.New()
	allProducts := make([]map[string]interface{}, 0)
	page, perPage := 1, 100

	for {
		resp, err := client.R().
			SetHeader("Accept", "application/json").
			SetQueryParams(map[string]string{
				"page":     fmt.Sprintf("%d", page),
				"per_page": fmt.Sprintf("%d", perPage),
			}).
			Get(fmt.Sprintf(
				"https://%s/wp-json/wc/v3/products?consumer_key=%s&consumer_secret=%s",
				conf.Site, conf.WooConsumerKey, conf.WooConsumerSecret,
			))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch products on page %d: %w", page, err)
		}
		if resp.IsError() {
			return nil, fmt.Errorf("error fetching page %d: %s, %s", page, resp.Status(), resp.String())
		}

		var products []map[string]interface{}
		if err := json.Unmarshal(resp.Body(), &products); err != nil {
			return nil, fmt.Errorf("failed to parse products on page %d: %w", page, err)
		}

		allProducts = append(allProducts, products...)
		if len(products) < perPage {
			// no more pages
			break
		}
		page++
	}

	// Save to cache for next run
	pc.saveToCache(cacheFile, allProducts)
	return allProducts, nil
}

// -------------------------------------------------------------------
// OpenAI logic (unchanged)
// -------------------------------------------------------------------
func OpenAIProcess(conf *Config, productName, shortDescription, description string, categories []interface{}) (string, string, error) {
	client := openai.NewClient(conf.OpenAIKey)
	prompt := fmt.Sprintf(`
You are an experienced SEO specialist and copywriter with expertise in flooring materials like RVP (Rigid Vinyl Plank) and LVT (Luxury Vinyl Tile).

I will provide:
- A product’s name
- A short description
- A detailed description (in Markdown/HTML)
- A list of categories.

Your task is to:
1. Understand the key product attributes, especially if it is RVP or LVT, and incorporate their unique features where applicable:
   - **RVP (Rigid Vinyl Plank)**: Mention its rigid SPC core (Stone Polymer Composite) for dimensional stability, ultra-strong rigid construction for flat installation, and built-in acoustic underlay for sound absorption (e.g., 19db impact reduction). Highlight its fast deployment if the foundation is perfectly level.
   - **LVT (Luxury Vinyl Tile)**: Emphasize it is multi-layered vinyl that replicates wood or stone, offering a durable, low-maintenance, and visually appealing solution.

2. Create an SEO-friendly **meta title** (up to **60 characters**) that:
   - Clearly identifies the product type (e.g., RVP or LVT).
   - Highlights its unique benefits or specifications.
   - Is concise, compelling, and within the 60-character limit.

3. Generate an SEO-friendly **meta description** (up to **160 characters**) that:
   - Clearly explains the product and its use cases.
   - Summarizes its unique features and benefits.
   - Is concise, natural, and strictly under 160 characters.

**Examples of Titles and Descriptions**:
- Meta Title Example: "RVP Flooring with SPC Core | Durable & Quiet"
- Meta Description Example: "Rigid Vinyl Plank with SPC core for stability, sound absorption, and fast installation. Perfect for level floors."

4. Output your response as **valid JSON**, formatted like this:

{
  "meta_title": "Your meta title here",
  "meta_description": "Your meta description here"
}

Important:
- The **meta title** must be 60 characters or fewer.
- The **meta description** must be 160 characters or fewer.
- Use natural, human-readable language.
- Do not include anything except the JSON object in your response.
- Ensure the JSON is valid and properly escaped.

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

// -------------------------------------------------------------------
// Helper to convert HTML to Markdown (unchanged)
// -------------------------------------------------------------------
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

// -------------------------------------------------------------------
// SEO update tracker struct + helpers
// -------------------------------------------------------------------
type seoUpdateTracker struct {
	UpdatedIDs map[int]bool `json:"updated_ids"`
	mu         sync.Mutex
}

func loadSEOUpdateTracker(filename string) (*seoUpdateTracker, error) {
	t := &seoUpdateTracker{UpdatedIDs: make(map[int]bool)}
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// no file yet, return empty
			return t, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *seoUpdateTracker) save(filename string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// -------------------------------------------------------------------
// UpdateSEO now has a restartTracking param and uses the tracker
// -------------------------------------------------------------------
func UpdateSEO(conf *Config, restartTracking bool) error {
	client := resty.New()

	trackerFile := "seo-update-tracker.json"
	var tracker *seoUpdateTracker
	fmt.Println("Starting SEO update...: ", restartTracking)
	if restartTracking {
		// Clear old data
		tracker = &seoUpdateTracker{UpdatedIDs: make(map[int]bool)}
	} else {
		// Load existing file if any
		var err error
		tracker, err = loadSEOUpdateTracker(trackerFile)
		if err != nil {
			return fmt.Errorf("failed to load SEO update tracker: %w", err)
		}
	}

	// Get products
	cacheFile := "products-cache.json"
	maxCacheAge := 24 * time.Hour
	products, err := GetProducts(conf, cacheFile, maxCacheAge)
	if err != nil {
		return fmt.Errorf("failed to fetch products: %w", err)
	}
	fmt.Printf("Products To Be Processed: %d\n", len(products))

	// reader := bufio.NewReader(os.Stdin)

	for _, product := range products {
		rawID := product["id"]
		idFloat, ok := rawID.(float64)
		if !ok {
			log.Printf("Skipping product with invalid ID type: %v\n", rawID)
			continue
		}

		productID := int(idFloat)
		if tracker.UpdatedIDs[productID] {
			log.Printf("Skipping product ID %v (already updated)\n", productID)
			continue
		} else {
			log.Printf("Processing product ID %v\n", productID)
		}

		productName, _ := product["name"].(string)
		shortDescription, _ := product["short_description"].(string)
		description, _ := product["description"].(string)
		categories, _ := product["categories"].([]interface{})

		cleanedDescription, err := cleanHTMLToMarkdown(description)
		if err != nil {
			return fmt.Errorf("failed to clean description for product ID %v: %w", productID, err)
		}

		const maxTitleLength = 60
		const maxDescriptionLength = 160

		var metaTitle, metaDescription string
		retries := 10

		for i := 0; i < retries; i++ {
			metaTitle, metaDescription, err = OpenAIProcess(conf, productName, shortDescription, cleanedDescription, categories)
			if err != nil {
				log.Printf("Error generating meta fields for product ID %v: %v", productID, err)
				continue
			}
			if len(metaTitle) <= maxTitleLength && len(metaDescription) <= maxDescriptionLength {
				break
			} else {
				log.Printf("Meta fields exceeded char limits for product ID %v (attempt %d/%d)", productID, i+1, retries)
			}
		}

		if len(metaTitle) > maxTitleLength || len(metaDescription) > maxDescriptionLength {
			log.Printf("Failed to generate valid meta fields for product ID %v after %d retries", productID, retries)
			continue
		}

		fmt.Println("Meta Title: " + metaTitle)
		fmt.Println("Meta Description: " + metaDescription)

		skipThisProduct := false
		// for {
		// 	fmt.Println("Do you approve these values? (y/n): ")
		// 	input, _ := reader.ReadString('\n')
		// 	input = strings.TrimSpace(input)

		// 	if input == "y" {
		// 		break
		// 	} else if input == "n" {
		// 		fmt.Println("Skipping this product...")
		// 		skipThisProduct = true
		// 		break
		// 	} else {
		// 		fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		// 	}
		// }

		if skipThisProduct {
			continue
		}

		// Update the product on WooCommerce
		updatePayload := map[string]interface{}{
			"meta_data": []map[string]string{
				{
					"key":   "_yoast_wpseo_title",
					"value": metaTitle,
				},
				{
					"key":   "_yoast_wpseo_metadesc",
					"value": metaDescription,
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

		// Mark this ID as updated in the tracker
		tracker.UpdatedIDs[productID] = true
		if err := tracker.save(trackerFile); err != nil {
			log.Printf("Warning: could not save SEO tracker file: %v", err)
		}
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
