package aacparser

import (
	"time"
	"fmt"
	"bytes"
	"io"

	"github.com/montybay/joy4/av"
	"github.com/montybay/joy4/utils/bits"
)

// copied from libavcodec/mpeg4audio.h
const (
	_ = iota           
	AOT_AAC_MAIN        ///< Y  Main
	AOT_AAC_LC          ///< Y  Low Complexity
	AOT_AAC_SSR         ///< N  Scalable Sample Rate
	AOT_AAC_LTP         ///< Y  Long Term Prediction
	AOT_SBR             ///< Y  Spectral Band Replication
	AOT_AAC_SCALABLE    ///< N  Scalable
	AOT_TWINVQ          ///< N  Twin Vector Quantizer
	AOT_CELP            ///< N  Code Excited Linear Prediction
	AOT_HVXC            ///< N  Harmonic Vector eXcitation Coding

	_                   // 10
	_                   // 11

	AOT_TTSI            ///< N  Text-To-Speech Interface
	AOT_MAINSYNTH       ///< N  Main Synthesis
	AOT_WAVESYNTH       ///< N  Wavetable Synthesis
	AOT_MIDI            ///< N  General MIDI
	AOT_SAFX            ///< N  Algorithmic Synthesis and Audio Effects
	AOT_ER_AAC_LC       ///< N  Error Resilient Low Complexity

	_                   // 18

	AOT_ER_AAC_LTP      ///< N  Error Resilient Long Term Prediction
	AOT_ER_AAC_SCALABLE ///< N  Error Resilient Scalable
	AOT_ER_TWINVQ       ///< N  Error Resilient Twin Vector Quantizer
	AOT_ER_BSAC         ///< N  Error Resilient Bit-Sliced Arithmetic Coding
	AOT_ER_AAC_LD       ///< N  Error Resilient Low Delay
	AOT_ER_CELP         ///< N  Error Resilient Code Excited Linear Prediction
	AOT_ER_HVXC         ///< N  Error Resilient Harmonic Vector eXcitation Coding
	AOT_ER_HILN         ///< N  Error Resilient Harmonic and Individual Lines plus Noise
	AOT_ER_PARAM        ///< N  Error Resilient Parametric
	AOT_SSC             ///< N  SinuSoidal Coding
	AOT_PS              ///< N  Parametric Stereo
	AOT_SURROUND        ///< N  MPEG Surround
	AOT_ESCAPE          ///< Y  Escape Value
	AOT_L1              ///< Y  Layer 1
	AOT_L2              ///< Y  Layer 2
	AOT_L3              ///< Y  Layer 3
	AOT_DST             ///< N  Direct Stream Transfer
	AOT_ALS             ///< Y  Audio LosslesS
	AOT_SLS             ///< N  Scalable LosslesS
	AOT_SLS_NON_CORE    ///< N  Scalable LosslesS (non core)
	AOT_ER_AAC_ELD      ///< N  Error Resilient Enhanced Low Delay
	AOT_SMR_SIMPLE      ///< N  Symbolic Music Representation Simple
	AOT_SMR_MAIN        ///< N  Symbolic Music Representation Main
	AOT_USAC_NOSBR      ///< N  Unified Speech and Audio Coding (no SBR)
	AOT_SAOC            ///< N  Spatial Audio Object Coding
	AOT_LD_SURROUND     ///< N  Low Delay MPEG Surround
	AOT_USAC            ///< N  Unified Speech and Audio Coding
)

// MPEG4AudioConfig retains the core rate/layout and all supported ASC fields.
// CodecData.SampleRate and ChannelLayout report explicitly signaled SBR/PS output.
type MPEG4AudioConfig struct {
	SampleRate      int
	ChannelLayout   av.ChannelLayout
	ObjectType      uint
	SampleRateIndex uint
	ChannelConfig   uint

	// SampleRate and ChannelLayout describe the core, as declared before SBR/PS.
	BaseObjectType           uint // Core object type; ObjectType retains explicit SBR/PS signaling.
	ExtensionSampleRate      int
	ExtensionSampleRateIndex uint
	SBRPresentFlag           bool
	PSPresentFlag            bool
	FrameLengthFlag          bool
	DependsOnCoreCoder       bool
	CoreCoderDelay           uint16
	ExtensionFlag            bool
	ExtensionFlag3           bool // Unsupported future syntax; true returns an error.
	SyncExtensionPresent     bool
	PSSyncExtensionPresent   bool
}

