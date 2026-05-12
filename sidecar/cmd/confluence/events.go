package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/client"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/kurtosis"
	"github.com/spf13/cobra"
)

func newEventsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "events",
		Short: "Stream SSE events from the control service as NDJSON",
		Long:  "Subscribe to the /v1/events SSE stream and emit each event as a single NDJSON line on stdout. Pipe through jq for filtering.",
		RunE:  runEvents,
	}
}

func runEvents(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	controlURL, err := resolveControlURL(ctx, cmd, kurtosis.NewExec())
	if err != nil {
		return err
	}

	c := client.New(controlURL, client.WithHTTPClient(&http.Client{}))

	body, err := c.Events(ctx)
	if err != nil {
		return fmt.Errorf("events: %w", err)
	}
	defer body.Close()

	done := make(chan error, 1)
	go func() {
		done <- streamSSEAsNDJSON(body, cmd.OutOrStdout())
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}

// streamSSEAsNDJSON reads an SSE stream from r and writes each data line as a
// NDJSON line to w. Comment lines (starting with ':') and blank separators are
// silently skipped.
func streamSSEAsNDJSON(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if _, err := fmt.Fprintln(w, payload); err != nil {
			return err
		}
	}
	return scanner.Err()
}
