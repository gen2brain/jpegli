package jpegli

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"sync"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed lib/jpegli.wasm.gz
var jpegliWasm []byte

const (
	jcsGrayscale = iota + 1
	jcsRGB
	jcsYCbCr
	jcsCMYK
	jcsYCCK
)

func decode(r io.Reader, configOnly, fancyUpsampling, blockSmoothing, arithCode bool, dctMethod DCTMethod, tw, th int) (image.Image, image.Config, error) {
	initializeOnce()

	var err error
	var cfg image.Config
	var data []byte

	if configOnly {
		data = make([]byte, 1024)
		_, err = r.Read(data)
		if err != nil {
			return nil, cfg, fmt.Errorf("read: %w", err)
		}
	} else {
		data, err = io.ReadAll(r)
		if err != nil {
			return nil, cfg, fmt.Errorf("read: %w", err)
		}
	}

	inSize := len(data)
	ctx := context.Background()

	res, err := _alloc.Call(ctx, uint64(inSize))
	if err != nil {
		return nil, cfg, fmt.Errorf("alloc: %w", err)
	}
	inPtr := res[0]
	defer _free.Call(ctx, inPtr)

	ok := mod.Memory().Write(uint32(inPtr), data)
	if !ok {
		return nil, cfg, ErrMemWrite
	}

	res, err = _alloc.Call(ctx, 4*4)
	if err != nil {
		return nil, cfg, fmt.Errorf("alloc: %w", err)
	}
	defer _free.Call(ctx, res[0])

	widthPtr := res[0]
	heightPtr := res[0] + 4
	colorspacePtr := res[0] + 8
	chromaPtr := res[0] + 12

	fancyUpsamplingVal := 0
	if fancyUpsampling {
		fancyUpsamplingVal = 1
	}

	blockSmoothingVal := 0
	if blockSmoothing {
		blockSmoothingVal = 1
	}

	arithCodeVal := 0
	if arithCode {
		arithCodeVal = 1
	}

	res, err = _decode.Call(ctx, inPtr, uint64(inSize), 1, widthPtr, heightPtr, colorspacePtr, chromaPtr, 0,
		uint64(fancyUpsamplingVal), uint64(blockSmoothingVal), uint64(arithCodeVal), uint64(dctMethod), uint64(tw), uint64(th))
	if err != nil {
		return nil, cfg, fmt.Errorf("decode: %w", err)
	}

	if res[0] == 0 {
		return nil, cfg, ErrDecode
	}

	width, ok := mod.Memory().ReadUint32Le(uint32(widthPtr))
	if !ok {
		return nil, cfg, ErrMemRead
	}

	height, ok := mod.Memory().ReadUint32Le(uint32(heightPtr))
	if !ok {
		return nil, cfg, ErrMemRead
	}

	colorspace, ok := mod.Memory().ReadUint32Le(uint32(colorspacePtr))
	if !ok {
		return nil, cfg, ErrMemRead
	}

	chroma, ok := mod.Memory().ReadUint32Le(uint32(chromaPtr))
	if !ok {
		return nil, cfg, ErrMemRead
	}

	cfg.Width = int(width)
	cfg.Height = int(height)

	var w, cw int
	var size, i0, i1, i2 int

	switch colorspace {
	case jcsGrayscale:
		cfg.ColorModel = color.GrayModel
		size = alignm(cfg.Width) * alignm(cfg.Height) * 1
	case jcsRGB:
		cfg.ColorModel = color.RGBAModel
		size = cfg.Width * cfg.Height * 4
	case jcsYCbCr:
		cfg.ColorModel = color.YCbCrModel
		w, _, cw, _ = yCbCrSize(image.Rect(0, 0, cfg.Width, cfg.Height), image.YCbCrSubsampleRatio(chroma))
		aw, ah, acw, ach := yCbCrSize(image.Rect(0, 0, alignm(cfg.Width), alignm(cfg.Height)), image.YCbCrSubsampleRatio(chroma))
		i0 = aw*ah + 0*acw*ach
		i1 = aw*ah + 1*acw*ach
		i2 = aw*ah + 2*acw*ach
		size = i2
	case jcsCMYK, jcsYCCK:
		cfg.ColorModel = color.CMYKModel
		size = cfg.Width * cfg.Height * 4
	default:
		return nil, cfg, fmt.Errorf("unsupported colorspace %d", colorspace)
	}

	if configOnly {
		return nil, cfg, nil
	}

	res, err = _alloc.Call(ctx, uint64(size))
	if err != nil {
		return nil, cfg, fmt.Errorf("alloc: %w", err)
	}
	outPtr := res[0]
	defer _free.Call(ctx, outPtr)

	res, err = _decode.Call(ctx, inPtr, uint64(inSize), 0, widthPtr, heightPtr, colorspacePtr, chromaPtr, outPtr,
		uint64(fancyUpsamplingVal), uint64(blockSmoothingVal), uint64(arithCodeVal), uint64(dctMethod), uint64(tw), uint64(th))
	if err != nil {
		return nil, cfg, fmt.Errorf("decode: %w", err)
	}

	if res[0] == 0 {
		return nil, cfg, ErrDecode
	}

	out, ok := mod.Memory().Read(uint32(outPtr), uint32(size))
	if !ok {
		return nil, cfg, ErrMemRead
	}

	var img image.Image

	switch colorspace {
	case jcsGrayscale:
		i := image.NewGray(image.Rect(0, 0, cfg.Width, cfg.Height))
		i.Pix = out
		img = i
	case jcsRGB:
		i := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
		i.Pix = out
		img = i
	case jcsYCbCr:
		img = &image.YCbCr{
			Y:              out[:i0:i0],
			Cb:             out[i0:i1:i1],
			Cr:             out[i1:i2:i2],
			SubsampleRatio: image.YCbCrSubsampleRatio(chroma),
			YStride:        w,
			CStride:        cw,
			Rect:           image.Rect(0, 0, cfg.Width, cfg.Height),
		}
	case jcsCMYK, jcsYCCK:
		i := image.NewCMYK(image.Rect(0, 0, cfg.Width, cfg.Height))
		i.Pix = out
		img = i
	default:
		return nil, cfg, fmt.Errorf("unsupported colorspace %d", colorspace)
	}

	return img, cfg, nil
}