var sampleRateTable = []int{
	96000, 88200, 64000, 48000, 44100, 32000,
	24000, 22050, 16000, 12000, 11025, 8000, 7350,
}

/*
These are the channel configurations:
0: Defined in AOT Specifc Config
1: 1 channel: front-center
2: 2 channels: front-left, front-right
3: 3 channels: front-center, front-left, front-right
4: 4 channels: front-center, front-left, front-right, back-center
5: 5 channels: front-center, front-left, front-right, back-left, back-right
6: 6 channels: front-center, front-left, front-right, back-left, back-right, LFE-channel
7: 8 channels: front-center, front-left, front-right, side-left, side-right, back-left, back-right, LFE-channel
8-15: Reserved
*/
var chanConfigTable = []av.ChannelLayout{
	0,
	av.CH_FRONT_CENTER,
	av.CH_FRONT_LEFT|av.CH_FRONT_RIGHT,
	av.CH_FRONT_CENTER|av.CH_FRONT_LEFT|av.CH_FRONT_RIGHT,
	av.CH_FRONT_CENTER|av.CH_FRONT_LEFT|av.CH_FRONT_RIGHT|av.CH_BACK_CENTER,
	av.CH_FRONT_CENTER|av.CH_FRONT_LEFT|av.CH_FRONT_RIGHT|av.CH_BACK_LEFT|av.CH_BACK_RIGHT,
	av.CH_FRONT_CENTER|av.CH_FRONT_LEFT|av.CH_FRONT_RIGHT|av.CH_BACK_LEFT|av.CH_BACK_RIGHT|av.CH_LOW_FREQ,
	av.CH_FRONT_CENTER|av.CH_FRONT_LEFT|av.CH_FRONT_RIGHT|av.CH_SIDE_LEFT|av.CH_SIDE_RIGHT|av.CH_BACK_LEFT|av.CH_BACK_RIGHT|av.CH_LOW_FREQ,
}

func ParseADTSHeader(frame []byte) (config MPEG4AudioConfig, hdrlen int, framelen int, samples int, err error) {
	if frame[0] != 0xff || frame[1]&0xf6 != 0xf0 {
		err = fmt.Errorf("aacparser: not adts header")
		return
	}
	config.ObjectType = uint(frame[2]>>6) + 1
	config.SampleRateIndex = uint(frame[2] >> 2 & 0xf)
	config.ChannelConfig = uint(frame[2]<<2&0x4 | frame[3]>>6&0x3)
	if config.ChannelConfig == uint(0) {
		err = fmt.Errorf("aacparser: adts channel count invalid")
		return
	}
	(&config).Complete()
	framelen = int(frame[3]&0x3)<<11 | int(frame[4])<<3 | int(frame[5]>>5)
	samples = (int(frame[6]&0x3) + 1) * 1024
	hdrlen = 7
	if frame[1]&0x1 == 0 {
		hdrlen = 9
	}
	if framelen < hdrlen {
		err = fmt.Errorf("aacparser: adts framelen < hdrlen")
		return
	}
	return
}

const ADTSHeaderLength = 7

