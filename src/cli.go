package wooh

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

func Run() {
	if err := newRootCmd().Execute(); err != nil {
		log.Fatal(err)
	}
}

func newRootCmd() *cobra.Command {
	var (
		showVersion     bool
		configPath      string
		imagesPath      string
		autofill        bool
		resetAutoFill   bool
		prompt          bool
		listProductMeta bool
	)

	_, currentFilePath, _, ok := runtime.Caller(0)
	if !ok {
		panic("No caller information")
	}
	dirPath := filepath.Dir(currentFilePath)
	versionFilePath := filepath.Join(dirPath, "/../version")

	ver, err := os.ReadFile(versionFilePath)
	ErrChk(err)

	var rootCmd = &cobra.Command{
		Use:   "wooh",
		Short: "Tool that helps turn images into woo commerce products" + string(ver),
		Run: func(cmd *cobra.Command, args []string) {
			if showVersion {
				fmt.Println(string(ver))
				return
			}

			imagesPath, err = filepath.Abs(imagesPath)
			if err != nil {
				log.Fatalf("Failed to get absolute path: %v", err)
			}

			if configPath == "wooh.yaml" {
				configPath, err = filepath.Abs(configPath)
				if err != nil {
					log.Fatalf("Failed to get absolute path: %v", err)
				}
			}

			conf, err := GetConfig(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config file '%s': %v\n", configPath, err)
				cmd.Help()
				return
			}

			if configPath != "" && PathExist(imagesPath) {
				UploadImageToWordPress(conf, imagesPath)
			}

			if autofill {
				UpdateSEO(conf, resetAutoFill, prompt)
			}

			if listProductMeta {
				ListProductMeta(conf)
			}

		}}

	rootCmd.Flags().BoolVarP(&autofill, "autofill", "a", false, "Yoast SEO Meta Data Autofill")
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "wooh.yaml", "Custom config path")
	rootCmd.Flags().StringVarP(&imagesPath, "images-path", "i", ".", "Images Path")
	rootCmd.Flags().BoolVarP(&listProductMeta, "listProductMeta", "l", false, "List Product Meta")
	rootCmd.Flags().BoolVarP(&prompt, "prompt", "p", false, "Prompt for confirmation for each product")
	rootCmd.Flags().BoolVarP(&resetAutoFill, "resetAutofill", "r", false, "Reset Yoast Cache and Products Data")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Get Version")

	rootCmd.AddCommand(newCompletionCmd())

	return rootCmd
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion",
		Short: "Generate fish completion script",
		Run:   generateFishCompletion,
	}
}

func generateFishCompletion(cmd *cobra.Command, args []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("failed to get user home directory: %v", err)
	}

	fishCompletionDir := filepath.Join(homeDir, ".config", "fish", "completions")
	if err := os.MkdirAll(fishCompletionDir, os.ModePerm); err != nil {
		log.Fatalf("failed to create fish completions directory: %v", err)
	}

	fishCompletionFile := filepath.Join(fishCompletionDir, "gen-webmanifest.fish")
	f, err := os.Create(fishCompletionFile)
	if err != nil {
		log.Fatalf("failed to create fish completion file: %v", err)
	}
	defer f.Close()

	if err := cmd.Root().GenFishCompletion(f, true); err != nil {
		log.Fatalf("failed to generate fish completion script: %v", err)
	}

	fmt.Printf("Fish completion script generated at: %s\n", fishCompletionFile)
}
