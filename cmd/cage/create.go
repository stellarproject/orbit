package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/oci"
	"github.com/mistifyio/go-zfs"
	"github.com/pkg/errors"
	v1 "github.com/stellarproject/orbit/api/orbit/v1"
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
		return nil
	},
}

func containerSpec(rootfs string, config *v1.Container, image containerd.Image) (*oci.Spec, error) {
	/*
		paths := opts.Paths{
			State: "/run/cage",
		}
	*/
	return nil, nil
}

func tmpContentStore() (content.Store, error) {
	if err := os.MkdirAll(contentStorePath, 0711); err != nil {
		return nil, err
	}
	s, err := image.NewContentStore(contentStorePath)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "dataset already exists")
}
