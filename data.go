package imageproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	optFit             = "fit"
	optFlipVertical    = "fv"
	optFlipHorizontal  = "fh"
	optFormatJPEG      = "jpeg"
	optFormatPNG       = "png"
	optFormatTIFF      = "tiff"
	optRotatePrefix    = "r"
	optQualityPrefix   = "q"
	optSignaturePrefix = "s"
	optSizeDelimiter   = "x"
	optScaleUp         = "scaleUp"
	optCropX           = "cx"
	optCropY           = "cy"
	optCropWidth       = "cw"
	optCropHeight      = "ch"
	optSmartCrop       = "sc"
)

// URLError reports a malformed URL error.
type URLError struct {
	Message string
	URL     *url.URL
}

func (e URLError) Error() string {
	return fmt.Sprintf("malformed URL %q: %s", e.URL, e.Message)
}

// Options specifies transformations to be performed on the requested image.
type Options struct {
	// Make sure Options never has pointer members. The config parsing depends on that.

	// See ParseOptions for interpretation of Width and Height values
	Width  float64 `json:"width"`
	Height float64 `json:"height"`

	// If true, resize the image to fit in the specified dimensions.  Image
	// will not be cropped, and aspect ratio will be maintained.
	Fit bool `json:"fit"`

	// Rotate image the specified degrees counter-clockwise.  Valid values
	// are 90, 180, 270.
	Rotate int `json:"rotate"`

	FlipVertical   bool `json:"flip_vertical"`
	FlipHorizontal bool `json:"flip_horizontal"`

	// Quality of output image
	Quality int `json:"quality"`

	// HMAC Signature for signed requests.
	Signature string `json:"signature"`

	// Allow image to scale beyond its original dimensions.  This value
	// will always be overwritten by the value of Proxy.ScaleUp.
	ScaleUp bool `json:"scale_up"`

	// Desired image format. Valid values are "jpeg", "png", "tiff".
	Format string `json:"format"`

	// Crop rectangle params
	CropX      float64 `json:"crop_x"`
	CropY      float64 `json:"crop_y"`
	CropWidth  float64 `json:"crop_width"`
	CropHeight float64 `json:"crop_height"`

	// Automatically find good crop points based on image content.
	SmartCrop bool `json:"smart_crop"`
}

type SourceConfiguration struct {
	BaseURL        *url.URL
	DefaultOptions Options
}

func (conf *SourceConfiguration) UnmarshalJSON(bytes []byte) error {
	/*
		Make it possible to unmarshal bytes into a struct with a *url.URL field by first unmarshaling into
		a struct without *url.URLs and then parsing the URL.
	*/
	var confWithString struct {
		BaseURL        string  `json:"base_url"`
		DefaultOptions Options `json:"default_options"`
	}
	err := json.Unmarshal(bytes, &confWithString)
	if err != nil {
		return err
	}
	baseURL, err := url.Parse(confWithString.BaseURL)
	if err != nil {
		return err
	}

	conf.BaseURL = baseURL
	conf.DefaultOptions = confWithString.DefaultOptions
	return nil
}

func (o Options) String() string {
	opts := []string{fmt.Sprintf("%v%s%v", o.Width, optSizeDelimiter, o.Height)}
	if o.Fit {
		opts = append(opts, optFit)
	}
	if o.Rotate != 0 {
		opts = append(opts, fmt.Sprintf("%s%d", string(optRotatePrefix), o.Rotate))
	}
	if o.FlipVertical {
		opts = append(opts, optFlipVertical)
	}
	if o.FlipHorizontal {
		opts = append(opts, optFlipHorizontal)
	}
	if o.Quality != 0 {
		opts = append(opts, fmt.Sprintf("%s%d", string(optQualityPrefix), o.Quality))
	}
	if o.Signature != "" {
		opts = append(opts, fmt.Sprintf("%s%s", string(optSignaturePrefix), o.Signature))
	}
	if o.ScaleUp {
		opts = append(opts, optScaleUp)
	}
	if o.Format != "" {
		opts = append(opts, o.Format)
	}
	if o.CropX != 0 {
		opts = append(opts, fmt.Sprintf("%s%v", string(optCropX), o.CropX))
	}
	if o.CropY != 0 {
		opts = append(opts, fmt.Sprintf("%s%v", string(optCropY), o.CropY))
	}
	if o.CropWidth != 0 {
		opts = append(opts, fmt.Sprintf("%s%v", string(optCropWidth), o.CropWidth))
	}
	if o.CropHeight != 0 {
		opts = append(opts, fmt.Sprintf("%s%v", string(optCropHeight), o.CropHeight))
	}
	if o.SmartCrop {
		opts = append(opts, optSmartCrop)
	}
	return strings.Join(opts, ",")
}

