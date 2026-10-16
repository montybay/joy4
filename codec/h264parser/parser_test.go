package h264parser

import (
	"encoding/hex"
	"reflect"
	"testing"
)

func TestParser(t *testing.T) {
	var typ int
	var nalus [][]byte

	annexbFrame, _ := hex.DecodeString("00000001223322330000000122332233223300000133000001000001")
	nalus, typ = SplitNALUs(annexbFrame)
	if typ != NALU_ANNEXB || !reflect.DeepEqual(nalus, [][]byte{{0x22, 0x33, 0x22, 0x33}, {0x22, 0x33, 0x22, 0x33, 0x22, 0x33}, {0x33}}) {
		t.Fatalf("Annex B: type=%d, nalus=%x", typ, nalus)
	}

	avccFrame, _ := hex.DecodeString(
		"00000008aabbccaabbccaabb00000001aa",
	)
	nalus, typ = SplitNALUs(avccFrame)
	if typ != NALU_AVCC || !reflect.DeepEqual(nalus, [][]byte{{0xaa, 0xbb, 0xcc, 0xaa, 0xbb, 0xcc, 0xaa, 0xbb}, {0xaa}}) {
		t.Fatalf("AVCC: type=%d, nalus=%x", typ, nalus)
	}
}
