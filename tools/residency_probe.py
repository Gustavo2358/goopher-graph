#!/usr/bin/env python3
"""Linux benchmark-only mincore observation; mmap does not read file pages.

Uses libc through Python's stdlib ctypes, without adding unsafe/cgo to Go.
Run on a private, owned benchmark copy: kernel permissions affect mincore data.
"""
import ctypes
import json
import os
import sys


def residency(path):
    if sys.platform != "linux" or ctypes.sizeof(ctypes.c_void_p) != 8:
        raise RuntimeError("Linux/64-bit required")
    lib = ctypes.CDLL(None, use_errno=True)
    lib.mmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_int,
                         ctypes.c_int, ctypes.c_int, ctypes.c_long]
    lib.mmap.restype = ctypes.c_void_p
    lib.mincore.argtypes = [ctypes.c_void_p, ctypes.c_size_t,
                            ctypes.POINTER(ctypes.c_ubyte)]
    lib.munmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
    fd = os.open(path, os.O_RDONLY)
    try:
        size = os.fstat(fd).st_size
        if size <= 0:
            raise ValueError("nonempty file required")
        address = lib.mmap(None, size, 1, 2, fd, 0)  # PROT_READ, MAP_PRIVATE
        if address == ctypes.c_void_p(-1).value:
            raise OSError(ctypes.get_errno(), "mmap")
        try:
            pages = (size + os.sysconf("SC_PAGE_SIZE") - 1) // os.sysconf("SC_PAGE_SIZE")
            vector = (ctypes.c_ubyte * pages)()
            if lib.mincore(address, size, vector) != 0:
                raise OSError(ctypes.get_errno(), "mincore")
            return dict(pages=pages, resident_pages=sum(v & 1 for v in vector), size=size)
        finally:
            if lib.munmap(address, size) != 0:
                raise OSError(ctypes.get_errno(), "munmap")
    finally:
        os.close(fd)


if __name__ == "__main__":
    print(json.dumps(residency(sys.argv[1])))
