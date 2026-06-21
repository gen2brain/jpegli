JPEGLI_VERSION = main

JPEGLI_SRC = $(PWD)/jpegli
JPEGLI_BUILD = $(JPEGLI_SRC)/build

WASI_SDK_PATH = /opt/wasi-sdk
export CC = $(WASI_SDK_PATH)/bin/clang --sysroot=$(WASI_SDK_PATH)/share/wasi-sysroot
export CXX = $(WASI_SDK_PATH)/bin/clang++ --sysroot=$(WASI_SDK_PATH)/share/wasi-sysroot

CMAKE_TOOLCHAIN_FILE=$(PWD)/toolchain.cmake

BIN := jpegli.wasm

all: $(BIN)

$(JPEGLI_SRC):
	git clone -b $(JPEGLI_VERSION) --depth 1 --recursive --jobs `nproc` https://github.com/google/jpegli
	mkdir -p $(JPEGLI_BUILD)
	test -d $@

$(JPEGLI_BUILD)/lib/libjpegli-static.a: $(JPEGLI_SRC)
	cd $(JPEGLI_BUILD); \
	cmake $(JPEGLI_SRC) \
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

	cd $(JPEGLI_BUILD); \
	$(MAKE) -j$(shell nproc) jpegli-static

$(BIN): $(JPEGLI_BUILD)/lib/libjpegli-static.a
	$(CXX) \
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
		-I ${JPEGLI_SRC} \
		-I ${JPEGLI_SRC}/lib \
		-I ${JPEGLI_BUILD}/lib/include/jpegli \
		-o $@ \
		-Wall \
		-x c++ jpegli.c -x none \
		${JPEGLI_BUILD}/lib/libjpegli-static.a \
		${JPEGLI_BUILD}/third_party/highway/libhwy.a

.PHONY: clean

clean:
	-rm -rf $(JPEGLI_SRC)
