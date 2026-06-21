JPEGLI_VERSION = main

JPEGLI_SRC = $(PWD)/jpegli

WASI_SDK_PATH = /opt/wasi-sdk
export CC = $(WASI_SDK_PATH)/bin/clang --sysroot=$(WASI_SDK_PATH)/share/wasi-sysroot
export CXX = $(WASI_SDK_PATH)/bin/clang++ --sysroot=$(WASI_SDK_PATH)/share/wasi-sysroot

CMAKE_TOOLCHAIN_FILE = $(PWD)/toolchain.cmake

CMAKE_FLAGS = \
	-DCMAKE_BUILD_TYPE=Release \
	-DBUILD_SHARED_LIBS=0 \
	-DBUILD_TESTING=0 \
	-DJPEGLI_ENABLE_OPENEXR=0 \
	-DJPEGLI_ENABLE_TOOLS=0 \
	-DJPEGLI_ENABLE_DOXYGEN=0 \
	-DJPEGLI_ENABLE_MANPAGES=0 \
	-DJPEGLI_ENABLE_BENCHMARK=0 \
	-DJPEGLI_ENABLE_JNI=0 \
	-DJPEGLI_ENABLE_SJPEG=0 \
	-DJPEGLI_ENABLE_JPEGLI_LIBJPEG=0 \
	-DJPEGLI_ENABLE_WASM_THREADS=0 \
	-DJPEGLI_LIBJPEG_LIBRARY_SOVERSION=8 \
	-DCMAKE_TOOLCHAIN_FILE=$(CMAKE_TOOLCHAIN_FILE)

LINK_FLAGS = \
	--target=wasm32-wasip1 \
	-D__wasm_exception_handling__ \
	-O3 \
	-Wl,--no-entry \
	-Wl,--export=malloc \
	-Wl,--export=free \
	-Wl,--export=decode \
	-Wl,--export=encode \
	-Wl,--strip-debug \
	-mexec-model=reactor \
	-fno-exceptions \
	-mnontrapping-fptoint \
	-I $(JPEGLI_SRC) \
	-I $(JPEGLI_SRC)/lib \
	-Wall

# Default: SIMD wasm gzipped for the wazero (amd64) backend.
all: jpegli.wasm.gz

# Scalar wasm transpiled to Go for the wasm2go (non-amd64) backend.
wasm2go: ../libjpegli.go

$(JPEGLI_SRC):
	git clone -b $(JPEGLI_VERSION) --depth 1 --recursive --jobs `nproc` https://github.com/google/jpegli

build-simd/lib/libjpegli-static.a: | $(JPEGLI_SRC)
	mkdir -p build-simd; cd build-simd; \
	cmake $(JPEGLI_SRC) $(CMAKE_FLAGS) -DUSE_SIMD=ON; \
	$(MAKE) -j$(shell nproc) jpegli-static

build-scalar/lib/libjpegli-static.a: | $(JPEGLI_SRC)
	mkdir -p build-scalar; cd build-scalar; \
	cmake $(JPEGLI_SRC) $(CMAKE_FLAGS) -DUSE_SIMD=OFF; \
	$(MAKE) -j$(shell nproc) jpegli-static

jpegli.wasm: build-simd/lib/libjpegli-static.a
	$(CXX) $(LINK_FLAGS) -I build-simd/lib/include/jpegli -o $@ \
		-x c++ jpegli.c -x none \
		build-simd/lib/libjpegli-static.a build-simd/third_party/highway/libhwy.a

jpegli-scalar.wasm: build-scalar/lib/libjpegli-static.a
	$(CXX) $(LINK_FLAGS) -I build-scalar/lib/include/jpegli -o $@ \
		-x c++ jpegli.c -x none \
		build-scalar/lib/libjpegli-static.a build-scalar/third_party/highway/libhwy.a

jpegli.wasm.gz: jpegli.wasm
	gzip -9 -c $< > $@

../libjpegli.go: jpegli-scalar.wasm
	wasm2go -pkg jpegli -unsafe -tags '!amd64' -o $@ $<

.PHONY: all wasm2go clean

clean:
	-rm -rf $(JPEGLI_SRC) jpegli.wasm jpegli-scalar.wasm jpegli.wasm.gz