func FillADTSHeader(header []byte, config MPEG4AudioConfig, samples int, payloadLength int) {
	payloadLength += 7
	//AAAAAAAA AAAABCCD EEFFFFGH HHIJKLMM MMMMMMMM MMMOOOOO OOOOOOPP (QQQQQQQQ QQQQQQQQ)
	header[0] = 0xff
	header[1] = 0xf1
	header[2] = 0x50
	header[3] = 0x80
	header[4] = 0x43
	header[5] = 0xff
	header[6] = 0xcd
	//config.ObjectType = uint(frames[2]>>6)+1
	//config.SampleRateIndex = uint(frames[2]>>2&0xf)
	//config.ChannelConfig = uint(frames[2]<<2&0x4|frames[3]>>6&0x3)
	header[2] = (byte(config.ObjectType-1)&0x3)<<6 | (byte(config.SampleRateIndex)&0xf)<<2 | byte(config.ChannelConfig>>2)&0x1
	header[3] = header[3]&0x3f | byte(config.ChannelConfig&0x3)<<6
	header[3] = header[3]&0xfc | byte(payloadLength>>11)&0x3
	header[4] = byte(payloadLength >> 3)
	header[5] = header[5]&0x1f | (byte(payloadLength)&0x7)<<5
	header[6] = header[6]&0xfc | byte(samples/1024-1)
	return
}

// CoreSamplesPerFrame returns the number of samples at SampleRate (the core rate).
// SBR changes the output sample count and rate, not the frame duration.
func (c MPEG4AudioConfig) CoreSamplesPerFrame() int {
	if c.FrameLengthFlag {
		return 960
	}
	return 1024
}

// OutputSampleRate includes explicitly signaled SBR. Implicit SBR cannot be
// determined from the ASC alone and requires inspecting decoded audio.
func (c MPEG4AudioConfig) OutputSampleRate() int {
	if c.SBRPresentFlag && c.ExtensionSampleRate != 0 {
		return c.ExtensionSampleRate
	}
	return c.SampleRate
}

// OutputChannelLayout includes explicitly signaled parametric stereo.
func (c MPEG4AudioConfig) OutputChannelLayout() av.ChannelLayout {
	if c.PSPresentFlag {
		return av.CH_FRONT_LEFT | av.CH_FRONT_RIGHT
	}
	return c.ChannelLayout
}

func (self MPEG4AudioConfig) IsValid() bool {
	_, err := normalizeAudioConfig(self)
	return err == nil
}

func (self *MPEG4AudioConfig) Complete() {
	if int(self.SampleRateIndex) < len(sampleRateTable) {
		self.SampleRate = sampleRateTable[self.SampleRateIndex]
	}
	if int(self.ChannelConfig) < len(chanConfigTable) {
		self.ChannelLayout = chanConfigTable[self.ChannelConfig]
	}
	return
}

// ascReader tracks the exact ASC boundary, including unread bits buffered by bits.Reader.
type ascReader struct {
	r          bits.Reader
	pos, total int
	err        error
}

func (r *ascReader) read(n int) uint {
	if r.err != nil {
		return 0
	}
	if n < 0 || n > r.total-r.pos {
		r.err = io.ErrUnexpectedEOF
		return 0
	}
	v, err := r.r.ReadBits(n)
	r.err = err
	if err == nil {
		r.pos += n
	}
	return v
}
func (r *ascReader) flag() bool { return r.read(1) != 0 }
func (r *ascReader) frequency() (uint, int) {
	index := r.read(4)
	var rate int
	if index == 15 {
		rate = int(r.read(24))
	} else if index < uint(len(sampleRateTable)) {
		rate = sampleRateTable[index]
	} else if r.err == nil {
		r.err = fmt.Errorf("reserved sampling frequency index %d", index)
	}
	if r.err == nil && rate == 0 {
		r.err = fmt.Errorf("zero sampling frequency")
	}
	return index, rate
}
func supportedGA(objectType uint) bool {
	switch objectType {
	case AOT_AAC_MAIN, AOT_AAC_LC, AOT_AAC_SSR, AOT_AAC_LTP:
		return true
	}
	return false
}

