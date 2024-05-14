## jpegli
[![Status](https://github.com/gen2brain/jpegli/actions/workflows/test.yml/badge.svg)](https://github.com/gen2brain/jpegli/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/gen2brain/jpegli.svg)](https://pkg.go.dev/github.com/gen2brain/jpegli)

Go encoder/decoder for [JPEG](https://en.wikipedia.org/wiki/JPEG).

Based on [jpegli](https://github.com/libjxl/libjxl/blob/main/lib/jpegli/README.md) from libjxl compiled to [WASM](https://en.wikipedia.org/wiki/WebAssembly) and used with [wazero](https://wazero.io/) runtime (CGo-free).

### Benchmark

```
goos: linux
goarch: amd64
pkg: github.com/gen2brain/jpegli
cpu: 11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz

BenchmarkDecodeStd-8       	     549	   2187227 ns/op	  407120 B/op	       7 allocs/op
BenchmarkDecode-8          	     554	   2172405 ns/op	  154672 B/op	      32 allocs/op

BenchmarkEncodeStd-8       	     260	   4601558 ns/op	    6109 B/op	       7 allocs/op
BenchmarkEncode-8          	     314	   3791604 ns/op	  394816 B/op	      12 allocs/op

BenchmarkEncodeRGBAStd-8   	     228	   5237803 ns/op	    9762 B/op	       8 allocs/op
BenchmarkEncodeRGBA-8      	     261	   4558648 ns/op	    4862 B/op	      13 allocs/op
```

### Resources

* https://giannirosato.com/blog/post/jpegli/
* https://cloudinary.com/blog/jpeg-xl-and-the-pareto-front
* https://github.com/google-research/google-research/tree/master/mucped23
