#!/usr/bin/env swift
//
// decompress-lz4: Decompress Apple Compression framework LZ4 data
//
// This helper decompresses LZ4-compressed data created by Apple's Compression
// framework (as used by Tart). The format uses Apple's proprietary bv41 wrapper
// around LZ4 blocks with dictionary chaining.
//
// Usage: decompress-lz4 <input-file> <output-file>
//

import Foundation
import Compression

func decompress(inputPath: String, outputPath: String) throws {
    // Read compressed data
    let inputURL = URL(fileURLWithPath: inputPath)
    let compressedData = try Data(contentsOf: inputURL, options: [.mappedIfSafe])

    // Decompress using Apple's Compression framework
    // This handles the bv41 format and dictionary chaining automatically
    let decompressed = try (compressedData as NSData).decompressed(using: .lz4) as Data

    // Write decompressed data
    let outputURL = URL(fileURLWithPath: outputPath)
    try (decompressed as Data).write(to: outputURL, options: [.atomic])
}

// Main
guard CommandLine.arguments.count == 3 else {
    fputs("Usage: decompress-lz4 <input-file> <output-file>\n", stderr)
    exit(1)
}

let inputPath = CommandLine.arguments[1]
let outputPath = CommandLine.arguments[2]

do {
    try decompress(inputPath: inputPath, outputPath: outputPath)
} catch {
    fputs("Error: \(error.localizedDescription)\n", stderr)
    exit(1)
}
