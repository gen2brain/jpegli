//go:build wasm2go || arm64

package jpegli

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
)

const (
	jcsGrayscale = iota + 1
	jcsRGB
	jcsYCbCr
	jcsCMYK
	jcsYCCK
)

func decode(r io.Reader, configOnly, fancyUpsampling, blockSmoothing, arithCode bool, dctMethod DCTMethod, tw, th int) (img image.Image, cfg image.Config, err error) {
	mod := newModule()

	defer func() {
		if e := recover(); e != nil {
			if _, ok := e.(procExit); ok {
				img, err = nil, ErrDecode
				return
			}
			panic(e)
		}
	}()

	var data []byte

	if configOnly {
		data, err = io.ReadAll(io.LimitReader(r, 1024))
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

	inPtr := mod.Xmalloc(int32(inSize))
	if inPtr == 0 {
		return nil, cfg, ErrMemAlloc
	}
	defer mod.Xfree(inPtr)

	ok := mod.write(inPtr, data)
	if !ok {
		return nil, cfg, ErrMemWrite
	}

	ptr := mod.Xmalloc(4 * 4)
	if ptr == 0 {
		return nil, cfg, ErrMemAlloc
	}
	defer mod.Xfree(ptr)

	widthPtr := ptr
	heightPtr := ptr + 4
	colorspacePtr := ptr + 8
	chromaPtr := ptr + 12

	fancyUpsamplingVal := int32(0)
	if fancyUpsampling {
		fancyUpsamplingVal = 1
	}

	blockSmoothingVal := int32(0)
	if blockSmoothing {
		blockSmoothingVal = 1
	}

	arithCodeVal := int32(0)
	if arithCode {
		arithCodeVal = 1
	}

	res := mod.Xdecode(inPtr, int32(inSize), 1, widthPtr, heightPtr, colorspacePtr, chromaPtr, 0,
		fancyUpsamplingVal, blockSmoothingVal, arithCodeVal, int32(dctMethod), int32(tw), int32(th))
	if res == 0 {
		return nil, cfg, ErrDecode
	}

	width, ok := mod.readUint32(widthPtr)
	if !ok {
		return nil, cfg, ErrMemRead
	}

	height, ok := mod.readUint32(heightPtr)
	if !ok {
		return nil, cfg, ErrMemRead
	}

	colorspace, ok := mod.readUint32(colorspacePtr)
	if !ok {
		return nil, cfg, ErrMemRead
	}

	chroma, ok := mod.readUint32(chromaPtr)
	if !ok {
		return nil, cfg, ErrMemRead
	}

	cfg.Width = int(width)
	cfg.Height = int(height)

	if err = checkDimensions(cfg.Width, cfg.Height, inSize, configOnly); err != nil {
		return nil, cfg, err
	}

	var w, h, cw, ch int
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
		w, h, cw, ch = yCbCrSize(image.Rect(0, 0, alignm(cfg.Width), alignm(cfg.Height)), image.YCbCrSubsampleRatio(chroma))
		i0 = w*h + 0*cw*ch
		i1 = w*h + 1*cw*ch
		i2 = w*h + 2*cw*ch
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

	outPtr := mod.Xmalloc(int32(size))
	if outPtr == 0 {
		return nil, cfg, ErrMemAlloc
	}
	defer mod.Xfree(outPtr)

	res = mod.Xdecode(inPtr, int32(inSize), 0, widthPtr, heightPtr, colorspacePtr, chromaPtr, outPtr,
		fancyUpsamplingVal, blockSmoothingVal, arithCodeVal, int32(dctMethod), int32(tw), int32(th))
	if res == 0 {
		return nil, cfg, ErrDecode
	}

	out, ok := mod.read(outPtr, int32(size))
	if !ok {
		return nil, cfg, ErrMemRead
	}

	img, err = buildImage(colorspace, chroma, out, cfg, w, cw, i0, i1, i2)
	if err != nil {
		return nil, cfg, err
	}

	return img, cfg, nil
}

func encode(w io.Writer, m image.Image, quality, chromaSubsampling, progressiveLevel int, optimizeCoding, adaptiveQuantization,
	standardQuantTables, fancyDownsampling bool, dctMethod DCTMethod) (err error) {

	mod := newModule()

	defer func() {
		if e := recover(); e != nil {
			if _, ok := e.(procExit); ok {
				err = ErrEncode
				return
			}
			panic(e)
		}
	}()

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
		switch img.SubsampleRatio {
		case image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422,
			image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440:
			data = packYCbCr(img)
			colorspace = jcsYCbCr
			chroma = int(img.SubsampleRatio)
		default:
			i := imageToRGBA(img)
			data = i.Pix
			colorspace = jcsRGB
		}
	default:
		i := imageToRGBA(img)
		data = i.Pix
		colorspace = jcsRGB
	}

	inPtr := mod.Xmalloc(int32(len(data)))
	defer mod.Xfree(inPtr)

	ok := mod.write(inPtr, data)
	if !ok {
		return ErrMemWrite
	}

	sizePtr := mod.Xmalloc(8)
	defer mod.Xfree(sizePtr)

	optimizeCodingVal := int32(0)
	if optimizeCoding {
		optimizeCodingVal = 1
	}

	adaptiveQuantizationVal := int32(0)
	if adaptiveQuantization {
		adaptiveQuantizationVal = 1
	}

	standardQuantTablesVal := int32(0)
	if standardQuantTables {
		standardQuantTablesVal = 1
	}

	fancyDownsamplingVal := int32(0)
	if fancyDownsampling {
		fancyDownsamplingVal = 1
	}

	outPtr := mod.Xencode(inPtr, int32(m.Bounds().Dx()), int32(m.Bounds().Dy()), int32(colorspace), int32(chroma), sizePtr, int32(quality),
		int32(progressiveLevel), optimizeCodingVal, adaptiveQuantizationVal, standardQuantTablesVal, fancyDownsamplingVal, int32(dctMethod))
	defer mod.Xfree(outPtr)

	size, ok := mod.readUint64(sizePtr)
	if !ok {
		return ErrMemRead
	}

	if size == 0 {
		return ErrEncode
	}

	out, ok := mod.read(outPtr, int32(size))
	if !ok {
		return ErrMemRead
	}

	_, err = w.Write(out)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// Init is a no-op; the wasm2go backend builds a fresh module per call.
func Init() {}

func newModule() *module {
	mod := newModuleRaw(&wasiHost{})
	mod.X_initialize()

	return mod
}

func (m *module) write(ptr int32, data []byte) bool {
	if ptr < 0 || int(ptr)+len(data) > len(m.memory) {
		return false
	}

	copy(m.memory[ptr:], data)

	return true
}

func (m *module) read(ptr, size int32) ([]byte, bool) {
	if ptr < 0 || size < 0 || int(ptr)+int(size) > len(m.memory) {
		return nil, false
	}

	return m.memory[ptr : ptr+size : ptr+size], true
}

func (m *module) readUint32(ptr int32) (uint32, bool) {
	if ptr < 0 || int(ptr)+4 > len(m.memory) {
		return 0, false
	}

	return load32(m.memory[ptr:]), true
}

func (m *module) readUint64(ptr int32) (uint64, bool) {
	if ptr < 0 || int(ptr)+8 > len(m.memory) {
		return 0, false
	}

	return load64(m.memory[ptr:]), true
}

// procExit carries the exit code of a wasi proc_exit call so Decode/Encode can
// turn an unexpected module abort into an error instead of crashing.
type procExit struct {
	code int32
}

// wasiHost implements the minimal wasi_snapshot_preview1 imports the jpegli
// module needs: jpegli only emits diagnostic messages (fd_write) and may abort
// on a fatal error (proc_exit); the remaining calls are stubs.
type wasiHost struct {
	mod *module
}

// Init is called by the generated newModuleRaw with the freshly created module so the
// host can reach its linear memory.
func (h *wasiHost) Init(m any) {
	h.mod = m.(*module)
}

func (h *wasiHost) Xfd_close(fd int32) int32 {
	return 0
}

func (h *wasiHost) Xfd_seek(fd int32, offset int64, whence, retPtr int32) int32 {
	return 0
}

func (h *wasiHost) Xfd_write(fd, iovs, iovsLen, nwrittenPtr int32) int32 {
	mem := h.mod.memory

	var dst *os.File
	switch fd {
	case 1:
		dst = os.Stdout
	case 2:
		dst = os.Stderr
	}

	var written uint32
	for i := int32(0); i < iovsLen; i++ {
		ptr := load32(mem[iovs+i*8:])
		length := load32(mem[iovs+i*8+4:])
		if length != 0 && dst != nil {
			dst.Write(mem[ptr : ptr+length])
		}
		written += length
	}

	store32(mem[nwrittenPtr:], written)

	return 0
}

func (h *wasiHost) Xproc_exit(code int32) {
	panic(procExit{code})
}
