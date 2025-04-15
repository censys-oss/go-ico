package ico

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strings"

	bmp "github.com/jsummers/gobmp"
)

const PngHeader = "\x89PNG\r\n\x1a\n"
// 
const BmpFileHeaderSize = 14
const BmpDibHeaderSize = 40

// A FormatError reports that the input is not a valid ICO.
type FormatError string

func (e FormatError) Error() string { return "invalid ICO format: " + string(e) }

// If the io.Reader does not also have ReadByte, then decode will introduce its own buffering.
type reader interface {
	io.Reader
	io.ByteReader
}

var ErrMemoryLimitExceeded = FormatError("memory limit exceeded during ICO decoding")

type decoder struct {
	r                            reader
	num                          uint16
	dir                          []entry
	image                        []image.Image
	cfg                          image.Config
	memoryLimit, allocatedMemory uint32
}

type DecodeOptions struct {
	// max bytes allowed to allocate
	memoryLimit uint32
}

type DecodeOptFunc func(*DecodeOptions)

// WithMemoryLimit sets memory limit
func WithMemoryLimit(limit uint32) DecodeOptFunc {
	return func(o *DecodeOptions) {
		o.memoryLimit = max(o.memoryLimit, limit)
	}
}

func (d *decoder) allocMemory(size uint32) ([]byte, error) {
	if d.memoryLimit > 0 {
		if d.allocatedMemory+size > d.memoryLimit {
			return nil, ErrMemoryLimitExceeded
		}
		d.allocatedMemory += size
	}
	return make([]byte, size), nil
}

func (d *decoder) decode(r io.Reader, configOnly bool) error {
	// Add buffering if r does not provide ReadByte.
	if rr, ok := r.(reader); ok {
		d.r = rr
	} else {
		d.r = bufio.NewReader(r)
	}

	if err := d.readHeader(); err != nil {
		return err
	}
	if err := d.readImageDir(configOnly); err != nil {
		return err
	}
	if configOnly {
		cfg, err := d.parseConfig(d.dir[0])
		if err != nil {
			return err
		}
		d.cfg = cfg
	} else {
		// d.num is upper bounded by 65k
		// not going to account for this in allocs
		d.image = make([]image.Image, d.num)
		for i, entry := range d.dir {
			img, err := d.parseImage(entry)
			if err != nil {
				return err
			}
			d.image[i] = img
		}
	}
	return nil
}

type Header struct {
	First, Second, Num uint16
}

func (d *decoder) readHeader() error {

	h := Header{}
	err := binary.Read(d.r, binary.LittleEndian, &h)
	if err != nil {
		return FormatError(fmt.Sprintf("failed to read first 6 bytes/header: %v", err))
	}

	if h.First != 0 {
		return FormatError(fmt.Sprintf("first 2 bytes is %d instead of 0", h.First))
	}
	if h.Second != 1 {
		return FormatError(fmt.Sprintf("second 2 bytes is %d instead of 1", h.Second))
	}
	if h.Num == 0 {
		return FormatError(fmt.Sprintf("third 2 bytes is %d, implying 0 images", d.num))
	}
	d.num = h.Num
	return nil
}

func (d *decoder) readImageDir(configOnly bool) error {
	n := int(d.num)
	if configOnly {
		n = 1
	}
	for i := 0; i < n; i++ {
		var e entry
		err := binary.Read(d.r, binary.LittleEndian, &e)
		if err != nil {
			return err
		}
		d.dir = append(d.dir, e)
	}
	return nil
}

