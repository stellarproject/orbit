/*
	Copyright (c) 2019 Stellar Project

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package image

import (
	"archive/tar"
	"context"
	"encoding/json"
	"os"

	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/content/local"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/rootfs"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func NewContentStore(root string) (content.Store, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return local.NewStore(root)
}

func Fetch(ctx context.Context, http bool, cs content.Store, imageName string) (*v1.Descriptor, error) {
	authorizer := docker.NewAuthorizer(nil, getDockerCredentials)
	resolver := docker.NewResolver(docker.ResolverOptions{
		PlainHTTP:  http,
		Authorizer: authorizer,
	})
	name, desc, err := resolver.Resolve(ctx, imageName)
	if err != nil {
		return nil, err
	}
	fetcher, err := resolver.Fetcher(ctx, name)
	if err != nil {
		return nil, err
	}
	logrus.Infof("fetching image %s", imageName)
	childrenHandler := images.ChildrenHandler(cs)
	h := images.Handlers(remotes.FetchHandler(cs, fetcher), childrenHandler)
	if err := images.Dispatch(ctx, h, nil, desc); err != nil {
		return nil, err
	}
	return &desc, nil
}

func Unpack(ctx context.Context, cs content.Store, desc *v1.Descriptor, dest string) error {
	_, layers, err := getLayers(ctx, cs, *desc)
	if err != nil {
		return err
	}
	logrus.Infof("unpacking image to %q", dest)
	for _, layer := range layers {
		if err := extract(ctx, cs, layer, dest); err != nil {
			return err
		}
	}
	return nil
}

func extract(ctx context.Context, cs content.Store, layer rootfs.Layer, dest string) error {
	ra, err := cs.ReaderAt(ctx, layer.Blob)
	if err != nil {
		return err
	}
	defer ra.Close()

	cr := content.NewReader(ra)
	r, err := compression.DecompressStream(cr)
	if err != nil {
		return err
	}
	defer r.Close()

	if r.(compression.DecompressReadCloser).GetCompression() == compression.Uncompressed {
		return nil
	}
	logrus.WithField("layer", layer.Blob.Digest).Info("apply layer")
	if _, err := archive.Apply(ctx, dest, r, archive.WithFilter(HostFilter)); err != nil {
		return err
	}
	return nil
}

const excludedModes = os.ModeDevice | os.ModeCharDevice | os.ModeSocket | os.ModeNamedPipe

func HostFilter(h *tar.Header) (bool, error) {
	// exclude devices
	if h.FileInfo().Mode()&excludedModes != 0 {
		return false, nil
	}
	return true, nil
}

func getConfig(ctx context.Context, provider content.Provider, desc v1.Descriptor) (*Image, error) {
	p, err := content.ReadBlob(ctx, provider, desc)
	if err != nil {
		return nil, err
	}
	var config Image
	if err := json.Unmarshal(p, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func GetConfig(ctx context.Context, cs content.Store, desc v1.Descriptor) (*Image, error) {
	manifest, err := images.Manifest(ctx, cs, desc, nil)
	if err != nil {
		return nil, err
	}
	return getConfig(ctx, cs, manifest.Config)
}

func getLayers(ctx context.Context, cs content.Store, desc v1.Descriptor) (*Image, []rootfs.Layer, error) {
	manifest, err := images.Manifest(ctx, cs, desc, nil)
	if err != nil {
		return nil, nil, err
	}
	config, err := getConfig(ctx, cs, manifest.Config)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to resolve config")
	}
	diffIDs := config.RootFS.DiffIDs
	if len(diffIDs) != len(manifest.Layers) {
		return nil, nil, errors.Errorf("mismatched image rootfs and manifest layers")
	}
	var layers []rootfs.Layer
	for i := range diffIDs {
		var l rootfs.Layer
		l.Diff = v1.Descriptor{
			MediaType: v1.MediaTypeImageLayer,
			Digest:    diffIDs[i],
		}
		l.Blob = manifest.Layers[i]
		layers = append(layers, l)
	}
	return config, layers, nil
}
