// Copyright 2013 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package data provides common shared data structures for imageproxy.
package proxy

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Options specifies transformations that can be performed on a
// requested image.
type Options struct {
	Width  float64 // requested width, in pixels
	Height float64 // requested height, in pixels

	// If true, resize the image to fit in the specified dimensions.  Image
	// will not be cropped, and aspect ratio will be maintained.
	Fit bool

	// Rotate image the specified degrees counter-clockwise.  Valid values are 90, 180, 270.
	Rotate int

	FlipVertical   bool
	FlipHorizontal bool
}

func (o Options) String() string {
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "%vx%v", o.Width, o.Height)
	if o.Fit {
		buf.WriteString(",fit")
	}
	if o.Rotate != 0 {
		fmt.Fprintf(buf, ",r%d", o.Rotate)
	}
	if o.FlipVertical {
		buf.WriteString(",fv")
	}
	if o.FlipHorizontal {
		buf.WriteString(",fh")
	}
	return buf.String()
}

func ParseOptions(str string) *Options {
	o := new(Options)
	var h, w string

	parts := strings.Split(str, ",")

	// parse size
	size := strings.SplitN(parts[0], "x", 2)
	w = size[0]
	if len(size) > 1 {
		h = size[1]
	} else {
		h = w
	}

	if w != "" {
		o.Width, _ = strconv.ParseFloat(w, 64)
	}
	if h != "" {
		o.Height, _ = strconv.ParseFloat(h, 64)
	}

	for _, part := range parts[1:] {
		if part == "fit" {
			o.Fit = true
			continue
		}
		if part == "fv" {
			o.FlipVertical = true
			continue
		}
		if part == "fh" {
			o.FlipHorizontal = true
			continue
		}

		if len(part) > 2 && strings.HasPrefix(part, "r") {
			o.Rotate, _ = strconv.Atoi(part[1:])
			continue
		}
	}

	return o
}

type Request struct {
	URL     *url.URL // URL of the image to proxy
	Options *Options // Image transformation to perform
}

// Image represents a remote image that is being proxied.  It tracks where
// the image was originally retrieved from and how long the image can be cached.
type Image struct {
	// URL of original remote image.
	URL string

	// Expires is the cache expiration time for the original image, as
	// returned by the remote server.
	Expires time.Time

	// Etag returned from server when fetching image.
	Etag string

	// Bytes contains the actual image.
	Bytes []byte
}