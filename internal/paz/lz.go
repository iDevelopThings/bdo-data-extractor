package paz

import "github.com/idevelopthings/bdo-data-extractor/internal/bss"

// literalLength maps the low nibble of the group header to a literal run length.
var litLenTable = [16]int{4, 0, 1, 0, 2, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1, 0}

// parseFileHeader returns (decompLen, compLen, headerSize).
func parseFileHeader(data []byte) (int, int, int) {
	if data[0]&0x02 != 0 {
		compLen := int(bss.U32(data, 1))
		decompLen := int(bss.U32(data, 5))
		return decompLen, compLen, 9
	}
	return int(data[2]), int(data[1]), 3
}

// parseBlockHeader returns (dist, length, stepBytes).
func parseBlockHeader(h uint32) (int, int, int) {
	switch h & 0x03 {
	case 0x03:
		if (h & 0x7F) == 3 {
			return int(h >> 15), int(((h >> 7) & 0xFF) + 3), 4
		}
		return int((h >> 7) & 0x1FFFF), int(((h >> 2) & 0x1F) + 2), 3
	case 0x02:
		return int((h & 0xFFFF) >> 6), int(((h >> 2) & 0xF) + 3), 2
	case 0x01:
		return int((h & 0xFFFF) >> 2), 3, 2
	default:
		return int((h & 0xFF) >> 2), 3, 1
	}
}

// Decompress runs BDO's custom LZ over a container blob (flags byte at data[0]).
func Decompress(data []byte, originalSize int) []byte {
	if len(data) == 0 {
		return nil
	}
	flags := data[0]
	target, compLen, headerSize := parseFileHeader(data)
	if len(data) < compLen {
		compLen = len(data)
	}
	data = data[:compLen]

	if flags&0x01 == 0 { // stored
		end := headerSize + target
		if end > len(data) {
			end = len(data)
		}
		return append([]byte(nil), data[headerSize:end]...)
	}

	out := make([]byte, target)
	inIdx, outIdx := headerSize, 0
	groupHeader := uint32(1)
	n := len(data)

	for outIdx < target && inIdx < n {
		if groupHeader == 1 {
			if inIdx+4 > n {
				break
			}
			groupHeader = bss.U32(data, inIdx)
			inIdx += 4
		}
		if groupHeader&1 != 0 {
			if inIdx+4 > n {
				break
			}
			h := bss.U32(data, inIdx)
			dist, length, step := parseBlockHeader(h)
			inIdx += step
			if outIdx < dist || outIdx+length > target {
				break // corrupt match; stop gracefully
			}
			src := outIdx - dist
			for k := 0; k < length; k++ { // overlapping LZ77 copy
				out[outIdx+k] = out[src+k]
			}
			outIdx += length
			groupHeader >>= 1
		} else {
			litLen := litLenTable[groupHeader&0xF]
			if outIdx+4 > target || inIdx+4 > n {
				break
			}
			copy(out[outIdx:outIdx+4], data[inIdx:inIdx+4])
			outIdx += litLen
			inIdx += litLen
			groupHeader >>= uint(litLen)
		}
	}

	// tail
	for outIdx < target {
		if groupHeader == 1 {
			if inIdx+4 <= n {
				inIdx += 4
			}
			groupHeader = 0x80000000
		}
		if inIdx >= n {
			break
		}
		out[outIdx] = data[inIdx]
		outIdx++
		inIdx++
		groupHeader >>= 1
	}
	return out[:outIdx]
}
