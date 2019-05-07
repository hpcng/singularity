// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sylabs/singularity/internal/pkg/client/cache"
	"github.com/sylabs/singularity/internal/pkg/sylog"
)

func cleanLibraryCache() error {
	c, err := cache.Create()
	if c == nil || err != nil {
		return fmt.Errorf("unable to create the cache object")
	}

	sylog.Debugf("Removing: %v", c.Library)

	err = os.RemoveAll(c.Library)
	if err != nil {
		return fmt.Errorf("unable to clean library cache: %v", err)
	}

	return nil
}

func cleanOciCache() error {
	c, err := cache.Create()
	if c == nil || err != nil {
		return fmt.Errorf("unable to create the cache object")
	}

	sylog.Debugf("Removing: %v", c.OciTemp)

	err = os.RemoveAll(c.OciTemp)
	if err != nil {
		return fmt.Errorf("unable to clean oci-tmp cache: %v", err)
	}

	return nil
}

func cleanBlobCache() error {
	c, err := cache.Create()
	if c == nil || err != nil {
		return fmt.Errorf("unable to create the cache object")
	}

	sylog.Debugf("Removing: %v", c.OciTemp)

	err = os.RemoveAll(c.OciTemp)
	if err != nil {
		return fmt.Errorf("unable to clean oci-blob cache: %v", err)
	}

	return nil

}

// CleanCache : clean a type of cache (cacheType string). will return a error if one occurs.
func CleanCache(cacheType string) error {
	c, err := cache.Create()
	if c == nil || err != nil {
		return fmt.Errorf("unable to create the cache object")
	}

	switch cacheType {
	case "library":
		err := cleanLibraryCache()
		return err
	case "oci":
		err := cleanOciCache()
		return err
	case "blob", "blobs":
		err := cleanBlobCache()
		return err
	case "all":
		err := c.Clean()
		return err
	default:
		// The caller checks the returned error and will exit as required
		return fmt.Errorf("not a valid type: %s", cacheType)
	}
}

func cleanLibraryCacheName(cacheName string) (bool, error) {
	foundMatch := false
	sCache, err := cache.Create()
	if sCache == nil || err != nil {
		return false, fmt.Errorf("unable to create the cache object")
	}

	libraryCacheFiles, err := ioutil.ReadDir(sCache.Library)
	if err != nil {
		return false, fmt.Errorf("unable to opening library cache folder: %v", err)
	}
	for _, f := range libraryCacheFiles {
		cont, err := ioutil.ReadDir(filepath.Join(sCache.Library, f.Name()))
		if err != nil {
			return false, fmt.Errorf("unable to look in library cache folder: %v", err)
		}
		for _, c := range cont {
			if c.Name() == cacheName {
				sylog.Debugf("Removing: %v", filepath.Join(sCache.Library, f.Name(), c.Name()))
				err = os.RemoveAll(filepath.Join(sCache.Library, f.Name(), c.Name()))
				if err != nil {
					return false, fmt.Errorf("unable to remove library cache: %v", err)
				}
				foundMatch = true
			}
		}
	}
	return foundMatch, nil
}

func cleanOciCacheName(cacheName string) (bool, error) {
	foundMatch := false

	c, err := cache.Create()
	if c == nil || err != nil {
		return false, fmt.Errorf("unable to create the cache object")
	}

	blobs, err := ioutil.ReadDir(c.OciTemp)
	if err != nil {
		return false, fmt.Errorf("unable to opening oci-tmp cache folder: %v", err)
	}
	for _, f := range blobs {
		blob, err := ioutil.ReadDir(filepath.Join(c.OciTemp, f.Name()))
		if err != nil {
			return false, fmt.Errorf("unable to look in oci-tmp cache folder: %v", err)
		}
		for _, b := range blob {
			if b.Name() == cacheName {
				sylog.Debugf("Removing: %v", filepath.Join(c.OciTemp, f.Name(), b.Name()))
				err = os.RemoveAll(filepath.Join(c.OciTemp, f.Name(), b.Name()))
				if err != nil {
					return false, fmt.Errorf("unable to remove oci-tmp cache: %v", err)
				}
				foundMatch = true
			}
		}
	}
	return foundMatch, nil
}

// CleanCacheName : will clean a container with the same name as cacheName (in the cache directory).
// if libraryCache is true; only search thrught library cache. if ociCache is true; only search the
// oci-tmp cache. if both are false; search all cache, and if both are true; again, search all cache.
func CleanCacheName(cacheName string, libraryCache, ociCache bool) (bool, error) {
	if libraryCache == ociCache {
		matchLibrary, err := cleanLibraryCacheName(cacheName)
		if err != nil {
			return false, err
		}
		matchOci, err := cleanOciCacheName(cacheName)
		if err != nil {
			return false, err
		}
		if matchLibrary || matchOci {
			return true, nil
		}
		return false, nil
	}

	match := false
	if libraryCache {
		match, err := cleanLibraryCacheName(cacheName)
		if err != nil {
			return false, err
		}
		return match, nil
	} else if ociCache {
		match, err := cleanOciCacheName(cacheName)
		if err != nil {
			return false, err
		}
		return match, nil
	}
	return match, nil
}

// CleanSingularityCache : the main function that drives all these other functions, if allClean is true; clean
// all cache. if typeNameClean contains somthing; only clean that type. if cacheName contains somthing; clean only
// cache with that name.
func CleanSingularityCache(cleanAll bool, cacheCleanTypes []string, cacheName string) error {
	libraryClean := false
	ociClean := false
	blobClean := false

	for _, t := range cacheCleanTypes {
		switch t {
		case "library":
			libraryClean = true
		case "oci":
			ociClean = true
		case "blob", "blobs":
			blobClean = true
		case "all":
			cleanAll = true
		default:
			// The caller checks the returned error and exit when appropriate
			return fmt.Errorf("not a valid type: %s", t)
		}
	}

	if len(cacheName) >= 1 && !cleanAll {
		foundMatch, err := CleanCacheName(cacheName, libraryClean, ociClean)
		if err != nil {
			return err
		}
		if !foundMatch {
			sylog.Warningf("No cache found with given name: %s", cacheName)
		}
		return nil
	}

	if cleanAll {
		if err := CleanCache("all"); err != nil {
			return err
		}
	}

	if libraryClean {
		if err := CleanCache("library"); err != nil {
			return err
		}
	}
	if ociClean {
		if err := CleanCache("oci"); err != nil {
			return err
		}
	}
	if blobClean {
		if err := CleanCache("blob"); err != nil {
			return err
		}
	}
	return nil
}