// transform returns whether o includes transformation options.  Some fields
// are not transform related at all (like Signature), and others only apply in
// the presence of other fields (like Fit).  A non-empty Format value is
// assumed to involve a transformation.
func (o Options) transform() bool {
	return o.Width != 0 || o.Height != 0 || o.Rotate != 0 || o.FlipHorizontal || o.FlipVertical || o.Quality != 0 || o.Format != "" || o.CropX != 0 || o.CropY != 0 || o.CropWidth != 0 || o.CropHeight != 0
}

// ParseFormValues parses a url.Values to transformation options.
// The options can be specified in in order, with duplicate options overwriting
// previous values.
//
// Rectangle Crop
//
// 	crop={x,y,width,height}
//
// 	x      - X coordinate of top left rectangle corner (default: 0)
// 	y      - Y coordinate of top left rectangle corner (default: 0)
// 	width  - rectangle width (default: image width)
// 	height - rectangle height (default: image height)
//
// For all options, integer values are interpreted as exact pixel values and
// floats between 0 and 1 are interpreted as percentages of the original image
// size. Negative values for cx and cy are measured from the right and bottom
// edges of the image, respectively.
//
// If the crop width or height exceed the width or height of the image, the
// crop width or height will be adjusted, preserving the specified cx and cy
// values.  Rectangular crop is applied before any other transformations.
//
// Smart Crop
//
// 	mode=smartcrop
//
// The option will perform a content-aware smart crop to fit the
// requested image width and height dimensions (see Size and Cropping below).
// The smart crop option will override any requested rectangular crop.
//
// Size and Cropping
//
//	width={width}&height={height}
//
// Integer values greater than 1 are interpreted as exact
// pixel values. Floats between 0 and 1 are interpreted as percentages of the
// original image size. If either value is omitted or set to 0, it will be
// automatically set to preserve the aspect ratio based on the other dimension.
//
//	size={size}
//
// If a single size is provided, it will override both width and height.
//
// Depending on the size options specified, an image may be cropped to fit the
// requested size. In all cases, the original aspect ratio of the image will be
// preserved; imageproxy will never stretch the original image.
//
// When no explicit crop mode is specified, the following rules are followed:
//
// - If both width and height values are specified, the image will be scaled to
// fill the space, cropping if necessary to fit the exact dimension.
//
// - If only one of the width or height values is specified, the image will be
// resized to fit the specified dimension, scaling the other dimension as
// needed to maintain the aspect ratio.
//
// If the "mode=fit" option is specified together with a width and height value, the
// image will be resized to fit within a containing box of the specified size.
// As always, the original aspect ratio will be preserved. Specifying the "fit"
// option with only one of either width or height does the same thing as if
// "fit" had not been specified.
//
// Rotation and Flips
//
// The "r={degrees}" option will rotate the image the specified number of
// degrees, counter-clockwise. Valid degrees values are 90, 180, and 270.
//
// The "flip=v" option will flip the image vertically. The "flip=h" option will flip
// the image horizontally. Images are flipped after being rotated.
//
// Quality
//
// The "quality={qualityPercentage}" option can be used to specify the quality of the
// output file (JPEG only). If not specified, the default value of "95" is used.
//
// Format
//
// The "format=jpeg", "format=png", and "format=tiff"  options can be used to specify
// the desired image format of the proxied image.
//
// Signature
//
// The "signature={signature}" option specifies an optional base64 encoded HMAC used to
// sign the remote URL in the request.  The HMAC key used to verify signatures is
// provided to the imageproxy server on startup.
//
// See https://github.com/willnorris/imageproxy/wiki/URL-signing
// for examples of generating signatures.
//
// Examples
//
// 	size=0                  - no resizing
// 	width=200               - 200 pixels wide, proportional height
// 	height=0.15             - 15% original height, proportional width
// 	width=100&height=150    - 100 by 150 pixels, cropping as needed
// 	size=100                - 100 pixels square, cropping as needed
// 	size=150,mode=fit       - scale to fit 150 pixels square, no cropping
// 	size=100,rotate=90      - 100 pixels square, rotated 90 degrees
// 	size=100,flip=v,flip=h  - 100 pixels square, flipped horizontal and vertical
// 	width=200,quality=60    - 200 pixels wide, proportional height, 60% quality
// 	width=200,format=png    - 200 pixels wide, converted to PNG format
// 	crop=0,0,100,100        - crop image to 100px square, starting at (0,0)
// 	crop=10,20,100,200      - crop image starting at (10,20) is 100px wide and 200px tall
func ParseFormValues(form url.Values, defaultOptions Options) Options {
	// This should make a copy, since we are dealing with structs, not pointers, and Options does not have pointer members.
	options := defaultOptions

	modeSeen := false

	for key, values := range form {
		for _, value := range values {
			switch key {
			case "mode":
				switch value {
				case "fit":
					options.Fit = true
				case "smartcrop":
					options.SmartCrop = true
				}
				modeSeen = true
			case "flip":
				switch value {
				case "v":
					options.FlipVertical = true
				case "h":
					options.FlipHorizontal = true
				}
			case "format":
				switch value {
				case optFormatJPEG:
					options.Format = optFormatJPEG
				case optFormatPNG:
					options.Format = optFormatPNG
				case optFormatTIFF:
					options.Format = optFormatTIFF
				}
			case "rotate":
				options.Rotate, _ = strconv.Atoi(value)
			case "quality":
				options.Quality, _ = strconv.Atoi(value)
			case "signature":
				options.Signature = value
			case "crop":
				cropValues := strings.Split(value, ",")
				if len(cropValues) == 4 {
					options.CropX, _ = strconv.ParseFloat(cropValues[0], 64)
					options.CropY, _ = strconv.ParseFloat(cropValues[1], 64)
					options.CropWidth, _ = strconv.ParseFloat(cropValues[2], 64)
					options.CropHeight, _ = strconv.ParseFloat(cropValues[3], 64)
				}
			case "width":
				options.Width, _ = strconv.ParseFloat(value, 64)
			case "height":
				options.Height, _ = strconv.ParseFloat(value, 64)
			case "size":
				size, err := strconv.ParseFloat(value, 64)
				if err == nil {
					options.Width = size
					options.Height = size
				}
			}
		}
	}

	/*
		The transformation code doesn't currently do anything if Fit is true and either width or height is 0.
		However, for libpixel compatibility Fit is supposed to be the default. Ask for Fit if no mode
		was specified and width and height are positive.
	*/
	if !modeSeen && options.Width > 0 && options.Height > 0 {
		options.Fit = true
	}

	return options
}

