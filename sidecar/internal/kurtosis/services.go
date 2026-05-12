package kurtosis

import (
	"context"
	"fmt"
	"io"
)

// ServiceLogs streams logs for a service to w.
// When follow is true, --follow is passed to keep the stream open until ctx
// is cancelled.
func ServiceLogs(ctx context.Context, cli CLI, enclave, service string, follow bool, w io.Writer) error {
	args := []string{"service", "logs"}
	if follow {
		args = append(args, "--follow")
	}
	args = append(args, enclave, service)
	if err := cli.Run(ctx, args, nil, w, w); err != nil {
		return fmt.Errorf("kurtosis service logs: %w", err)
	}
	return nil
}
