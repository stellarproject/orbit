package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/mistifyio/go-zfs"
	"github.com/pkg/errors"
	"github.com/stellarproject/orbit/pkg/image"
	"github.com/urfave/cli"
)

var createCommand = cli.Command{
	Name:  "create",
	Usage: "create a new container",
	Flags: []cli.Flag{
		cli.StringSliceFlag{
			Name:  "net",
			Usage: "set network",
		},
		cli.BoolFlag{
			Name:  "http",
			Usage: "pull over http",
		},
	},
	Action: func(clix *cli.Context) error {
		var (
			ref         = clix.Args().First()
			id          = clix.Args().Get(1)
			ctx         = cancelContext()
			path        = filepath.Join(root, id)
			rootfs      = filepath.Join(path, "rootfs")
			datasetName = fmt.Sprintf("%s/cage/%s", clix.GlobalString("dataset"), id)
		)
		store, err := tmpContentStore()
		if err != nil {
			return err
		}
		desc, err := image.Fetch(ctx, clix.Bool("http"), store, ref)
		if err != nil {
			return errors.Wrap(err, "fetch image")
		}
		if _, err := zfs.CreateFilesystem(fmt.Sprintf("%s/cage", clix.GlobalString("dataset")), map[string]string{}); err != nil {
			if !isAlreadyExists(err) {
				return errors.Wrap(err, "create cage dataset")
			}
		}
		dataset, err := zfs.CreateFilesystem(datasetName, map[string]string{
			"mountpoint": path,
		})
		if err != nil {
			return err
		}
		dataset = dataset

		if err := os.MkdirAll(rootfs, 0711); err != nil {
			return err
		}
		if err := image.Unpack(ctx, store, desc, rootfs); err != nil {
			return errors.Wrap(err, "unpack image")
		}
		config, err := image.GetConfig(ctx, store, *desc)
		if err != nil {
			return errors.Wrap(err, "get config")
		}
		opts := specOpt(config, id, path, rootfs, nil, nil)
		spec, err := oci.GenerateSpec(ctx, nil, &containers.Container{
			ID: id,
		}, opts)
		if err != nil {
			return errors.Wrap(err, "generate spec")
		}
		return writeSpec(path, spec)
	},
}