func encode(w io.Writer, m image.Image, quality, chromaSubsampling, progressiveLevel int, optimizeCoding, adaptiveQuantization,
	standardQuantTables, fancyDownsampling bool, dctMethod DCTMethod) error {

	initializeOnce()

	var data []byte
	var colorspace int
	var chroma int

	switch img := m.(type) {
	case *image.Gray:
		data = img.Pix
		colorspace = jcsGrayscale
	case *image.RGBA:
		data = img.Pix
		colorspace = jcsRGB
		chroma = chromaSubsampling
	case *image.NRGBA:
		data = img.Pix
		colorspace = jcsRGB
		chroma = chromaSubsampling
	case *image.CMYK:
		data = img.Pix
		colorspace = jcsCMYK
	case *image.YCbCr:
		length := len(img.Y) + len(img.Cb) + len(img.Cr)
		var b = struct {
			addr *uint8
			len  int
			cap  int
		}{&img.Y[0], length, length}
		data = *(*[]byte)(unsafe.Pointer(&b))
		colorspace = jcsYCbCr
		chroma = int(img.SubsampleRatio)
	default:
		i := imageToRGBA(img)
		data = i.Pix
		colorspace = jcsRGB
	}

	ctx := context.Background()

	res, err := _alloc.Call(ctx, uint64(len(data)))
	if err != nil {
		return fmt.Errorf("alloc: %w", err)
	}
	inPtr := res[0]
	defer _free.Call(ctx, inPtr)

	ok := mod.Memory().Write(uint32(inPtr), data)
	if !ok {
		return ErrMemWrite
	}

	res, err = _alloc.Call(ctx, 8)
	if err != nil {
		return fmt.Errorf("alloc: %w", err)
	}
	sizePtr := res[0]
	defer _free.Call(ctx, sizePtr)

	optimizeCodingVal := 0
	if optimizeCoding {
		optimizeCodingVal = 1
	}

	adaptiveQuantizationVal := 0
	if adaptiveQuantization {
		adaptiveQuantizationVal = 1
	}

	standardQuantTablesVal := 0
	if standardQuantTables {
		standardQuantTablesVal = 1
	}

	fancyDownsamplingVal := 0
	if fancyDownsampling {
		fancyDownsamplingVal = 1
	}

	res, err = _encode.Call(ctx, inPtr, uint64(m.Bounds().Dx()), uint64(m.Bounds().Dy()), uint64(colorspace), uint64(chroma), sizePtr, uint64(quality),
		uint64(progressiveLevel), uint64(optimizeCodingVal), uint64(adaptiveQuantizationVal), uint64(standardQuantTablesVal), uint64(fancyDownsamplingVal), uint64(dctMethod))
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	defer _free.Call(ctx, res[0])

	size, ok := mod.Memory().ReadUint64Le(uint32(sizePtr))
	if !ok {
		return ErrMemRead
	}

	if size == 0 {
		return ErrEncode
	}

	out, ok := mod.Memory().Read(uint32(res[0]), uint32(size))
	if !ok {
		return ErrMemRead
	}

	_, err = w.Write(out)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

var (
	mod api.Module

	_alloc  api.Function
	_free   api.Function
	_decode api.Function
	_encode api.Function

	initializeOnce = sync.OnceFunc(initialize)
)

func initialize() {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)

	r, err := gzip.NewReader(bytes.NewReader(jpegliWasm))
	if err != nil {
		panic(err)
	}
	defer r.Close()

	var data bytes.Buffer
	_, err = data.ReadFrom(r)
	if err != nil {
		panic(err)
	}

	compiled, err := rt.CompileModule(ctx, data.Bytes())
	if err != nil {
		panic(err)
	}

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	mod, err = rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithStderr(os.Stderr).WithStdout(os.Stdout))
	if err != nil {
		panic(err)
	}

	_alloc = mod.ExportedFunction("malloc")
	_free = mod.ExportedFunction("free")
	_decode = mod.ExportedFunction("decode")
	_encode = mod.ExportedFunction("encode")
}
