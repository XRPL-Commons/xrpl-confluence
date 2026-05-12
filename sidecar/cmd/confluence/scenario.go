package main

import (
	"encoding/json"
	"fmt"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
	"github.com/spf13/cobra"
)

func newScenarioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Manage Scenario files",
	}
	cmd.AddCommand(newScenarioValidateCmd())
	return cmd
}

func newScenarioValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate PATH",
		Short: "Validate a Scenario YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := scenario.Load(args[0])
			if err != nil {
				return outputValidation(cmd, false, []api.Error{{
					Code:    "scenario_unreadable",
					Message: err.Error(),
				}})
			}
			errs := scenario.Validate(s)
			ok := len(errs) == 0
			return outputValidation(cmd, ok, errs)
		},
	}
}

func outputValidation(cmd *cobra.Command, ok bool, errs []api.Error) error {
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("scenario validate: --json flag: %w", err)
	}
	if errs == nil {
		errs = []api.Error{}
	}

	if asJSON {
		payload := struct {
			OK     bool        `json:"ok"`
			Errors []api.Error `json:"errors"`
		}{ok, errs}
		if jerr := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); jerr != nil {
			return jerr
		}
	} else if ok {
		fmt.Fprintln(cmd.OutOrStdout(), "ok")
	} else {
		for _, e := range errs {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", e.Field, e.Message)
		}
	}

	if !ok {
		// Returning an error makes cobra propagate a non-zero exit; we suppress
		// printing it (SilenceErrors on root) so JSON consumers see only the JSON
		// payload on stdout.
		return fmt.Errorf("scenario invalid")
	}
	return nil
}
