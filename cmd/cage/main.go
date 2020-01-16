package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/containerd/containerd/namespaces"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	root             = "/var/lib/cage"
	contentStorePath = "/var/lib/cage/content"
)

func main() {
	app := cli.NewApp()
	app.Name = "cage"
	app.Version = "1"
	app.Usage = "like jails but for linux"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "enable debug output in the logs",
			EnvVar: "DEBUG",
		},
		cli.StringFlag{
			Name:   "dataset",
			Usage:  "zfs dataset",
			Value:  "tank",
			EnvVar: "CAGE_DATASET",
		},
	}
	app.Commands = []cli.Command{
		createCommand,
	}
	app.Before = func(clix *cli.Context) error {
		if clix.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		if err := os.MkdirAll(root, 0711); err != nil {
			return err
		}
		return nil
	}
	/*
		app.Action = func(clix *cli.Context) error {
			return nil
		}
	*/
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cancelContext() context.Context {
	ctx, cancel := context.WithCancel(namespaces.WithNamespace(context.Background(), "cage"))
	s := make(chan os.Signal)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-s
		cancel()
	}()
	return ctx
}
