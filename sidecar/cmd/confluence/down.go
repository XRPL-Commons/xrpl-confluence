package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/discovery"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return newDownCmdWith(defaultDownDeps())
}

func newDownCmdWith(d *downDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "down [ENCLAVE]",
		Short: "Tear down the current confluence enclave",
		Args:  cobra.MaximumNArgs(1),
		RunE:  d.run,
	}
}

type downDeps struct {
	cli kurtosis.CLI
}

func defaultDownDeps() *downDeps {
	return &downDeps{cli: kurtosis.NewExec()}
}

func (d *downDeps) run(cmd *cobra.Command, args []string) error {
	enclaveName := ""
	if len(args) > 0 {
		enclaveName = args[0]
	}

	var cur *discovery.Current
	if enclaveName == "" {
		var err error
		cur, err = discovery.Read()
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if cur != nil {
			enclaveName = cur.EnclaveID
		}
	}

	if enclaveName == "" {
		return fmt.Errorf("no current enclave (run 'confluence up' first)")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if err := kurtosis.RemoveEnclave(ctx, d.cli, enclaveName); err != nil {
		return err
	}

	if cur == nil {
		// Positional arg was given — check if it matches the discovery file.
		c, err := discovery.Read()
		if err == nil && c != nil && c.EnclaveID == enclaveName {
			cur = c
		}
	}
	if cur != nil {
		if err := discovery.Remove(); err != nil {
			return err
		}
	}

	return emitDown(cmd, enclaveName)
}

func emitDown(cmd *cobra.Command, enclaveName string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		payload := struct {
			EnclaveID string `json:"enclave_id"`
			OK        bool   `json:"ok"`
		}{EnclaveID: enclaveName, OK: true}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed enclave %q\n", enclaveName)
	return nil
}