// ParseOptions is useful, although no longer exposed to the API
func ParseOptions(str string) Options {
	var options Options

	for _, opt := range strings.Split(str, ",") {
		switch {
		case len(opt) == 0:
			break
		case opt == optFit:
			options.Fit = true
		case opt == optFlipVertical:
			options.FlipVertical = true
		case opt == optFlipHorizontal:
			options.FlipHorizontal = true
		case opt == optScaleUp: // this option is intentionally not documented above
			options.ScaleUp = true
		case opt == optFormatJPEG, opt == optFormatPNG, opt == optFormatTIFF:
			options.Format = opt
		case opt == optSmartCrop:
			options.SmartCrop = true
		case strings.HasPrefix(opt, optRotatePrefix):
			value := strings.TrimPrefix(opt, optRotatePrefix)
			options.Rotate, _ = strconv.Atoi(value)
		case strings.HasPrefix(opt, optQualityPrefix):
			value := strings.TrimPrefix(opt, optQualityPrefix)
			options.Quality, _ = strconv.Atoi(value)
		case strings.HasPrefix(opt, optSignaturePrefix):
			options.Signature = strings.TrimPrefix(opt, optSignaturePrefix)
		case strings.HasPrefix(opt, optCropX):
			value := strings.TrimPrefix(opt, optCropX)
			options.CropX, _ = strconv.ParseFloat(value, 64)
		case strings.HasPrefix(opt, optCropY):
			value := strings.TrimPrefix(opt, optCropY)
			options.CropY, _ = strconv.ParseFloat(value, 64)
		case strings.HasPrefix(opt, optCropWidth):
			value := strings.TrimPrefix(opt, optCropWidth)
			options.CropWidth, _ = strconv.ParseFloat(value, 64)
		case strings.HasPrefix(opt, optCropHeight):
			value := strings.TrimPrefix(opt, optCropHeight)
			options.CropHeight, _ = strconv.ParseFloat(value, 64)
		case strings.Contains(opt, optSizeDelimiter):
			size := strings.SplitN(opt, optSizeDelimiter, 2)
			if w := size[0]; w != "" {
				options.Width, _ = strconv.ParseFloat(w, 64)
			}
			if h := size[1]; h != "" {
				options.Height, _ = strconv.ParseFloat(h, 64)
			}
		default:
			if size, err := strconv.ParseFloat(opt, 64); err == nil {
				options.Width = size
				options.Height = size
			}
		}
	}

	return options
}

