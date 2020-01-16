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
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stellarproject/orbit/pkg/image"
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

func specOpt(i *image.Image, id, path, rootfs string, args, env []string) oci.SpecOpts {
	opts := []oci.SpecOpts{
		oci.WithRootFSPath(rootfs),
		oci.WithHostname(id),
		withImageConfigArgs(i, args),
		oci.WithHostLocaltime,
		oci.WithEnv(env),
		seccomp.WithDefaultProfile(),
		oci.WithNoNewPrivileges,
		oci.WithHostResolvconf,
		WithContainerHostsFile(path),
	}

	/*
		opts = append(opts,  oci.WithLinuxNamespace(specs.LinuxNamespace{
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