// ParseMPEG4AudioConfigBytes parses one complete, exactly bounded ASC. Main, LC,
// SSR, LTP and explicit HE-AAC are supported. Program Config Elements (channel
// configuration zero), ER/scalable profiles and future GA syntax return errors.
func ParseMPEG4AudioConfigBytes(data []byte) (c MPEG4AudioConfig, err error) {
	r := &ascReader{r: bits.Reader{R: bytes.NewReader(data)}, total: len(data) * 8}
	c.ObjectType = r.read(5)
	if c.ObjectType == AOT_ESCAPE {
		c.ObjectType = 32 + r.read(6)
	}
	c.SampleRateIndex, c.SampleRate = r.frequency()
	c.ChannelConfig = r.read(4)
	if r.err != nil {
		return c, fmt.Errorf("aacparser: ASC header: %w", r.err)
	}
	if c.ChannelConfig == 0 {
		return c, fmt.Errorf("aacparser: channel configuration zero requires an unsupported Program Config Element")
	}
	c.BaseObjectType = c.ObjectType
	if c.ObjectType == AOT_SBR || c.ObjectType == AOT_PS {
		c.SBRPresentFlag = true
		c.PSPresentFlag = c.ObjectType == AOT_PS
		c.ExtensionSampleRateIndex, c.ExtensionSampleRate = r.frequency()
		c.BaseObjectType = r.read(5)
	}
	if r.err != nil {
		return c, fmt.Errorf("aacparser: HE configuration: %w", r.err)
	}
	if !supportedGA(c.BaseObjectType) {
		return c, fmt.Errorf("aacparser: unsupported core object type %d", c.BaseObjectType)
	}
	c.FrameLengthFlag = r.flag()
	c.DependsOnCoreCoder = r.flag()
	if c.DependsOnCoreCoder {
		c.CoreCoderDelay = uint16(r.read(14))
	}
	c.ExtensionFlag = r.flag()
	if c.ExtensionFlag {
		c.ExtensionFlag3 = r.flag()
	}
	if r.err != nil {
		return c, fmt.Errorf("aacparser: GA configuration: %w", r.err)
	}
	if c.ExtensionFlag3 {
		return c, fmt.Errorf("aacparser: unsupported extensionFlag3 syntax")
	}
	if r.total-r.pos > 7 {
		if c.ObjectType == AOT_SBR || c.ObjectType == AOT_PS {
			return c, fmt.Errorf("aacparser: unexpected extension after explicit HE configuration")
		}
		marker := r.read(11)
		extensionType := r.read(5)
		if r.err != nil {
			return c, fmt.Errorf("aacparser: sync extension: %w", r.err)
		}
		if marker != 0x2b7 || extensionType != AOT_SBR {
			return c, fmt.Errorf("aacparser: unsupported sync extension")
		}
		c.SyncExtensionPresent = true
		c.SBRPresentFlag = r.flag()
		if c.SBRPresentFlag {
			c.ExtensionSampleRateIndex, c.ExtensionSampleRate = r.frequency()
		}
		if r.total-r.pos > 7 && r.err == nil {
			marker = r.read(11)
			c.PSSyncExtensionPresent = true
			c.PSPresentFlag = r.flag()
			if r.err == nil && marker != 0x548 {
				return c, fmt.Errorf("aacparser: unsupported PS sync extension")
			}
		}
	}
	if r.err != nil {
		return c, fmt.Errorf("aacparser: sync extension: %w", r.err)
	}
	if r.total-r.pos > 7 || r.read(r.total-r.pos) != 0 {
		return c, fmt.Errorf("aacparser: invalid ASC padding")
	}
	return normalizeAudioConfig(c)
}

// normalizeFrequency preserves explicit (index 15) signaling and fills omitted
// indexed rates for existing callers. Index zero is also the Go zero value;
// when a rate is supplied without a nonzero index, the rate selects the index.
func normalizeFrequency(index uint, rate int) (uint, int, error) {
	if index > 15 || index == 13 || index == 14 {
		return 0, 0, fmt.Errorf("aacparser: invalid sampling frequency index %d", index)
	}
	if rate == 0 && index < uint(len(sampleRateTable)) {
		rate = sampleRateTable[index]
	}
	if rate <= 0 || rate > 0xffffff {
		return 0, 0, fmt.Errorf("aacparser: invalid sampling frequency %d", rate)
	}
	if index == 15 {
		return index, rate, nil
	}
	if index != 0 && sampleRateTable[index] != rate {
		return 0, 0, fmt.Errorf("aacparser: sample rate/index mismatch")
	}
	for i, v := range sampleRateTable {
		if v == rate {
			return uint(i), rate, nil
		}
	}
	return 15, rate, nil
}

