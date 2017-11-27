package service

import (
	"context"
	"path"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/thecodeteam/gofsutil"
)

func (s *service) GetVolumeID(
	ctx context.Context,
	name string) (string, error) {

	volPath := path.Join(s.vol, name)
	if !fileExists(volPath) {
		return "", nil
	}
	return name, nil
}

func (s *service) GetVolumeInfo(
	ctx context.Context,
	id, name string) (*csi.VolumeInfo, error) {

	if id == "" {
		id = name
	}

	volPath := path.Join(s.vol, id)
	if !fileExists(volPath) {
		return nil, nil
	}

	return &csi.VolumeInfo{Id: id}, nil
}

func (s *service) IsControllerPublished(
	ctx context.Context,
	id, nodeID string) (map[string]string, error) {

	volPath := path.Join(s.vol, id)
	devPath := path.Join(s.dev, id)
	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		return nil, err
	}

	for _, mi := range minfo {
		if mi.Source == volPath && mi.Path == devPath {
			return map[string]string{"path": devPath}, nil
		}
	}

	return nil, nil
}

func (s *service) IsNodePublished(
	ctx context.Context,
	id string,
	pubInfo map[string]string,
	targetPath string) (bool, error) {

	mntPath := path.Join(s.mnt, id)

	minfo, err := gofsutil.GetMounts(ctx)
	if err != nil {
		return false, err
	}

	for _, mi := range minfo {
		if mi.Source == mntPath && mi.Path == targetPath {
			return true, nil
		}
	}

	return false, nil
}
