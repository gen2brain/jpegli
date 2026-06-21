package jpegli_test

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/gen2brain/jpegli"
)

//go:embed testdata/test.jpg
var testJpg []byte

//go:embed testdata/test.png
var testPng []byte

//go:embed testdata/gray.jpg
var testGray []byte

//go:embed testdata/rgba.jpg
var testRgba []byte

//go:embed testdata/cmyk.jpg
var testCmyk []byte

func init() {
	jpegli.Init()
}

func TestDecode(t *testing.T) {
	img, err := jpegli.Decode(bytes.NewReader(testJpg))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpeg.Encode(w, img, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestDecodeGray(t *testing.T) {
	img, err := jpegli.Decode(bytes.NewReader(testGray))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpeg.Encode(w, img, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestDecodeRGBA(t *testing.T) {
	img, err := jpegli.Decode(bytes.NewReader(testRgba))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpeg.Encode(w, img, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestDecodeCMYK(t *testing.T) {
	img, err := jpegli.Decode(bytes.NewReader(testCmyk))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpegli.Encode(w, img)
	if err != nil {
		t.Error(err)
	}
}

func TestDecodeConfig(t *testing.T) {
	cfg, err := jpegli.DecodeConfig(bytes.NewReader(testJpg))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ColorModel != color.YCbCrModel {
		t.Errorf("color: got %d, want %d", cfg.ColorModel, color.YCbCrModel)
	}

	if cfg.Width != 512 {
		t.Errorf("width: got %d, want %d", cfg.Width, 512)
	}

	if cfg.Height != 512 {
		t.Errorf("height: got %d, want %d", cfg.Height, 512)
	}
}

func TestDecodeWithOptions(t *testing.T) {
	scaleSize := 256

	img, err := jpegli.DecodeWithOptions(bytes.NewReader(testJpg), &jpegli.DecodingOptions{
		ScaleTarget:     image.Rect(0, 0, scaleSize, scaleSize),
		FancyUpsampling: true,
		BlockSmoothing:  true,
		DCTMethod:       jpegli.DCTIFast,
	})
	if err != nil {
		t.Fatal(err)
	}

	if img.ColorModel() != color.RGBAModel {
		t.Errorf("color: got %d, want %d", img.ColorModel(), color.RGBAModel)
	}

	if img.Bounds().Dx() != scaleSize {
		t.Errorf("width: got %d, want %d", img.Bounds().Dx(), scaleSize)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpeg.Encode(w, img, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestEncodeGray(t *testing.T) {
	img, err := jpeg.Decode(bytes.NewReader(testGray))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpegli.Encode(w, img)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEncodeRGBA(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(testPng))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpegli.Encode(w, img)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEncodeYCbCr(t *testing.T) {
	img, err := jpeg.Decode(bytes.NewReader(testJpg))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpegli.Encode(w, img)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEncodeCMYK(t *testing.T) {
	img, err := jpegli.Decode(bytes.NewReader(testCmyk))
	if err != nil {
		t.Fatal(err)
	}

	w, err := writeCloser()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = jpegli.Encode(w, img)
	if err != nil {
		t.Fatal(err)
	}
}

func makeYCbCr(w, h int, ratio image.YCbCrSubsampleRatio) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, w, h), ratio)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Y[img.YOffset(x, y)] = uint8((x*7 + y*5) & 0xff)
		}
	}

	for i := range img.Cb {
		img.Cb[i] = uint8((i*3 + 40) & 0xff)
		img.Cr[i] = uint8((i*11 + 90) & 0xff)
	}

	return img
}

func stridedYCbCr(src *image.YCbCr) *image.YCbCr {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	sch := len(src.Cb) / src.CStride
	scw := src.CStride

	dst := &image.YCbCr{
		YStride:        src.YStride + 13,
		CStride:        src.CStride + 5,
		SubsampleRatio: src.SubsampleRatio,
		Rect:           src.Rect,
	}
	dst.Y = make([]byte, dst.YStride*h)
	dst.Cb = make([]byte, dst.CStride*sch)
	dst.Cr = make([]byte, dst.CStride*sch)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Y[dst.YOffset(x, y)] = src.Y[src.YOffset(x, y)]
		}
	}

	for cy := 0; cy < sch; cy++ {
		for cx := 0; cx < scw; cx++ {
			dst.Cb[cy*dst.CStride+cx] = src.Cb[cy*src.CStride+cx]
			dst.Cr[cy*dst.CStride+cx] = src.Cr[cy*src.CStride+cx]
		}
	}

	return dst
}

func TestEncodeYCbCrRoundtrip(t *testing.T) {
	ratios := []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444,
		image.YCbCrSubsampleRatio422,
		image.YCbCrSubsampleRatio420,
		image.YCbCrSubsampleRatio440,
	}

	for _, ratio := range ratios {
		ratio := ratio
		t.Run(ratio.String(), func(t *testing.T) {
			src := makeYCbCr(17, 19, ratio)

			var buf bytes.Buffer
			err := jpegli.Encode(&buf, src, &jpegli.EncodingOptions{Quality: 95, ChromaSubsampling: ratio})
			if err != nil {
				t.Fatal(err)
			}

			out, err := jpeg.Decode(&buf)
			if err != nil {
				t.Fatal(err)
			}

			if out.Bounds() != src.Bounds() {
				t.Fatalf("bounds: got %v, want %v", out.Bounds(), src.Bounds())
			}

			dec := out.(*image.YCbCr)
			var sum, n int
			for y := 0; y < 19; y++ {
				for x := 0; x < 17; x++ {
					d := int(dec.Y[dec.YOffset(x, y)]) - int(src.Y[src.YOffset(x, y)])
					if d < 0 {
						d = -d
					}
					sum += d
					n++
				}
			}
			if mean := float64(sum) / float64(n); mean > 12 {
				t.Errorf("mean Y diff too high: %.2f", mean)
			}
		})
	}
}

func TestEncodeYCbCrStrided(t *testing.T) {
	ratios := []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444,
		image.YCbCrSubsampleRatio422,
		image.YCbCrSubsampleRatio420,
		image.YCbCrSubsampleRatio440,
	}

	for _, ratio := range ratios {
		ratio := ratio
		t.Run(ratio.String(), func(t *testing.T) {
			src := makeYCbCr(23, 21, ratio)
			strided := stridedYCbCr(src)

			var a, b bytes.Buffer
			if err := jpegli.Encode(&a, src, &jpegli.EncodingOptions{Quality: 90, ChromaSubsampling: ratio}); err != nil {
				t.Fatal(err)
			}
			if err := jpegli.Encode(&b, strided, &jpegli.EncodingOptions{Quality: 90, ChromaSubsampling: ratio}); err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(a.Bytes(), b.Bytes()) {
				t.Errorf("strided encode differs from contiguous: %d vs %d bytes", a.Len(), b.Len())
			}
		})
	}
}

func TestEncodeSync(t *testing.T) {
	wg := sync.WaitGroup{}
	ch := make(chan bool, 2)

	img, err := jpeg.Decode(bytes.NewReader(testJpg))
	if err != nil {
		t.Error(err)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			ch <- true
			defer func() { <-ch; wg.Done() }()

			err = jpegli.Encode(io.Discard, img, nil)
			if err != nil {
				t.Error(err)
			}
		}()
	}

	wg.Wait()
}

func BenchmarkDecodeStd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := jpeg.Decode(bytes.NewReader(testJpg))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := jpegli.Decode(bytes.NewReader(testJpg))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeStd(b *testing.B) {
	img, err := jpeg.Decode(bytes.NewReader(testJpg))
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		err := jpeg.Encode(io.Discard, img, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	img, err := jpeg.Decode(bytes.NewReader(testJpg))
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		err := jpegli.Encode(io.Discard, img, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRGBAStd(b *testing.B) {
	img, err := png.Decode(bytes.NewReader(testPng))
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		err := jpeg.Encode(io.Discard, img, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRGBA(b *testing.B) {
	img, err := png.Decode(bytes.NewReader(testPng))
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		err := jpegli.Encode(io.Discard, img, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

type discard struct{}

func (d discard) Close() error {
	return nil
}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}

var discardCloser io.WriteCloser = discard{}

func writeCloser(s ...string) (io.WriteCloser, error) {
	if len(s) > 0 {
		f, err := os.Create(s[0])
		if err != nil {
			return nil, err
		}

		return f, nil
	}

	return discardCloser, nil
}