func (d *decoder) parseImage(e entry) (image.Image, error) {
	data, err := d.allocMemory(e.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate data buffer: %w", err)
	}
	// sometimes e.Size is larger than the data
	// permissively handle this
	_, _ = io.ReadFull(d.r, data)

	// Check if the image is a PNG by the first 8 bytes of the image data
	if strings.HasPrefix(string(data), PngHeader) {
		return png.Decode(bytes.NewReader(data))
	}

	// Decode as BMP instead
	bmpBytes, maskBytes, offset, err := d.setupBMP(e, data)
	if err != nil {
		return nil, err
	}

	src, err := bmp.Decode(bytes.NewReader(bmpBytes))
	if err != nil {
		return nil, err
	}

	bnd := src.Bounds()
	mask := image.NewAlpha(image.Rect(0, 0, bnd.Dx(), bnd.Dy()))
	dst := image.NewNRGBA(image.Rect(0, 0, bnd.Dx(), bnd.Dy()))
	// Fill in mask from the ICO file's AND mask data
	rowSize := ((int(e.Width) + 31) / 32) * 4
	b := make([]byte, 4)
	imageRowSize := ((int(e.Bits)*int(e.Width) + 31) / 32) * 4
	// guard against out of bounds access
	// guards against case only hit in 32 bpp icos
	if e.Bits == 32 && offset+int(e.Height-1)*int(imageRowSize)+int(e.Width-1)*4 > len(bmpBytes) {
		return nil,
			FormatError(
				"failed to parse image, offset+r*imageRowSize+c*4 exceeds bmpBytes len")
	}

	for r := 0; r < int(e.Height); r++ {
		for c := 0; c < int(e.Width); c++ {
			if len(maskBytes) > 0 {
				// always safe, see size of maskBytes in setupBMP
				alpha := (maskBytes[r*rowSize+c/8] >> (1 * (7 - uint(c)%8))) & 0x01
				if alpha != 1 {
					mask.SetAlpha(c, int(e.Height)-r-1, color.Alpha{255})
				}

			}
			// 32 bit bmps do hacky things with an alpha channel, it's included as the 4th byte of the colors
			if e.Bits == 32 {
				_, err = io.ReadFull(bytes.NewReader(bmpBytes[offset+r*imageRowSize+c*4:]), b)
				if err != nil {
					return nil, err
				}
				mask.SetAlpha(c, int(e.Height)-r-1, color.Alpha{b[3]})
			}
		}
	}
	draw.DrawMask(dst, dst.Bounds(), src, bnd.Min, mask, bnd.Min, draw.Src)

	return dst, nil
}

func (d *decoder) parseConfig(e entry) (cfg image.Config, err error) {
	tmp, err := d.allocMemory(e.Size)
	if err != nil {
		return cfg, fmt.Errorf("failed to allocate image buffer: %w", err)
	}
	n, err := io.ReadFull(d.r, tmp)
	if n != int(e.Size) {
		return cfg, FormatError(fmt.Sprintf("only %d of %d bytes read", n, e.Size))
	}
	if err != nil {
		return cfg, err
	}

	cfg, err = png.DecodeConfig(bytes.NewReader(tmp))
	if err != nil {
		tmp, _, _, _ = d.setupBMP(e, tmp)
		cfg, err = bmp.DecodeConfig(bytes.NewReader(tmp))
	}
	return cfg, err
}