func normalizeAudioConfig(c MPEG4AudioConfig) (MPEG4AudioConfig, error) {
	explicitHE := c.ObjectType == AOT_SBR || c.ObjectType == AOT_PS
	if !explicitHE && !supportedGA(c.ObjectType) {
		return c, fmt.Errorf("aacparser: unsupported object type %d", c.ObjectType)
	}
	if c.BaseObjectType == 0 {
		c.BaseObjectType = c.ObjectType
		if explicitHE {
			c.BaseObjectType = AOT_AAC_LC
		}
	}
	if !supportedGA(c.BaseObjectType) || (!explicitHE && c.BaseObjectType != c.ObjectType) {
		return c, fmt.Errorf("aacparser: invalid core object type %d", c.BaseObjectType)
	}
	if c.ChannelConfig == 0 && c.ChannelLayout != 0 {
		for i, layout := range chanConfigTable {
			if layout == c.ChannelLayout {
				c.ChannelConfig = uint(i)
				break
			}
		}
	}
	if c.ChannelConfig == 0 {
		return c, fmt.Errorf("aacparser: channel configuration zero requires an unsupported Program Config Element")
	}
	if c.ChannelConfig >= uint(len(chanConfigTable)) {
		return c, fmt.Errorf("aacparser: unsupported channel configuration %d", c.ChannelConfig)
	}
	c.ChannelLayout = chanConfigTable[c.ChannelConfig]
	var err error
	c.SampleRateIndex, c.SampleRate, err = normalizeFrequency(c.SampleRateIndex, c.SampleRate)
	if err != nil {
		return c, err
	}
	if explicitHE {
		if c.SyncExtensionPresent || c.PSSyncExtensionPresent {
			return c, fmt.Errorf("aacparser: explicit HE with sync extension")
		}
		c.SBRPresentFlag = true
		if c.ObjectType == AOT_PS {
			c.PSPresentFlag = true
		} else if c.PSPresentFlag {
			return c, fmt.Errorf("aacparser: PS requires AOT_PS or a PS sync extension")
		}
	}
	if c.SBRPresentFlag {
		if !explicitHE && !c.SyncExtensionPresent {
			return c, fmt.Errorf("aacparser: SBR requires sync signaling")
		}
		// Unlike the base rate, an extension rate must be explicitly supplied.
		if c.ExtensionSampleRate == 0 && c.ExtensionSampleRateIndex == 0 {
			return c, fmt.Errorf("aacparser: missing extension sample rate")
		}
		c.ExtensionSampleRateIndex, c.ExtensionSampleRate, err = normalizeFrequency(c.ExtensionSampleRateIndex, c.ExtensionSampleRate)
		if err != nil {
			return c, err
		}
	} else if c.ExtensionSampleRate != 0 || c.ExtensionSampleRateIndex != 0 {
		return c, fmt.Errorf("aacparser: extension rate without SBR")
	}
	if c.PSSyncExtensionPresent && !c.SyncExtensionPresent {
		return c, fmt.Errorf("aacparser: PS sync extension without SBR sync extension")
	}
	if c.PSPresentFlag && (!c.SBRPresentFlag || (!explicitHE && !c.PSSyncExtensionPresent)) {
		return c, fmt.Errorf("aacparser: invalid PS signaling")
	}
	if c.CoreCoderDelay > 0x3fff || (!c.DependsOnCoreCoder && c.CoreCoderDelay != 0) {
		return c, fmt.Errorf("aacparser: invalid core coder delay")
	}
	if c.ExtensionFlag3 {
		return c, fmt.Errorf("aacparser: unsupported extensionFlag3 syntax")
	}
	return c, nil
}

// A small byte-oriented writer avoids utils/bits.Writer's 64-bit boundary
// behavior when both core and extension frequencies are explicitly encoded.
type ascWriter struct {
	data []byte
	pos  int
}

