package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/imMamdouhaboammar/skill-healer/pkg/dashboard"
	"github.com/imMamdouhaboammar/skill-healer/pkg/scanner"
	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

var errCleanupUsage = errors.New("usage: ash cleanup <plan|remove|trash|restore|purge> [flags]")

// cleanupCommand uses the same source restrictions, vault, and metadata as the
// dashboard. Command and flag confirmations are intentionally non-interactive,
// so automation cannot silently remove tools without the exact source hash.
func cleanupCommand(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errCleanupUsage
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	service := dashboard.NewCleanupService(home)
	command := args[0]
	if command == "trash" {
		if len(args) != 1 {
			return errCleanupUsage
		}
		records, err := service.Trash()
		if err != nil {
			return err
		}
		return encodeCleanupJSON(out, records)
	}
	flags := flag.NewFlagSet("ash cleanup "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "Exact discovered tool ID or Trash record ID")
	fingerprint := flags.String("fingerprint", "", "Fingerprint from a fresh cleanup plan")
	confirmation := flags.String("confirm", "", "Exact tool name, or DELETE <name> for permanent purge")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w: %v", errCleanupUsage, err)
	}
	if flags.NArg() != 0 || *id == "" {
		return errCleanupUsage
	}
	switch command {
	case "restore":
		if err := service.Restore(*id); err != nil {
			return err
		}
		return encodeCleanupJSON(out, map[string]any{"ok": true, "restored": *id})
	case "purge":
		if *confirmation == "" {
			return errors.New("permanent deletion requires --confirm 'DELETE <tool-name>'")
		}
		if err := service.Purge(*id, *confirmation); err != nil {
			return err
		}
		return encodeCleanupJSON(out, map[string]any{"ok": true, "purged": *id})
	case "plan", "remove":
		pool, err := scanner.ScanPool(nil)
		if err != nil {
			return err
		}
		item, found := findCleanupItem(pool, *id)
		if !found {
			return fmt.Errorf("installed tool %q was not discovered", *id)
		}
		if command == "plan" {
			plan, err := service.Plan(item)
			if err != nil {
				return err
			}
			return encodeCleanupJSON(out, plan)
		}
		if *fingerprint == "" || *confirmation == "" {
			return errors.New("removal requires --fingerprint from 'ash cleanup plan' and --confirm with exact tool name")
		}
		record, err := service.Remove(item, *fingerprint, *confirmation)
		if err != nil {
			return err
		}
		return encodeCleanupJSON(out, record)
	default:
		return errCleanupUsage
	}
}

func findCleanupItem(pool *types.Pool, id string) (types.Entity, bool) {
	for _, collection := range []map[string]types.Entity{pool.Skills, pool.MCPs, pool.CLIs, pool.Agents} {
		for _, item := range collection {
			if item.ID == id {
				return item, true
			}
		}
	}
	return types.Entity{}, false
}

func encodeCleanupJSON(writer io.Writer, data any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