func StripOurOptions(rawQuery string) (string, error) {
	// Delete our options. This is useful when the request is pushed upstream.
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", err
	}
	newValues := make(url.Values, len(values))
	for key := range values {
		switch key {
		// Do not copy our values
		case "mode":
		case "flip":
		case "format":
		case "rotate":
		case "quality":
		case "signature":
		case "crop":
		case "width":
		case "height":
		case "size":

		// Do copy other values
		default:
			newValues[key] = values[key]
		}
	}
	return newValues.Encode(), nil
}

// Request is an imageproxy request which includes a remote URL of an image to
// proxy, and an optional set of transformations to perform.
type Request struct {
	URL      *url.URL      // URL of the image to proxy
	Options  Options       // Image transformation to perform
	Original *http.Request // The original HTTP request
}

// String returns the request URL as a string, with r.Options encoded in the
// URL fragment.
func (r Request) String() string {
	u := *r.URL
	u.Fragment = r.Options.String()
	return u.String()
}

// NewRequest parses an http.Request into an imageproxy Request.  Options and
// the remote image URL are specified in the request path, formatted as:
// /{options}/{remote_url}.  Options may be omitted, so a request path may
// simply contain /{remote_url}.  The remote URL must be an absolute "http" or
// "https" URL, should not be URL encoded, and may contain a query string.
//
// Assuming an imageproxy server running on localhost, the following are all
// valid imageproxy requests:
//
// 	http://localhost/100x200/http://example.com/image.jpg
// 	http://localhost/100x200,r90/http://example.com/image.jpg?foo=bar
// 	http://localhost//http://example.com/image.jpg
// 	http://localhost/http://example.com/image.jpg
func NewRequest(r *http.Request, prefixesToConfigs map[string]*SourceConfiguration) (*Request, error) {
	var err error
	req := &Request{Original: r}

	req.URL, err = buildFinalAbsoluteURL(prefixesToConfigs, r.URL)
	if err != nil {
		return nil, err
	}

	if !req.URL.IsAbs() {
		return nil, URLError{"must provide absolute remote URL", r.URL}
	}

	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, URLError{"remote URL must have http or https scheme", r.URL}
	}

	_, config := bestMatchingConfig(prefixesToConfigs, r.URL)
	var defaultOptions Options
	if config != nil {
		defaultOptions = config.DefaultOptions
	} else {
		defaultOptions = Options{}
	}

	// Options are now based on query strings params of the original request
	err = r.ParseForm()
	if err != nil {
		return nil, err
	}
	req.Options = ParseFormValues(r.Form, defaultOptions)

	req.URL.RawQuery = r.URL.RawQuery

	return req, nil
}

func buildFinalAbsoluteURL(prefixesToConfigs map[string]*SourceConfiguration, originalURL *url.URL) (*url.URL, error) {
	path := originalURL.EscapedPath()[1:]

	matchingPrefix, config := bestMatchingConfig(prefixesToConfigs, originalURL)

	if config != nil {
		urlPrefixWithoutTail := strings.TrimRight(matchingPrefix, "/")
		strippedPath := path[len(urlPrefixWithoutTail):] // strip the prefix

		finalURL, err := parseURL(strippedPath)

		// Add parsed URL to the matching base URL if there is one
		if config.BaseURL != nil {
			finalURL = config.BaseURL.ResolveReference(finalURL)
		}
		return finalURL, err

	} else {
		// Not a single matching prefix was found.
		return parseURL(path)
	}
}

func bestMatchingConfig(prefixesToConfigs map[string]*SourceConfiguration, originalURL *url.URL) (string, *SourceConfiguration) {
	bestMatchLen := -1
	bestMatchPrefix := ""
	var bestConfig *SourceConfiguration = nil

	for urlPrefix, config := range prefixesToConfigs {
		urlPrefixWithoutTail := strings.TrimRight(urlPrefix, "/")

		if strings.HasPrefix(originalURL.EscapedPath(), urlPrefixWithoutTail) {
			matchLen := len(urlPrefixWithoutTail)
			if matchLen < bestMatchLen {
				continue
			}

			// A better thing found
			bestMatchLen = matchLen
			bestMatchPrefix = urlPrefix
			bestConfig = config
		}
	}
	return bestMatchPrefix, bestConfig
}

var reCleanedURL = regexp.MustCompile(`^(https?):/+([^/])`)

// parseURL parses s as a URL, handling URLs that have been munged by
// path.Clean or a webserver that collapses multiple slashes.
func parseURL(s string) (*url.URL, error) {
	s = reCleanedURL.ReplaceAllString(s, "$1://$2")
	return url.Parse(s)
}
