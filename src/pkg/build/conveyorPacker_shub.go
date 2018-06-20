// Copyright (c) 2018, Sylabs Inc. All rights reserved.
// Copyright (c) 2018, Vanessa Sochat. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package build

/*
#include <unistd.h>
#include "image/image.h"
#include "util/config_parser.h"
*/
// #cgo CFLAGS: -I../../runtime/c/lib
// #cgo LDFLAGS: -L../../../builddir/lib -lruntime -luuid
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/image/docker"
	oci "github.com/containers/image/oci/layout"
	"github.com/containers/image/types"
	"github.com/singularityware/singularity/src/pkg/sylog"
	"github.com/singularityware/singularity/src/pkg/util/loop"
	args "github.com/singularityware/singularity/src/runtime/engines/singularity/rpc"
)

type shubAPIResponse struct {
	Image   string `json:"image"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Version string `json:"version"`
}

// ShubClient contains the uri and client, lowercase means not exported
type shubClient struct {
	Client   http.Client
	ImageURI string
	HTTPAddr string
}

// NewshubClient creates a RemoteBuilder with the specified details.
func newshubClient(uri string) (sc *shubClient) {

	sylog.Infof("%v\n", uri)
	sc = &shubClient{
		Client: http.Client{
			Timeout: 30 * time.Second,
		},
		HTTPAddr: "www.singularity-hub.org",
		ImageURI: uri,
	}

	return
}

// ShubConveyor holds data to be packed into a bundle.
type ShubConveyor struct {
	recipe   Definition
	src      string
	srcRef   types.ImageReference
	tmpfs    string
	tmpfsRef types.ImageReference
	tmpfile  string
}

// ShubConveyorPacker only needs to hold the conveyor to have the needed data to pack
type ShubConveyorPacker struct {
	ShubConveyor
}

// Get downloads container information from Singularityhub
func (c *ShubConveyor) Get(recipe Definition) (err error) {

	c.recipe = recipe

	//prepending slashes to src for ParseReference expected string format
	src := "//" + recipe.Header["from"]

	// Shub URI largely follows same namespace convention
	c.srcRef, err = docker.ParseReference(src)
	if err != nil {
		return
	}

	c.tmpfs, err = ioutil.TempDir("", "temp-shub-")
	if err != nil {
		return
	}

	c.tmpfsRef, err = oci.ParseReference(c.tmpfs + ":" + "tmp")
	if err != nil {
		return
	}

	// Get the image manifest
	manifest, err := c.getManifest()

	// The full Google Storage download media link
	sylog.Infof("%v\n", manifest.Image)

	// retrieve the image
	c.tmpfile, err = c.fetch(manifest.Image)
	if err != nil {
		log.Fatal(err)
		return
	}

	return nil
}

// Pack puts relevant objects in a Bundle!
func (cp *ShubConveyorPacker) Pack() (b *Bundle, err error) {
	defer os.RemoveAll(cp.tmpfs)

	b, err = NewBundle()

	err = cp.unpackTmpfs(b)
	if err != nil {
		log.Fatal(err)
		return
	}

	b.Recipe = cp.recipe

	return b, nil
}

// Download an image from Singularity Hub, writing as we download instead
// of storing in memory
func (c *ShubConveyor) fetch(url string) (image string, err error) {

	// Create temporary download name
	tmpfile, err := ioutil.TempFile(c.tmpfs, "shub-container")
	sylog.Infof("\nCreating temporary image file %v\n", tmpfile.Name())
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	// Get the image data
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(tmpfile, resp.Body)
	if err != nil {
		return "", err
	}

	return tmpfile.Name(), err
}

// getManifest will return the image manifest for a container uri
// from Singularity Hub. We return the shubAPIResponse and error
func (c *ShubConveyor) getManifest() (manifest *shubAPIResponse, err error) {

	// Create a new Singularity Hub client
	sc := &shubClient{}

	// TODO: need to parse the tag / digest and send along too
	uri := strings.Split(c.srcRef.StringWithinTransport(), ":")[0]

	// Format the http address, coinciding with the image uri
	httpAddr := fmt.Sprintf("www.singularity-hub.org/api/container%s/", uri)
	sylog.Infof("%v\n", httpAddr)

	// Create the request, add headers context
	url := url.URL{
		Scheme: "https",
		Host:   sc.HTTPAddr,
		Path:   httpAddr,
	}

	sylog.Infof("%v\n", url)
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}

	// Do the request, if status isn't success, return error
	res, err := sc.Client.Do(req)
	sylog.Infof("response: %v\n", res)

	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		err = errors.New(res.Status)
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &manifest)
	sylog.Infof("manifest: %v\n", manifest.Image)
	if err != nil {
		return nil, err
	}

	return manifest, err

}

func (cp *ShubConveyorPacker) unpackTmpfs(b *Bundle) (err error) {

	rootfs := cp.tmpfile

	//leverage C code to properly mount squashfs image
	C.singularity_config_init()

	imageObject := C.singularity_image_init(C.CString(rootfs), 0)

	info := new(loop.Info64)

	switch C.singularity_image_type(&imageObject) {
	case 1:
		//squashfs
		info.Offset = uint64(C.uint(imageObject.offset))
		info.SizeLimit = uint64(C.uint(imageObject.size))
		err = cp.unpackSquashfs(b, info, rootfs)
		if err != nil {
			sylog.Fatalf("unpackSquashfs Failed", err.Error())
			return err
		}
	case 2:
		//ext3
		info.Offset = uint64(C.uint(imageObject.offset))
		info.SizeLimit = uint64(C.uint(imageObject.size))
		err = cp.unpackExt3(b, info, rootfs)
		if err != nil {
			sylog.Fatalf("unpackExt3 Failed", err.Error())
			return err
		}
	default:
		sylog.Fatalf("Invalid image format from shub")
	}

	return err
}

func (cp *ShubConveyorPacker) unpackSquashfs(b *Bundle, info *loop.Info64, rootfs string) (err error) {
	trimfile, err := ioutil.TempFile(cp.tmpfs, "trim.squashfs")
	//trim header
	cmd := exec.Command("dd", "bs="+strconv.Itoa(int(info.Offset)), "skip=1", "if="+rootfs, "of="+trimfile.Name())
	err = cmd.Run()
	if err != nil {
		sylog.Errorf("dd Failed", err.Error())
		return err
	}

	//copy filesystem into bundle rootfs
	cmd = exec.Command("unsquashfs", "-f", "-d", b.Rootfs(), trimfile.Name())
	err = cmd.Run()
	if err != nil {
		sylog.Errorf("unsquashfs Failed", err.Error())
		return err
	}

	return err
}

func (cp *ShubConveyorPacker) unpackExt3(b *Bundle, info *loop.Info64, rootfs string) (err error) {
	tmpmnt, err := ioutil.TempDir(cp.tmpfs, "mnt")

	var number int
	info.Flags = loop.FlagsAutoClear
	arguments := &args.LoopArgs{
		Image: rootfs,
		Mode:  os.O_RDONLY,
		Info:  *info,
	}
	err = getLoopDevice(arguments)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/dev/loop%d", number)
	sylog.Debugf("Mounting loop device %s to %s\n", path, tmpmnt)
	err = syscall.Mount(path, tmpmnt, "ext3", syscall.MS_NOSUID|syscall.MS_RDONLY|syscall.MS_NODEV, "errors=remount-ro")
	if err != nil {
		sylog.Errorf("Mount Failed", err.Error())
		return err
	}
	defer syscall.Unmount(tmpmnt, 0)

	//copy filesystem into bundle rootfs
	cmd := exec.Command("cp", "-r", tmpmnt+`/.`, b.Rootfs())
	err = cmd.Run()
	if err != nil {
		sylog.Errorf("cp Failed", err.Error())
		return err
	}

	return err
}

// getLoopDevice attaches a loop device with the specified arguments
func getLoopDevice(arguments *args.LoopArgs) error {
	var reply int
	reply = 1
	loopdev := new(loop.Device)

	if err := loopdev.Attach(arguments.Image, arguments.Mode, &reply); err != nil {
		return err
	}

	if err := loopdev.SetStatus(&arguments.Info); err != nil {
		return err
	}
	return nil
}
