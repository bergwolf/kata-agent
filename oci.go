//
// Copyright (c) 2018 NVIDIA CORPORATION
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// OCI config file
const (
	ociConfigFile     string      = "config.json"
	ociConfigFileMode os.FileMode = 0444
	ociConfigBasePath string      = "/run/libcontainer"
)

// writeSpecToFile writes the container's OCI spec to "/run/libcontainer/<container-id>/config.json"
// Note that the OCI bundle (rootfs) is at a different path
func writeSpecToFile(spec *specs.Spec, containerId string) error {
	configJsonDir := filepath.Join(ociConfigBasePath, containerId)
	err := os.MkdirAll(configJsonDir, 0700)
	if err != nil {
		return err
	}
	configPath := filepath.Join(configJsonDir, ociConfigFile)
	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE, ociConfigFileMode)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(spec)
}

// changeToBundlePath changes the cwd to the OCI bundle path defined as
// dirname(spec.Root.Path) and returns the old cwd.
func changeToBundlePath(spec *specs.Spec, containerId string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return cwd, err
	}

	if spec == nil || spec.Root == nil || spec.Root.Path == "" {
		return cwd, errors.New("invalid OCI spec")
	}

	bundlePath := filepath.Dir(spec.Root.Path)
	configPath := filepath.Join(ociConfigBasePath, containerId, ociConfigFile)

	// config.json is at "/run/libcontainer/<container-id>/"
	// Actual bundle (rootfs) is at dirname(spec.Root.Path)
	if _, err := os.Stat(configPath); err != nil {
		return cwd, errors.New("invalid OCI bundle")
	}

	return cwd, os.Chdir(bundlePath)
}

func isValidHook(file os.FileInfo) (bool, error) {
	if file.IsDir() {
		return false, errors.New("is a directory")
	}

	mode := file.Mode()
	if (mode & os.ModeSymlink) != 0 {
		return false, errors.New("is a symbolic link")
	}

	perm := mode & os.ModePerm
	if (perm & 0111) == 0 {
		return false, errors.New("is not executable")
	}

	return true, nil
}

// findHooks searches guestHookPath for any OCI hooks for a given hookType
func findHooks(guestHookPath, hookType string) (hooksFound []specs.Hook) {
	hooksPath := path.Join(guestHookPath, hookType)

	files, err := ioutil.ReadDir(hooksPath)
	if err != nil {
		agentLog.WithError(err).WithField("oci-hook-type", hookType).Info("Skipping hook type")
		return
	}

	for _, file := range files {
		name := file.Name()
		if ok, err := isValidHook(file); !ok {
			agentLog.WithError(err).WithField("oci-hook-name", name).Warn("Skipping hook")
			continue
		}

		agentLog.WithFields(logrus.Fields{
			"oci-hook-name": name,
			"oci-hook-type": hookType,
		}).Info("Adding hook")
		hooksFound = append(hooksFound, specs.Hook{
			Path: path.Join(hooksPath, name),
			Args: []string{name, hookType},
		})
	}

	agentLog.WithField("oci-hook-type", hookType).Infof("Added %d hooks", len(hooksFound))

	return
}
