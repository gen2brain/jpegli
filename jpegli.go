// Package jpegli implements an JPEG image encoder/decoder based on jpegli compiled to WASM.
package jpegli

import (
	"errors"
	"image"
	"image/draw"
	"io"
)

// Errors .
var (
	ErrMemRead  = errors.New("jpegli: mem read failed")
	ErrMemWrite = errors.New("jpegli: mem write failed")
	ErrDecode   = errors.New("jpegli: decode failed")
	ErrEncode   = errors.New("jpegli: encode failed")
)

// DefaultQuality is the default quality encoding parameter.
const DefaultQuality = 75

// DefaultDCTMethod is the default DCT algorithm method.
const DefaultDCTMethod = DCTISlow

// DCTMethod is the DCT/IDCT method type.
type DCTMethod int

const (
	// DCTISlow is slow but accurate integer algorithm
	DCTISlow DCTMethod = iota
	// DCTIFast is faster, less accurate integer method
	DCTIFast
	// DCTFloat is floating-point: accurate, fast on fast HW
	DCTFloat
)

const (
	alignSize = 16
)

// EncodingOptions are the encoding parameters.
type EncodingOptions struct {
	// Quality in the range [0,100]. Default is 75.
	Quality int
	// Chroma subsampling setting, 444|440|422|420.
	ChromaSubsampling image.YCbCrSubsampleRatio
	// Progressive level in the range [0,2], where level 0 is sequential, and greater level value means more progression steps.
	ProgressiveLevel int
	// Huffman code optimization.
	// Enabled by default.
	OptimizeCoding bool
	// Uses adaptive quantization for creating more zero coefficients.
	// Enabled by default.
	AdaptiveQuantization bool
	// Use standard quantization tables from Annex K of the JPEG standard.
	// By default, jpegli uses a different set of quantization tables and different scaling parameters for DC and AC coefficients.
	StandardQuantTables bool
	// Apply fancy downsampling.
	FancyDownsampling bool
	// DCTMethod is the DCT algorithm method.
	DCTMethod DCTMethod
}

// DecodingOptions are the decoding parameters.
type DecodingOptions struct {
	// ScaleTarget is the target size to scale image.
	ScaleTarget image.Rectangle
	// Fancy upsampling.
	FancyUpsampling bool
	// Block smoothing.
	BlockSmoothing bool
	// Use arithmetic coding instead of Huffman.
	ArithCode bool
	// DCTMethod is DCT Algorithm method.
	DCTMethod DCTMethod
}

// Decode reads a JPEG image from r and returns it as an image.Image.
func Decode(r io.Reader) (image.Image, error) {
	var err error
	var img image.Image

	img, _, err = decode(r, false, false, false, false, DefaultDCTMethod, 0, 0)
	if err != nil {
		return nil, err
	}

	return img, nil
}

// DecodeWithOptions reads a JPEG image from r with decoding options.
func DecodeWithOptions(r io.Reader, o *DecodingOptions) (image.Image, error) {
	var err error
	var img image.Image

	tw := o.ScaleTarget.Dx()
	th := o.ScaleTarget.Dy()
	fancyUpsampling := o.FancyUpsampling
	blockSmoothing := o.BlockSmoothing
	arithCode := o.ArithCode
	dctMethod := o.DCTMethod

	img, _, err = decode(r, false, fancyUpsampling, blockSmoothing, arithCode, dctMethod, tw, th)
	if err != nil {
		return nil, err
	}

	return img, nil
}

// DecodeConfig returns the color model and dimensions of a JPEG image without decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	var err error
	var cfg image.Config

	_, cfg, err = decode(r, true, false, false, false, DefaultDCTMethod, 0, 0)
	if err != nil {
		return image.Config{}, err
	}

	return cfg, nil
}

