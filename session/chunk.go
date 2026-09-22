package session

import "github.com/sandertv/gophertunnel/minecraft/protocol/packet"

// The variables below are automatically generated to save runtime performance when sending empty chunks, and represents
// the full payload required to send an empty chunk in a specific dimension.
var (
	emptyChunksOverworld = []byte{1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0}
	emptyChunksNether    = []byte{1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0}
	emptyChunksEnd       = []byte{1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0}
)

// emptyChunk returns the correct data to send an empty chunk in a specific dimension.
func emptyChunk(dimension int32) []byte {
	if dimension == packet.DimensionNether {
		return emptyChunksNether
	} else if dimension == packet.DimensionEnd {
		return emptyChunksEnd
	}
	return emptyChunksOverworld
}
