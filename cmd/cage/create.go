package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/contrib/seccomp"
	"github.com/containerd/containerd/oci"
	"github.com/mistifyio/go-zfs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/stellarproject/orbit/pkg/image"
	"github.com/urfave/cli"
)

const (
	rwm               = "rwm"
	defaultRootfsPath = "rootfs"
)

var (
	defaultUnixEnv = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
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
		opts := specOpt(config, rootfs, nil, nil)
		spec, err := oci.GenerateSpec(ctx, nil, &containers.Container{
			ID: id,
		}, opts)
		if err != nil {
			return errors.Wrap(err, "generate spec")
		}
		return writeSpec(path, spec)
	},
}

func writeSpec(path string, spec *oci.Spec) error {
	f, err := os.Create(filepath.Join(path, "config.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(spec)
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

func specOpt(i *image.Image, rootfs string, args, env []string) oci.SpecOpts {
	opts := []oci.SpecOpts{
		oci.WithRootFSPath(rootfs),
		withImageConfigArgs(i, args),
		oci.WithHostLocaltime,
		oci.WithEnv(env),
		//withMounts(container.Mounts),
		//withConfigs(paths, container.Configs),
		//oci.WithHostname(container.ID),
		seccomp.WithDefaultProfile(),
		oci.WithNoNewPrivileges,
	}

	/*
		opts = append(opts, oci.WithHostResolvconf, WithContainerHostsFile(paths.State), oci.WithLinuxNamespace(specs.LinuxNamespace{
			Type: specs.NetworkNamespace,
			Path: paths.NetworkPath(container.ID),
		}),
		)
		if container.Resources != nil {
			opts = append(opts, withResources(container.Resources))
		}
	*/
	return oci.Compose(opts...)
}

func withImageConfigArgs(i *image.Image, args []string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
		config := i.Config
		setProcess(s)
		defaults := config.Env
		if len(defaults) == 0 {
			defaults = defaultUnixEnv
		}
		s.Process.Env = replaceOrAppendEnvValues(defaults, s.Process.Env)
		cmd := config.Cmd
		if len(args) > 0 {
			cmd = args
		}
		s.Process.Args = append(config.Entrypoint, cmd...)

		cwd := config.WorkingDir
		if cwd == "" {
			cwd = "/"
		}
		s.Process.Cwd = cwd
		if config.User != "" {
			if err := oci.WithUser(config.User)(ctx, client, c, s); err != nil {
				return err
			}
			return oci.WithAdditionalGIDs(fmt.Sprintf("%d", s.Process.User.UID))(ctx, client, c, s)
		}
		// we should query the image's /etc/group for additional GIDs
		// even if there is no specified user in the image config
		return oci.WithAdditionalGIDs("root")(ctx, client, c, s)
	}
}

func name() {

}

// setProcess sets Process to empty if unset
func setProcess(s *oci.Spec) {
	if s.Process == nil {
		s.Process = &specs.Process{}
	}
}

func WithContainerHostsFile(root string) oci.SpecOpts {
	return func(ctx context.Context, _ oci.Client, c *containers.Container, s *oci.Spec) error {
		hosts, hostname, err := WriteHostsFiles(root, c.ID)
		if err != nil {
			return err
		}
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      hosts,
			Options:     []string{"rbind", "ro"},
		})
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      hostname,
			Options:     []string{"rbind", "ro"},
		})
		return nil
	}
}

func WriteHostsFiles(root, id string) (string, string, error) {
	if err := os.MkdirAll(root, 0711); err != nil {
		return "", "", err
	}
	path := filepath.Join(root, "hosts")
	f, err := os.Create(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	if err := f.Chmod(0666); err != nil {
		return "", "", err
	}
	if _, err := f.WriteString("127.0.0.1       localhost\n"); err != nil {
		return "", "", err
	}
	if _, err := f.WriteString(fmt.Sprintf("127.0.0.1       %s\n", id)); err != nil {
		return "", "", err
	}
	if _, err := f.WriteString("::1     localhost ip6-localhost ip6-loopback\n"); err != nil {
		return "", "", err
	}
	hpath := filepath.Join(root, "hostname")
	hf, err := os.Create(hpath)
	if err != nil {
		return "", "", err
	}
	if _, err := hf.WriteString(id); err != nil {
		return "", "", err
	}
	return path, hpath, nil
}

// replaceOrAppendEnvValues returns the defaults with the overrides either
// replaced by env key or appended to the list
func replaceOrAppendEnvValues(defaults, overrides []string) []string {
	cache := make(map[string]int, len(defaults))
	results := make([]string, 0, len(defaults))
	for i, e := range defaults {
		parts := strings.SplitN(e, "=", 2)
		results = append(results, e)
		cache[parts[0]] = i
	}

	for _, value := range overrides {
		// Values w/o = means they want this env to be removed/unset.
		if !strings.Contains(value, "=") {
			if i, exists := cache[value]; exists {
				results[i] = "" // Used to indicate it should be removed
			}
			continue
		}

		// Just do a normal set/update
		parts := strings.SplitN(value, "=", 2)
		if i, exists := cache[parts[0]]; exists {
			results[i] = value
		} else {
			results = append(results, value)
		}
	}

	// Now remove all entries that we want to "unset"
	for i := 0; i < len(results); i++ {
		if results[i] == "" {
			results = append(results[:i], results[i+1:]...)
			i--
		}
	}

	return results
}