// Encode writes the image m to w with the given options.
func Encode(w io.Writer, m image.Image, o ...*EncodingOptions) error {
	quality := DefaultQuality
	chromaSubsampling := image.YCbCrSubsampleRatio420
	progressiveLevel := 0
	optimizeCoding := true
	adaptiveQuantization := true
	standardQuantTables := false
	fancyDownsampling := false
	dctMethod := DefaultDCTMethod

	if o != nil && o[0] != nil {
		opt := o[0]
		quality = opt.Quality
		chromaSubsampling = opt.ChromaSubsampling
		progressiveLevel = opt.ProgressiveLevel

		if quality <= 0 {
			quality = DefaultQuality
		} else if quality > 100 {
			quality = 100
		}

		if progressiveLevel < 0 {
			progressiveLevel = 0
		} else if progressiveLevel > 2 {
			progressiveLevel = 2
		}

		optimizeCoding = opt.OptimizeCoding
		adaptiveQuantization = opt.AdaptiveQuantization
		standardQuantTables = opt.StandardQuantTables
		fancyDownsampling = opt.FancyDownsampling
		dctMethod = opt.DCTMethod
	}

	err := encode(w, m, quality, int(chromaSubsampling), progressiveLevel, optimizeCoding, adaptiveQuantization,
		standardQuantTables, fancyDownsampling, dctMethod)
	if err != nil {
		return err
	}

	return nil
}

// Init initializes wazero runtime and compiles the module.
// There is no need to explicitly call this function, first Decode/Encode will initialize the runtime.
func Init() {
	initOnce()
}

func imageToRGBA(src image.Image) *image.RGBA {
	if dst, ok := src.(*image.RGBA); ok {
		return dst
	}

	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)

	return dst
}

func yCbCrSize(r image.Rectangle, subsampleRatio image.YCbCrSubsampleRatio) (w, h, cw, ch int) {
	w, h = r.Dx(), r.Dy()

	switch subsampleRatio {
	case image.YCbCrSubsampleRatio422:
		cw = (r.Max.X+1)/2 - r.Min.X/2
		ch = h
	case image.YCbCrSubsampleRatio420:
		cw = (r.Max.X+1)/2 - r.Min.X/2
		ch = (r.Max.Y+1)/2 - r.Min.Y/2
	case image.YCbCrSubsampleRatio440:
		cw = w
		ch = (r.Max.Y+1)/2 - r.Min.Y/2
	case image.YCbCrSubsampleRatio411:
		cw = (r.Max.X+3)/4 - r.Min.X/4
		ch = h
	case image.YCbCrSubsampleRatio410:
		cw = (r.Max.X+3)/4 - r.Min.X/4
		ch = (r.Max.Y+1)/2 - r.Min.Y/2
	default:
		cw = w
		ch = h
	}

	return
}

func alignm(a int) int {
	return (a + (alignSize - 1)) & (^(alignSize - 1))
}

// packYCbCr packs an image.YCbCr into the contiguous MCU-aligned Y/Cb/Cr
// planes the raw_data_in encoder expects, edge-replicating the padding.
func packYCbCr(img *image.YCbCr) []byte {
	width := img.Rect.Dx()
	height := img.Rect.Dy()

	w := alignm(width)
	h := alignm(height)

	_, _, scw, sch := yCbCrSize(img.Rect, img.SubsampleRatio)

	var cw, ch int
	switch img.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		cw, ch = (w+1)/2, h
	case image.YCbCrSubsampleRatio420:
		cw, ch = (w+1)/2, (h+1)/2
	case image.YCbCrSubsampleRatio440:
		cw, ch = w, (h+1)/2
	default:
		cw, ch = w, h
	}

	i0 := w * h
	i1 := i0 + cw*ch
	i2 := i1 + cw*ch
	data := make([]byte, i2)

	for dy := 0; dy < h; dy++ {
		sy := dy
		if sy >= height {
			sy = height - 1
		}
		yo := img.YOffset(img.Rect.Min.X, img.Rect.Min.Y+sy)
		row := data[dy*w : dy*w+w]
		copy(row, img.Y[yo:yo+width])
		padEdge(row, width)
	}

	cbase := img.COffset(img.Rect.Min.X, img.Rect.Min.Y)

	for dc := 0; dc < ch; dc++ {
		sc := dc
		if sc >= sch {
			sc = sch - 1
		}
		so := cbase + sc*img.CStride
		cb := data[i0+dc*cw : i0+dc*cw+cw]
		cr := data[i1+dc*cw : i1+dc*cw+cw]
		copy(cb, img.Cb[so:so+scw])
		copy(cr, img.Cr[so:so+scw])
		padEdge(cb, scw)
		padEdge(cr, scw)
	}

	return data
}

func padEdge(row []byte, valid int) {
	if valid <= 0 || valid >= len(row) {
		return
	}

	last := row[valid-1]
	for i := valid; i < len(row); i++ {
		row[i] = last
	}
}

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", Decode, DecodeConfig)
}
