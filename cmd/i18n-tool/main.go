// Package main provides the i18n tool for extracting and managing localization messages.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

// NewRootCmd creates the root command for the i18n tool
func NewRootCmd() *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:   "i18n-tool",
		Short: "YAP Internationalization Tool",
		Long:  "A tool for managing YAP's internationalization files and checking integrity",
	}

	var checkCmd = &cobra.Command{
		Use:   "check",
		Short: "Check integrity of localization files",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Checking localization file integrity...")

			if err := i18n.CheckIntegrity(); err != nil {
				fmt.Printf("❌ Integrity check failed: %v\n", err)
				return err
			}

			fmt.Println("✅ All localization files passed integrity checks")

			return nil
		},
	}

	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List all message IDs",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := i18n.GetMessageIDs()
			if err != nil {
				return err
			}

			fmt.Println("Message IDs:")

			for _, id := range ids {
				fmt.Printf("  - %s\n", id)
			}

			fmt.Printf("\nTotal: %d message IDs\n", len(ids))

			return nil
		},
	}

	var statsCmd = &cobra.Command{
		Use:   "stats",
		Short: "Show localization statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Localization Statistics:")

			// Show supported languages
			supported := i18n.SupportedLanguages
			fmt.Printf("Supported languages: %v\n", supported)

			// Show message count
			ids, err := i18n.GetMessageIDs()
			if err != nil {
				return err
			}

			fmt.Printf("Total messages: %d\n", len(ids))

			return nil
		},
	}

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statsCmd)

	return rootCmd
}

func main() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
