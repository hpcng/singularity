// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sylabs/singularity/internal/pkg/sylog"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
)

const (
	// NetDir is the directory inside the cache.Dir where net images are cached
	NetDir = "net"
)

// Net returns the directory inside the cache.Dir() where shub images are
// cached
func getNetCachePath(c *SingularityCache) (string, error) {
	// This function may act on an cache object that is not fully initialized
	// so it is not a method on a SingularityCache but rather an independent
	// function

	return updateCacheSubdir(c, NetDir)
}

// NetImage creates a directory inside cache.Dir() with the name of the SHA sum
// of the image.
// sum and path must not be empty strings since it would create name collisions.
func (c *SingularityCache) NetImage(sum, name string) (string, error) {
	if !c.isValid() {
		return "", fmt.Errorf("invalid cache")
	}

	// name and sum cannot be empty strings otherwise we have name collision
	// between images and the cache directory itself
	if sum == "" || name == "" {
		return "", fmt.Errorf("invalid arguments")
	}

	return filepath.Join(c.Net, sum, name), nil
}

// NetImageExists returns whether the image with the SHA sum exists in the net
// cache.
func (c *SingularityCache) NetImageExists(sum, name string) (bool, error) {
	if !c.isValid() {
		return false, fmt.Errorf("invalid cache")
	}

	path, err := c.NetImage(sum, name)
	if err != nil {
		return false, fmt.Errorf("failed to get image's data: %s", err)
	}

	exists, err := fs.Exists(path)
	if !exists || err != nil {
		return false, err
	}

	if !checkImageHash(path, sum) {
		return false, fmt.Errorf("invalid image sum: %s", sum)
	}

	return true, nil
}

// cleanNetCache deletes the cache's sub-directory used for the Net cache.
func (c *SingularityCache) cleanNetCache() error {
	if !c.isValid() {
		return fmt.Errorf("invalid cache")
	}

	sylog.Debugf("Removing: %v", c.Net)

	err := os.RemoveAll(c.Net)
	if err != nil {
		return fmt.Errorf("unable to clean library cache: %v", err)
	}

	return nil
}
