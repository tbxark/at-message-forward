package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tbxark/at-message-forward/internal/config"
	"github.com/tbxark/at-message-forward/internal/forwarder"
	"github.com/tbxark/at-message-forward/internal/transport/serial"
	"github.com/tbxark/at-message-forward/internal/transport/usb"
)

var BuildVersion = "dev"

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRootCommand()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "atmsgfwd",
		Short: "Read SMS messages from an AT-capable modem and forward them",
	}

	root.AddCommand(newForwardCommand())
	root.AddCommand(newPortsCommand())
	root.AddCommand(newVersionCommand())
	return root
}

func newForwardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "forward [config]",
		Short: "Listen for SMS messages and forward them",
		Long:  "Listen to an AT-capable modem's serial port, parse SMS modem indications, and forward messages through configured notification channels. If config is omitted, config.json is used.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := config.DefaultPath
			if len(args) > 0 {
				configPath = args[0]
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return forwarder.Run(cmd.Context(), cfg)
		},
	}
}

func newPortsCommand() *cobra.Command {
	cfg := config.Default()
	baud := cfg.Baud
	probe := true
	probeTimeout := serial.DefaultProbeTimeout
	cmd := &cobra.Command{
		Use:   "ports",
		Short: "List serial port candidates",
		Long:  "List detected serial ports, probe candidates with a bare AT command, and mark the port auto-detect would choose.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if probe {
				if baud <= 0 {
					return fmt.Errorf("invalid baud %d", baud)
				}
				if probeTimeout <= 0 {
					return fmt.Errorf("invalid probe timeout %s", probeTimeout)
				}
				serial.PrintProbedCandidates(baud, probeTimeout)
			} else {
				serial.PrintCandidates()
			}
			printUSBCandidates(cmd, probeTimeout)
			return nil
		},
	}
	cmd.Flags().IntVar(&baud, "baud", baud, "baud rate used for AT probes")
	cmd.Flags().BoolVar(&probe, "probe", probe, "probe each candidate with a bare AT command")
	cmd.Flags().DurationVar(&probeTimeout, "probe-timeout", probeTimeout, "timeout per AT probe")
	return cmd
}

func printUSBCandidates(cmd *cobra.Command, probeTimeout time.Duration) {
	out := cmd.OutOrStdout()
	candidates, err := usb.List(probeTimeout)
	if err != nil {
		if errors.Is(err, usb.ErrNotSupported) {
			fmt.Fprintln(out, "usb transport: not built in (rebuild with `-tags usb` and CGO_ENABLED=1 to talk to modems that expose no serial port, e.g. on macOS)")
			return
		}
		fmt.Fprintf(out, "usb transport: enumeration failed: %v\n", err)
		return
	}
	if len(candidates) == 0 {
		fmt.Fprintln(out, "usb transport: no candidate modems found")
		return
	}
	for _, c := range candidates {
		status := "no AT interface"
		if c.ProbeOK {
			status = fmt.Sprintf("AT ok on if%d", c.Interface)
		} else if c.ProbeError != "" {
			status = c.ProbeError
		}
		fmt.Fprintf(out, "usb candidate %04x:%04x %q %q -> %s\n",
			c.VID, c.PID, strings.TrimSpace(c.Manufacturer), strings.TrimSpace(c.ProductName), status)
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), BuildVersion)
			return err
		},
	}
}