func (w *ascWriter) write(v uint, n int) {
	for bit := n - 1; bit >= 0; bit-- {
		if w.pos%8 == 0 {
			w.data = append(w.data, 0)
		}
		w.data[w.pos/8] |= byte((v>>uint(bit))&1) << uint(7-w.pos%8)
		w.pos++
	}
}
func (w *ascWriter) flag(v bool) {
	if v {
		w.write(1, 1)
	} else {
		w.write(0, 1)
	}
}
func (w *ascWriter) frequency(index uint, rate int) {
	w.write(index, 4)
	if index == 15 {
		w.write(uint(rate), 24)
	}
}

// WriteMPEG4AudioConfig serializes the complete supported configuration. Invalid
// configurations return an error without writing any bytes. Original ASC bytes
// in CodecData remain available for remuxing without reconstruction.
func WriteMPEG4AudioConfig(dst io.Writer, config MPEG4AudioConfig) error {
	c, err := normalizeAudioConfig(config)
	if err != nil {
		return err
	}
	w := &ascWriter{}
	w.write(c.ObjectType, 5)
	w.frequency(c.SampleRateIndex, c.SampleRate)
	w.write(c.ChannelConfig, 4)
	if c.ObjectType == AOT_SBR || c.ObjectType == AOT_PS {
		w.frequency(c.ExtensionSampleRateIndex, c.ExtensionSampleRate)
		w.write(c.BaseObjectType, 5)
	}
	w.flag(c.FrameLengthFlag)
	w.flag(c.DependsOnCoreCoder)
	if c.DependsOnCoreCoder {
		w.write(uint(c.CoreCoderDelay), 14)
	}
	w.flag(c.ExtensionFlag)
	if c.ExtensionFlag {
		w.flag(c.ExtensionFlag3)
	}
	if c.SyncExtensionPresent {
		w.write(0x2b7, 11)
		w.write(AOT_SBR, 5)
		w.flag(c.SBRPresentFlag)
		if c.SBRPresentFlag {
			w.frequency(c.ExtensionSampleRateIndex, c.ExtensionSampleRate)
		}
		if c.PSSyncExtensionPresent {
			w.write(0x548, 11)
			w.flag(c.PSPresentFlag)
		}
	}
	n, err := dst.Write(w.data)
	if err == nil && n != len(w.data) {
		err = io.ErrShortWrite
	}
	return err
}

type CodecData struct {
	ConfigBytes []byte
	Config MPEG4AudioConfig
}

func (self CodecData) Type() av.CodecType {
	return av.AAC
}

func (self CodecData) MPEG4AudioConfigBytes() []byte {
	return self.ConfigBytes
}

func (self CodecData) ChannelLayout() av.ChannelLayout {
	return self.Config.OutputChannelLayout()
}

func (self CodecData) SampleRate() int {
	return self.Config.OutputSampleRate()
}

func (self CodecData) SampleFormat() av.SampleFormat {
	return av.FLTP
}

func (self CodecData) PacketDuration(data []byte) (dur time.Duration, err error) {
	if self.Config.SampleRate <= 0 {
		return 0, fmt.Errorf("aacparser: invalid core sample rate")
	}
	dur = time.Duration(self.Config.CoreSamplesPerFrame()) * time.Second / time.Duration(self.Config.SampleRate)
	return
}

func NewCodecDataFromMPEG4AudioConfig(config MPEG4AudioConfig) (self CodecData, err error) {
	b := &bytes.Buffer{}
	if err = WriteMPEG4AudioConfig(b, config); err != nil {
		return
	}
	return NewCodecDataFromMPEG4AudioConfigBytes(b.Bytes())
}

func NewCodecDataFromMPEG4AudioConfigBytes(config []byte) (self CodecData, err error) {
	self.ConfigBytes = config
	if self.Config, err = ParseMPEG4AudioConfigBytes(config); err != nil {
		err = fmt.Errorf("aacparser: parse MPEG4AudioConfig failed(%s)", err)
		return
	}
	return
}

