/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package cmd provides CLI commands for the tenant migration tool
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-bscp/cmd/tenant-migration/config"
	"github.com/TencentBlueKing/bk-bscp/cmd/tenant-migration/migrator"
)

var (
	cfgFile string
	cfg     *config.Config
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "bk-bscp-tenant-migration",
	Short: "BSCP Tenant Migration Tool",
	Long: `A tool for migrating BSCP data from single-tenant environment 
to multi-tenant environment.

This tool handles:
- MySQL data migration with tenant_id population
- Vault KV data migration via API
- Data validation after migration`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (required)")
	rootCmd.MarkPersistentFlagRequired("config")

	// Add subcommands
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	if cfgFile == "" {
		return
	}

	var err error
	cfg, err = config.LoadConfig(cfgFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}
}

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run data migration",
	Long:  `Migrate data from source environment to target environment.`,
}

// migrateMySQLCmd migrates only MySQL data
var migrateMySQLCmd = &cobra.Command{
	Use:   "mysql",
	Short: "Migrate MySQL data only",
	Long:  `Migrate MySQL data from source to target database with tenant_id population.`,
	Run: func(cmd *cobra.Command, args []string) {
		if cfg == nil {
			fmt.Println("Error: configuration not loaded")
			os.Exit(1)
		}

		m, err := migrator.NewMigrator(cfg)
		if err != nil {
			fmt.Printf("Error creating migrator: %v\n", err)
			os.Exit(1)
		}
		defer m.Close()

		report, err := m.RunMySQL()
		if err != nil {
			fmt.Printf("Error during migration: %v\n", err)
		}

		m.PrintReport(report)

		if !report.Success {
			os.Exit(1)
		}
	},
}

// migrateVaultCmd migrates only Vault data
var migrateVaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Migrate Vault KV data only",
	Long:  `Migrate Vault KV data from source to target Vault via API.`,
	Run: func(cmd *cobra.Command, args []string) {
		if cfg == nil {
			fmt.Println("Error: configuration not loaded")
			os.Exit(1)
		}

		if cfg.Source.Vault.Address == "" || cfg.Target.Vault.Address == "" {
			fmt.Println("Error: Vault configuration is required for vault migration")
			os.Exit(1)
		}

		m, err := migrator.NewMigrator(cfg)
		if err != nil {
			fmt.Printf("Error creating migrator: %v\n", err)
			os.Exit(1)
		}
		defer m.Close()

		report, err := m.RunVault()
		if err != nil {
			fmt.Printf("Error during migration: %v\n", err)
		}

		m.PrintReport(report)

		if !report.Success {
			os.Exit(1)
		}
	},
}

// migrateAllCmd migrates both MySQL and Vault data
var migrateAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Migrate all data (MySQL + Vault)",
	Long:  `Migrate all data from source to target environment, including MySQL and Vault.`,
	Run: func(cmd *cobra.Command, args []string) {
		if cfg == nil {
			fmt.Println("Error: configuration not loaded")
			os.Exit(1)
		}

		m, err := migrator.NewMigrator(cfg)
		if err != nil {
			fmt.Printf("Error creating migrator: %v\n", err)
			os.Exit(1)
		}
		defer m.Close()

		report, err := m.RunAll()
		if err != nil {
			fmt.Printf("Error during migration: %v\n", err)
		}

		m.PrintReport(report)

		if !report.Success {
			os.Exit(1)
		}
	},
}

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate migrated data",
	Long: `Validate that data was migrated correctly by comparing 
source and target databases.`,
	Run: func(cmd *cobra.Command, args []string) {
		if cfg == nil {
			fmt.Println("Error: configuration not loaded")
			os.Exit(1)
		}

		m, err := migrator.NewMigrator(cfg)
		if err != nil {
			fmt.Printf("Error creating migrator: %v\n", err)
			os.Exit(1)
		}
		defer m.Close()

		report, err := m.Validate()
		if err != nil {
			fmt.Printf("Error during validation: %v\n", err)
			os.Exit(1)
		}

		// Use validator's print report
		v := migrator.NewValidator(cfg, nil, nil)
		v.PrintReport(report)

		if !report.Success {
			os.Exit(1)
		}
	},
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("bk-bscp-tenant-migration v1.0.0")
		fmt.Println("Single-tenant to Multi-tenant Data Migration Tool")
	},
}

func init() {
	// Add migrate subcommands
	migrateCmd.AddCommand(migrateMySQLCmd)
	migrateCmd.AddCommand(migrateVaultCmd)
	migrateCmd.AddCommand(migrateAllCmd)
}
