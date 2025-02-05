go-ico
======

A library for parsing and working with `.ico` image files. Compatible with Go’s standard `image` library. Handles potentially adversarial images without panicking and has a decode option for limiting memory.

If you have an ICO that works with a different library but fails to decode with this one, please open a ticket and upload that ICO.

## Installation

```
go get github.com/censys-oss/go-ico
```

## Dependencies

There is a single dependency on [github.com/jsummers/gobmp](http://github.com/jsummers/gobmp), a library for working with `.bmp` files in Go.
There is no builtin support for `.bmp` in the `image` package, there is an experimental library in `image/x/bmp` but it is not very good.

## Usage

```
reader, err := os.Open("example.ico")
if err != nil {
        log.Fatal(err)
}
defer reader.Close()

// To decode and return all images
images, err := Decode(r)  // images is of []image.Image type
if err != nil {
        log.Fatal(err)
}

// To decode, with memory limits
image, err := Decode(r, WithMemoryLimit(10_000_000))
if err != nil {
        log.Fatal(err)
}

// 
```
