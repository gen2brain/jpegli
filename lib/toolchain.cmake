include(/opt/wasi-sdk/share/cmake/wasi-sdk-p1.cmake)

# jpegli's public headers include <setjmp.h>, guarded by the wasi-sysroot behind
# __wasm_exception_handling__. jpegli never calls setjmp/longjmp, so we only need
# the type declarations; define the macro to satisfy the header. -msimd128 keeps
# the SIMD (highway) paths the wazero (amd64) backend executes; the wasm2go
# backend can't ingest SIMD, so USE_SIMD is turned off for its scalar build.
option(USE_SIMD "build the SIMD (highway) paths" ON)

set(EXTRA_FLAGS "-D__wasm_exception_handling__")
if(USE_SIMD)
	set(EXTRA_FLAGS "${EXTRA_FLAGS} -msimd128")
endif()

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} ${EXTRA_FLAGS}")
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${EXTRA_FLAGS}")
