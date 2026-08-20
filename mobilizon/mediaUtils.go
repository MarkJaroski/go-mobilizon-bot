// Package mobilizon implements a Mobilizon GraphQL client for golang
package mobilizon

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/gen2brain/avif"
	"github.com/vincent-petithory/dataurl"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const MAX_IMG_SIZE = 1024 * 800 // 800kb
const IMAGE_RESIZE_WIDTH = 600

func loadFileContents(path string) ([]byte, fs.FileInfo, error) {
	var fileContents []byte
	var fi fs.FileInfo
	if strings.HasPrefix(path, "data:") {
		dataURL, err := dataurl.DecodeString(path)
		if err != nil {
			return nil, nil, err
		}
		fileContents = dataURL.Data
	} else {

		// grab the file
		file, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		defer file.Close()

		// get the contents
		fileContents, err = io.ReadAll(file)
		if err != nil {
			return nil, nil, err
		}

		// get the filename etc
		fi, err = file.Stat()
		if err != nil {
			return nil, nil, err
		}
	}
	return fileContents, fi, nil
}

// thumbnail creates a resized image from the reader and writes it to
// the writer. The mimetype determines how the image will be decoded
// and must be either "image/jpeg" or "image/png". The desired width
// of the thumbnail is specified in pixels, and the resulting height
// will be calculated to preserve the aspect ratio.
func thumbnail(r io.Reader, w io.Writer, mimetype string, width int) error {
	var src image.Image
	var err error

	switch mimetype {
	case "image/jpg":
		src, err = jpeg.Decode(r)
	case "image/jpeg":
		src, err = jpeg.Decode(r)
	case "image/png":
		src, err = png.Decode(r)
	case "image/avif":
		src, err = avif.Decode(r)
	case "image/webp":
		src, err = webp.Decode(r)
	default:
		err = errors.New("Unknown MIME Type " + mimetype)
	}

	if err != nil {
		return err
	}

	ratio := (float64)(src.Bounds().Max.Y) / (float64)(src.Bounds().Max.X)
	height := int(math.Round(float64(width) * ratio))

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	err = jpeg.Encode(w, dst, nil)
	if err != nil {
		return err
	}

	return nil
}

// downloadFile downloads a file from a given URL and returns the local
// file path or "" and an error or nil
func downloadFile(URL string) (string, error) {
	// if this is a data URL just return it. The uplaod function will deal.
	if strings.HasPrefix(URL, "data:") {
		return URL, nil
	}

	//Get the response bytes from the url
	client := &http.Client{}
	req, err := http.NewRequest("GET", strings.Split(URL, "?")[0], nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36")
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return "", errors.New(fmt.Sprintf("Received response code %d for %s", response.StatusCode, URL))
	}

	// get tmp filename
	f, err := os.CreateTemp("", "cc2mob.")
	if err != nil {
		return f.Name(), err
	}

	//Create a empty file
	file, err := os.Create(f.Name())
	if err != nil {
		return f.Name(), err
	}
	defer file.Close()

	//Write the bytes to the file
	if response.ContentLength > MAX_IMG_SIZE || strings.HasSuffix(URL, ".avif") {
		err = thumbnail(response.Body, file, response.Header.Get("Content-Type"), IMAGE_RESIZE_WIDTH)
	} else {
		_, err = io.Copy(file, response.Body)
	}
	if err != nil {
		return f.Name(), err
	}

	return f.Name(), nil
}
