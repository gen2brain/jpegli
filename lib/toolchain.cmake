include(/opt/wasi-sdk/share/cmake/wasi-sdk-p1.cmake)

# jpegli's public headers include <setjmp.h>, guarded by the wasi-sysroot behind
# __wasm_exception_handling__. jpegli never calls setjmp/longjmp, so we only need
# the type declarations; define the macro to satisfy the header. -msimd128 keeps
# the SIMD (highway) paths the wazero runtime can execute.
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -D__wasm_exception_handling__ -msimd128")
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -D__wasm_exception_handling__ -msimd128")
