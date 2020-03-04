// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package shub

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/sylabs/singularity/internal/pkg/client/cache"

	jsonresp "github.com/sylabs/json-resp"
	"github.com/sylabs/singularity/internal/pkg/sylog"
	"github.com/sylabs/singularity/internal/pkg/util/fs"
	useragent "github.com/sylabs/singularity/pkg/util/user-agent"
	"github.com/vbauerster/mpb/v4"
	"github.com/vbauerster/mpb/v4/decor"
)

// Timeout for an image pull in seconds (2 hours)
const pullTimeout = 7200

// DownloadImage image will download a shub image to a path. This will not try
// to cache it, or use cache.
func DownloadImage(manifest ShubAPIResponse, filePath, shubRef string, force, noHTTPS bool) error {
	sylog.Debugf("Downloading container from Shub")
	if !force {
		if _, err := os.Stat(filePath); err == nil {
			return fmt.Errorf("image file already exists: %q - will not overwrite", filePath)
		}
	}

	// use custom parser to make sure we have a valid shub URI
	if ok := isShubPullRef(shubRef); !ok {
		sylog.Fatalf("Invalid shub URI")
	}

	shubURI, err := ShubParseReference(shubRef)
	if err != nil {
		return fmt.Errorf("failed to parse shub uri: %v", err)
	}

	if filePath == "" {
		filePath = fmt.Sprintf("%s_%s.simg", shubURI.container, shubURI.tag)
		sylog.Infof("Download filename not provided. Downloading to: %s\n", filePath)
	}

	// Get the image based on the manifest
	httpc := http.Client{
		Timeout: pullTimeout * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, manifest.Image, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", useragent.Value())

	if noHTTPS {
		req.URL.Scheme = "http"
	}

	// Do the request, if status isn't success, return error
	resp, err := httpc.Do(req)
	if resp == nil {
		return fmt.Errorf("no response received from singularity hub")
	}
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("the requested image was not found in singularity hub")
	}
	sylog.Debugf("%s response received, beginning image download\n", resp.Status)

	if resp.StatusCode != http.StatusOK {
		err := jsonresp.ReadError(resp.Body)
		if err != nil {
			return fmt.Errorf("download did not succeed: %s", err.Error())
		}
		return fmt.Errorf("download did not succeed: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	// Perms are 777 *prior* to umask
	out, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}
	defer out.Close()

	sylog.Debugf("Created output file: %s\n", filePath)

	bodySize := resp.ContentLength
	p := mpb.New()
	bar := p.AddBar(bodySize,
		mpb.PrependDecorators(
			decor.Counters(decor.UnitKiB, "%.1f / %.1f"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
			decor.AverageSpeed(decor.UnitKiB, " % .1f "),
			decor.AverageETA(decor.ET_STYLE_GO),
		),
	)

	// create proxy reader
	bodyProgress := bar.ProxyReader(resp.Body)

	// Write the body to file
	bytesWritten, err := io.Copy(out, bodyProgress)
	if err != nil {
		return err
	}

	// Simple check to make sure image received is the correct size
	if resp.ContentLength == -1 {
		sylog.Warningf("unknown image length")
	} else if bytesWritten != resp.ContentLength {
		return fmt.Errorf("image received is not the right size. supposed to be: %v actually: %v", resp.ContentLength, bytesWritten)
	}

	sylog.Debugf("Download complete: %s\n", filePath)

	return nil
}

// Pull will download a image from shub, and cache it if caching is enabled. Next time
// that container is downloaded this will just use that cached image.
func Pull(imgCache *cache.Handle, pullFrom string, tmpDir string, noHTTPS bool) (imagePath string, err error) {
	shubURI, err := ShubParseReference(pullFrom)
	if err != nil {
		return "", fmt.Errorf("failed to parse shub uri: %s", err)
	}

	// Get the image manifest
	manifest, err := GetManifest(shubURI, noHTTPS)
	if err != nil {
		return "", fmt.Errorf("failed to get manifest for: %s: %s", pullFrom, err)
	}

	if imgCache.IsDisabled() {
		file, err := ioutil.TempFile(tmpDir, "sbuild-tmp-cache-")
		if err != nil {
			return "", fmt.Errorf("unable to create tmp file: %v", err)
		}
		imagePath = file.Name()
		sylog.Infof("Downloading shub image to tmp cache: %s", imagePath)
		// Dont use cached image
		if err := DownloadImage(manifest, imagePath, pullFrom, true, noHTTPS); err != nil {
			return "", err
		}
	} else {
		cacheEntry, err := imgCache.GetEntry(cache.ShubCacheType, manifest.Commit)
		if err != nil {
			return "", fmt.Errorf("unable to check if %v exists in cache: %v", manifest.Commit, err)
		}
		if !cacheEntry.Exists {
			sylog.Infof("Downloading shub image")

			err := DownloadImage(manifest, cacheEntry.TmpPath, pullFrom, true, noHTTPS)
			if err != nil {
				return "", err
			}

			err = cacheEntry.Finalize()
			if err != nil {
				return "", err
			}
			imagePath = cacheEntry.Path
		} else {
			sylog.Infof("Use cached image")
			imagePath = cacheEntry.Path
		}

	}

	return imagePath, nil
}

// PullToFile will build a SIF image from the specified oci URI and place it at the specified dest
func PullToFile(imgCache *cache.Handle, pullTo, pullFrom, tmpDir string, noHTTPS bool) (imagePath string, err error) {

	src, err := Pull(imgCache, pullFrom, tmpDir, noHTTPS)
	if err != nil {
		return "", fmt.Errorf("error fetching image to cache: %v", err)
	}

	err = fs.CopyFile(src, pullTo, 0755)
	if err != nil {
		return "", fmt.Errorf("error fetching image to cache: %v", err)
	}

	return pullTo, nil
}
