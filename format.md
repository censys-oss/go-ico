# file format

This document describes the structure of the .ICO format (commonly stored in `.ico` files). An ICO file is a wrapper format that can hold one or more BMP/PNG images of varying parameters.

<img src="rc.png">

\- raymond chen

## File Structure

A ico file has three main sections:

1. The **icon directory header** (`ICONDIR`), which specifies that the file is an icon and how many images it contains. 6 bytes.
2. One or more **icon directory entries** (`ICONDIRENTRY`), each describing a sub-image.  16 bytes.
3. The actual **image data** for each sub-image (in BMP or PNG format), at the end of the file.

Conceptually, the structure looks like this:

```c 
typedef struct ICONDIR {
    WORD          idReserved;
    WORD          idType;
    WORD          idCount;
    ICONDIRENTRY  idEntries[];
    [...png/bmp images]
} ICONHEADER;
```

```c
struct ICONDIRENTRY {
    BYTE  bWidth;
    BYTE  bHeight;
    BYTE  bColorCount;
    BYTE  bReserved;
    WORD  wPlanes;
    WORD  wBitCount;
    DWORD dwBytesInRes;
    DWORD dwImageOffset;
};
```


### Icon Directory (ICONDIR)

The **ICONDIR** structure is a fixed-length header occupying 6 bytes:

| Offset | Size | Field       | Description                                               |
|-------:|-----:|------------|-----------------------------------------------------------|
| 0      | 2    | `idReserved`| Reserved. Must be 0                                     |
| 2      | 2    | `idType`    | Resource type. 1 for icons (`.ico`), 2 for cursors (unimplemented, but extremely similar) |
| 4      | 2    | `idCount`   | Number of images (icon directory entries) in the file   |

- **idReserved**: Always 0 for `.ico`
- **idType**: Set to 1 for `.ico`
- **idCount**: Indicates how many icon images (sub-icons) follow in the file.

### Icon Directory Entry (ICONDIRENTRY)

Each **ICONDIRENTRY** describes one sub-image stored in the file. The structure is 16 bytes in size:

| Offset | Size | Field          | Description                                                                                           |
|-------:|-----:|---------------|-------------------------------------------------------------------------------------------------------|
| 0      | 1    | `bWidth`       | Width of the image in pixels. If this is 0, it implies 256 pixels                                    |
| 1      | 1    | `bHeight`      | Height of the image in pixels. If this is 0, it implies 256 pixels                                   |
| 2      | 1    | `bColorCount`  | Number of colors in the palette. 0 if the image has 8 bits per pixel or more               |
| 3      | 1    | `bReserved`    | Reserved                                 |
| 4      | 2    | `wPlanes`      | Number of color planes (basically channels, though one plane may map to multiple channels)                                                                    |
| 6      | 2    | `wBitCount`    | Bits per pixel                                                                                       |
| 8      | 4    | `dwBytesInRes` | Size of the image data in bytes (including headers)                                              |
| 12     | 4    | `dwImageOffset`| Offset from the beginning of the file to the start of this sub-image's data                          |

### Image Data

Following the directory entries, the file contains the raw image data for each sub-image, placed at the offsets specified by each entry’s `dwImageOffset`. This data can be:

- A bitmap data block (sometimes called DIB or “device-independent bitmap” data), or
- A **PNG** data block.

## BMP vs. PNG Image Data

BMP and PNG are differentiated by the 8 byte PNG magic at the beginning of the image data.

### BMP (DIB) Format Inside ICO

When the image is stored as a bmp, it is decoded as such and `AND` masks are applied.

In 32 bpp icons, transparency may be controlled in the 4th byte rather as well as the mask [see here](https://devblogs.microsoft.com/oldnewthing/20101021-00/?p=12483).

### PNG Format Inside ICO

When the image is stored as a PNG:

1. The data begins with the standard PNG signature `\x89PNG\r\n\x1A\n`.
2. It is parsed like a normal png

## References

- [bmp](https://learn.microsoft.com/en-us/windows/win32/gdi/bitmap-storage)
- [wikipedia ico format](https://en.wikipedia.org/wiki/ICO_(file_format))
- [raymond chen blog acting as docs for .ico](https://web.archive.org/web/20150226170659/http://blogs.msdn.com/b/oldnewthing/archive/2010/10/18/10077133.aspx)