func (d *decoder) setupBMP(e entry, data []byte) ([]byte, []byte, int, error) {
	// Ico files are made up of a XOR mask and an AND mask
	// The XOR mask is the image itself, while the AND mask is a 1 bit-per-pixel alpha channel (transparent or opaque).
	// setupBMP returns the image as a BMP format byte array, and the mask as a (1bpp) pixel array

	// calculate image sizes
	// See wikipedia en.wikipedia.org/wiki/BMP_file_format
	var imageSize, maskSize uint32
	imageSize = uint32(len(data))
	if e.Bits != 32 {
		rowSize := (1 * (uint32(e.Width) + 31) / 32) * 4
		maskSize = rowSize * uint32(e.Height)
		if maskSize > imageSize {
			return nil, nil, 0, FormatError("masksize exceeds image size")
		}
		imageSize -= maskSize
	}

	if len(data) < int(imageSize) {
		return nil, nil, 0, FormatError(
			fmt.Sprintf("datalen %d smaller than imageSize %d", len(data), imageSize))
	}

	if BmpFileHeaderSize+imageSize < 10+BmpDibHeaderSize {
		return nil, nil, 0, FormatError(fmt.Sprintf("imagesize too small: %d", imageSize))
	}

	img, err := d.allocMemory(BmpFileHeaderSize + imageSize)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to allocate image buffer: %w", err)
	}

	mask, err := d.allocMemory(maskSize)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to allocate mask buffer: %w", err)
	}

	var n uint32
	// Read in image
	n = uint32(copy(img[BmpFileHeaderSize:], data[:imageSize]))
	if n != imageSize {
		return nil, nil, 0, FormatError(fmt.Sprintf("only %d of %d bytes read.", n, imageSize))
	}
	// Read in mask
	n = uint32(copy(mask, data[imageSize:]))
	if n != maskSize {
		return nil, nil, 0, FormatError(fmt.Sprintf("only %d of %d bytes read.", n, maskSize))
	}

	// following slices will not panic due to image size check above

	dibSize := binary.LittleEndian.Uint32(img[BmpFileHeaderSize : BmpFileHeaderSize+4])
	w := binary.LittleEndian.Uint32(img[BmpFileHeaderSize+4 : BmpFileHeaderSize+8])
	h := binary.LittleEndian.Uint32(img[BmpFileHeaderSize+8 : BmpFileHeaderSize+12])

	// what case is this handling?
	if h > w {
		binary.LittleEndian.PutUint32(img[BmpFileHeaderSize+8:BmpFileHeaderSize+12], h/2)
	}

	// Magic number
	copy(img[0:2], "\x42\x4D")

	// File size
	binary.LittleEndian.PutUint32(img[2:6], uint32(imageSize+BmpFileHeaderSize))

	// Calculate offset into image data
	numColors := binary.LittleEndian.Uint32(img[BmpFileHeaderSize+32 : BmpFileHeaderSize+36])
	e.Bits = binary.LittleEndian.Uint16(img[BmpFileHeaderSize+14 : BmpFileHeaderSize+16])
	e.Size = binary.LittleEndian.Uint32(img[BmpFileHeaderSize+20 : BmpFileHeaderSize+24])

	switch int(e.Bits) {
	case 1, 2, 4, 8:
		x := uint32(1 << e.Bits)
		if numColors == 0 || numColors > x {
			numColors = x
		}
	default:
		numColors = 0
	}

	var numColorsSize uint32
	switch int(dibSize) {
	case 12, 64:
		numColorsSize = numColors * 3
	default:
		numColorsSize = numColors * 4
	}

	var offset uint32
	offset = BmpFileHeaderSize + dibSize + numColorsSize

	if dibSize > BmpDibHeaderSize {
		if BmpFileHeaderSize+dibSize-4 > uint32(len(img)) {
			return nil, nil, 0,
				FormatError(fmt.Sprintf("cannot get icc with dibsize/imglen %d %d", dibSize, len(img)))
		}
		// icc
		offset += binary.LittleEndian.Uint32(img[BmpFileHeaderSize+dibSize-8 : BmpFileHeaderSize+dibSize-4])
	}
	binary.LittleEndian.PutUint32(img[10:14], offset)

	return img, mask, int(offset), nil
}

func Decode(r io.Reader, opts ...DecodeOptFunc) ([]image.Image, error) {
	var d decoder
	options := DecodeOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	d.memoryLimit = options.memoryLimit
	if err := d.decode(r, false); err != nil {
		return nil, err
	}
	return d.image, nil
}

func DecodeImg(r io.Reader) ([]image.Image, error) {
	return Decode(r)
}

func DecodeConfig(r io.Reader) (image.Config, error) {
	var d decoder
	if err := d.decode(r, true); err != nil {
		return image.Config{}, err
	}
	return d.cfg, nil
}

func init() {
	image.RegisterFormat("ico", "", func(r io.Reader) (image.Image, error) {
		imgs, err := Decode(r)
		if err != nil {
			return nil, err
		}
		// we will error if there are no images
		return imgs[0], nil
	}, DecodeConfig)
}
