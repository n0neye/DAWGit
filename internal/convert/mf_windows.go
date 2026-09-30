//go:build windows

package convert

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"dawgit/internal/audio"
)

// Media Foundation: a Source Reader decodes the input to PCM; a Sink Writer
// encodes it (MP3, AAC, FLAC). WAV is written here from the PCM.

var (
	mfplat      = windows.NewLazySystemDLL("mfplat.dll")
	mfreadwrite = windows.NewLazySystemDLL("mfreadwrite.dll")
	mfdll       = windows.NewLazySystemDLL("mf.dll")

	procMFStartup         = mfplat.NewProc("MFStartup")
	procMFShutdown        = mfplat.NewProc("MFShutdown")
	procMFCreateMediaType = mfplat.NewProc("MFCreateMediaType")
	procCreateReader      = mfreadwrite.NewProc("MFCreateSourceReaderFromURL")
	procCreateWriter      = mfreadwrite.NewProc("MFCreateSinkWriterFromURL")
	procAvailableTypes    = mfdll.NewProc("MFTranscodeGetAudioOutputAvailableTypes")
)

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

var (
	majorAudio   = mustGUID("{73647561-0000-0010-8000-00AA00389B71}")
	subPCM       = mustGUID("{00000001-0000-0010-8000-00AA00389B71}")
	subMP3       = mustGUID("{00000055-0000-0010-8000-00AA00389B71}")
	subAAC       = mustGUID("{00001610-0000-0010-8000-00AA00389B71}")
	subFLAC      = mustGUID("{0000F1AC-0000-0010-8000-00AA00389B71}")
	keyMajor     = mustGUID("{48eba18e-f8c9-4687-bf11-0a74c9f96a8f}")
	keySub       = mustGUID("{f7e34c9a-42e8-4714-b74b-cb29d72c35e5}")
	keyChannels  = mustGUID("{37e48bf5-645e-4c5b-89de-ada9e29b696a}")
	keyRate      = mustGUID("{5faeeae7-0290-4c31-9e8a-c534f68d9dba}")
	keyBits      = mustGUID("{f2deb57f-40fa-4764-aa33-ed4f2d1ff669}")
	keyBlock     = mustGUID("{322de230-9eeb-43bd-ab7a-ff412251541d}")
	keyAvgBytes  = mustGUID("{1aab75c8-cfef-451c-ab95-ac034b8e1731}")
	keyDuration  = mustGUID("{6c990d33-bb8e-477a-8598-0d5d96fcd88a}")
	iidMediaType = mustGUID("{44ae0fa8-ea31-4109-8d2e-4cae4997c555}")
)

const (
	firstAudioStream  = 0xFFFFFFFD
	allStreams        = 0xFFFFFFFE
	mediaSource       = 0xFFFFFFFF
	readerEndOfStream = 0x2
	readerError       = 0x1
	mftEnumAll        = 0x3F
)

// vtable indices
const (
	iRelease        = 2
	iQueryInterface = 0
	// IMFAttributes
	aGetUINT32 = 7
	aSetUINT32 = 21
	aSetGUID   = 24
	// IMFSourceReader
	rSetStreamSelection  = 4
	rGetNativeMediaType  = 5
	rGetCurrentMediaType = 6
	rSetCurrentMediaType = 7
	rReadSample          = 9
	rGetPresentationAttr = 12
	// IMFSinkWriter
	wAddStream         = 3
	wSetInputMediaType = 4
	wBeginWriting      = 5
	wWriteSample       = 6
	wFinalize          = 11
	// IMFSample, IMFMediaBuffer, IMFCollection
	sConvertToContiguousBuffer = 41
	bLock                      = 3
	bUnlock                    = 4
	cGetElementCount           = 3
	cGetElement                = 4
)

// vcall calls method index of a COM object. Pointer arguments converted to
// uintptr in the call are moved to the heap (uintptrescapes), so they stay
// put during the call.
//
//go:uintptrescapes
func vcall(obj unsafe.Pointer, index int, args ...uintptr) error {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, index*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return hr(r)
}

func hr(r uintptr) error {
	if int32(r) >= 0 {
		return nil
	}
	switch uint32(r) {
	case 0xC00D36B4:
		return fmt.Errorf("the format is not supported (MF_E_INVALIDMEDIATYPE)")
	case 0xC00D36C4:
		return fmt.Errorf("this file's format can't be read (MF_E_UNSUPPORTED_BYTESTREAM_TYPE)")
	case 0xC00D5212:
		return fmt.Errorf("no encoder for this format (MF_E_TOPO_CODEC_NOT_FOUND)")
	}
	return fmt.Errorf("media foundation error 0x%08X", uint32(r))
}

func release(p unsafe.Pointer) {
	if p != nil {
		vcall(p, iRelease)
	}
}

func setGUID(attrs unsafe.Pointer, key, val windows.GUID) error {
	return vcall(attrs, aSetGUID, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&val)))
}

func setU32(attrs unsafe.Pointer, key windows.GUID, v uint32) error {
	return vcall(attrs, aSetUINT32, uintptr(unsafe.Pointer(&key)), uintptr(v))
}

func getU32(attrs unsafe.Pointer, key windows.GUID) uint32 {
	var v uint32
	if vcall(attrs, aGetUINT32, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&v))) != nil {
		return 0
	}
	return v
}

func newMediaType() (unsafe.Pointer, error) {
	var t unsafe.Pointer
	r, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&t)))
	return t, hr(r)
}

func pcmType(p pcm) (unsafe.Pointer, error) {
	t, err := newMediaType()
	if err != nil {
		return nil, err
	}
	block := p.channels * p.bits / 8
	for _, e := range []error{
		setGUID(t, keyMajor, majorAudio), setGUID(t, keySub, subPCM),
		setU32(t, keyChannels, p.channels), setU32(t, keyRate, p.rate), setU32(t, keyBits, p.bits),
		setU32(t, keyBlock, block), setU32(t, keyAvgBytes, block*p.rate),
	} {
		if e != nil {
			release(t)
			return nil, e
		}
	}
	return t, nil
}

// mfStart sets up COM and Media Foundation on this (locked) thread; stop
// undoes it. Both count, so nested calls are fine.
func mfStart() (stop func(), err error) {
	runtime.LockOSThread()
	// S_FALSE (already initialized) still needs the matching uninitialize; a
	// thread set up differently (RPC_E_CHANGED_MODE) works as it is.
	co := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED)
	uninit := func() {
		if co == nil || co == syscall.Errno(1) {
			windows.CoUninitialize()
		}
		runtime.UnlockOSThread()
	}
	if r, _, _ := procMFStartup.Call(0x20070, 0); hr(r) != nil {
		uninit()
		return nil, fmt.Errorf("media foundation is not available: %w", hr(r))
	}
	return func() { procMFShutdown.Call(); uninit() }, nil
}

func convert(src, dst string, f Format, o Options) error {
	stop, err := mfStart()
	if err != nil {
		return err
	}
	defer stop()

	// Media Foundation does not read AIFF: go through WAV.
	in := src
	if ext := strings.ToLower(filepath.Ext(src)); ext == ".aif" || ext == ".aiff" {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		wav, err := audio.AIFFToWAV(data)
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp("", "dawgit-*.wav")
		if err != nil {
			return err
		}
		in = tmp.Name()
		defer os.Remove(in)
		_, err = tmp.Write(wav)
		if cerr := tmp.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}

	var reader unsafe.Pointer
	path, _ := windows.UTF16PtrFromString(in)
	if r, _, _ := procCreateReader.Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&reader))); hr(r) != nil {
		return fmt.Errorf("can't read %s: %w", filepath.Base(src), hr(r))
	}
	defer release(reader)
	vcall(reader, rSetStreamSelection, allStreams, 0)
	if err := vcall(reader, rSetStreamSelection, firstAudioStream, 1); err != nil {
		return fmt.Errorf("no audio in %s: %w", filepath.Base(src), err)
	}

	var native unsafe.Pointer
	if err := vcall(reader, rGetNativeMediaType, firstAudioStream, 0, uintptr(unsafe.Pointer(&native))); err != nil {
		return err
	}
	srcFmt := Info{Rate: int(getU32(native, keyRate)), Channels: int(getU32(native, keyChannels)), Bits: int(getU32(native, keyBits))}
	release(native)
	plan, err := Plan(f.ID, srcFmt, o)
	if err != nil {
		return err
	}
	o.Bitrate = plan.Bitrate
	want := pcm{rate: uint32(plan.Rate), channels: uint32(plan.Channels), bits: uint32(plan.Bits)}

	t, err := pcmType(want)
	if err != nil {
		return err
	}
	err = vcall(reader, rSetCurrentMediaType, firstAudioStream, 0, uintptr(t))
	release(t)
	if err != nil {
		return fmt.Errorf("can't decode %s to %d Hz / %d-bit: %w", filepath.Base(src), want.rate, want.bits, err)
	}
	var cur unsafe.Pointer
	if err := vcall(reader, rGetCurrentMediaType, firstAudioStream, uintptr(unsafe.Pointer(&cur))); err != nil {
		return err
	}
	defer release(cur)
	got := pcm{rate: getU32(cur, keyRate), channels: getU32(cur, keyChannels), bits: getU32(cur, keyBits)}
	duration := sourceDuration(reader)

	if f.ID == "wav16" || f.ID == "wav24" {
		return writeWAV(reader, dst, got, duration, o.Progress)
	}
	return encode(reader, cur, dst, f, got, o, duration)
}

// probe reads a compressed sample's format with Media Foundation.
func probe(src string) (Info, error) {
	stop, err := mfStart()
	if err != nil {
		return Info{}, err
	}
	defer stop()
	var reader unsafe.Pointer
	path, _ := windows.UTF16PtrFromString(src)
	if r, _, _ := procCreateReader.Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&reader))); hr(r) != nil {
		return Info{}, hr(r)
	}
	defer release(reader)
	var native unsafe.Pointer
	if err := vcall(reader, rGetNativeMediaType, firstAudioStream, 0, uintptr(unsafe.Pointer(&native))); err != nil {
		return Info{}, err
	}
	defer release(native)
	return Info{Rate: int(getU32(native, keyRate)), Channels: int(getU32(native, keyChannels)),
		Bits: int(getU32(native, keyBits)), Seconds: float64(sourceDuration(reader)) / 1e7}, nil
}

// sourceDuration in 100 ns units (0 if unknown).
func sourceDuration(reader unsafe.Pointer) int64 {
	var pv [24]byte // PROPVARIANT
	if vcall(reader, rGetPresentationAttr, mediaSource, uintptr(unsafe.Pointer(&keyDuration)), uintptr(unsafe.Pointer(&pv))) != nil {
		return 0
	}
	if binary.LittleEndian.Uint16(pv[:2]) != 21 { // VT_UI8
		return 0
	}
	return int64(binary.LittleEndian.Uint64(pv[8:16]))
}

// readAll hands every decoded sample to fn (with its time, 100 ns units).
func readAll(reader unsafe.Pointer, fn func(sample unsafe.Pointer, t int64) error) error {
	for {
		var index, flags uint32
		var ts int64
		var sample unsafe.Pointer
		if err := vcall(reader, rReadSample, firstAudioStream, 0, uintptr(unsafe.Pointer(&index)),
			uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&ts)), uintptr(unsafe.Pointer(&sample))); err != nil {
			return err
		}
		if flags&readerError != 0 {
			release(sample)
			return fmt.Errorf("reading the audio failed")
		}
		if sample != nil {
			err := fn(sample, ts)
			release(sample)
			if err != nil {
				return err
			}
		}
		if flags&readerEndOfStream != 0 {
			return nil
		}
	}
}

func writeWAV(reader unsafe.Pointer, dst string, p pcm, duration int64, progress func(float64)) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	header := make([]byte, 44)
	if _, err := out.Write(header); err != nil {
		return err
	}
	var size int64
	err = readAll(reader, func(sample unsafe.Pointer, ts int64) error {
		var buf unsafe.Pointer
		if err := vcall(sample, sConvertToContiguousBuffer, uintptr(unsafe.Pointer(&buf))); err != nil {
			return err
		}
		defer release(buf)
		var data unsafe.Pointer
		var max, n uint32
		if err := vcall(buf, bLock, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&max)), uintptr(unsafe.Pointer(&n))); err != nil {
			return err
		}
		_, werr := out.Write(unsafe.Slice((*byte)(data), n))
		vcall(buf, bUnlock)
		size += int64(n)
		if duration > 0 {
			progress(float64(ts) / float64(duration))
		}
		return werr
	})
	if err != nil {
		return err
	}
	block := p.channels * p.bits / 8
	le := binary.LittleEndian
	copy(header[0:], "RIFF")
	le.PutUint32(header[4:], uint32(36+size))
	copy(header[8:], "WAVEfmt ")
	le.PutUint32(header[16:], 16)
	le.PutUint16(header[20:], 1)
	le.PutUint16(header[22:], uint16(p.channels))
	le.PutUint32(header[24:], p.rate)
	le.PutUint32(header[28:], p.rate*block)
	le.PutUint16(header[32:], uint16(block))
	le.PutUint16(header[34:], uint16(p.bits))
	copy(header[36:], "data")
	le.PutUint32(header[40:], uint32(size))
	if _, err := out.WriteAt(header, 0); err != nil {
		return err
	}
	progress(1)
	return out.Close()
}

func encode(reader, input unsafe.Pointer, dst string, f Format, p pcm, o Options, duration int64) error {
	outType, err := encoderType(f, p, o.Bitrate)
	if err != nil {
		return err
	}
	defer release(outType)
	var writer unsafe.Pointer
	path, _ := windows.UTF16PtrFromString(dst)
	if r, _, _ := procCreateWriter.Call(uintptr(unsafe.Pointer(path)), 0, 0, uintptr(unsafe.Pointer(&writer))); hr(r) != nil {
		return fmt.Errorf("can't write %s: %w", filepath.Base(dst), hr(r))
	}
	defer release(writer)
	var stream uint32
	if err := vcall(writer, wAddStream, uintptr(outType), uintptr(unsafe.Pointer(&stream))); err != nil {
		return fmt.Errorf("%s encoder: %w", f.Name, err)
	}
	if err := vcall(writer, wSetInputMediaType, uintptr(stream), uintptr(input), 0); err != nil {
		return fmt.Errorf("%s encoder input: %w", f.Name, err)
	}
	if err := vcall(writer, wBeginWriting); err != nil {
		return err
	}
	err = readAll(reader, func(sample unsafe.Pointer, ts int64) error {
		if duration > 0 {
			o.Progress(float64(ts) / float64(duration))
		}
		return vcall(writer, wWriteSample, uintptr(stream), uintptr(sample))
	})
	if err != nil {
		return err
	}
	if err := vcall(writer, wFinalize); err != nil {
		return fmt.Errorf("finishing %s: %w", filepath.Base(dst), err)
	}
	o.Progress(1)
	return nil
}

// encoderType finds the encoder's output type for the format, rate,
// channels (and bitrate for lossy formats) among the types it offers.
// encoderTypes calls fn with each output type the format's encoder offers
// until it returns true (and keeps that type: the caller releases it).
func encoderTypes(f Format, fn func(t unsafe.Pointer) bool) error {
	sub := map[string]windows.GUID{"mp3": subMP3, "aac": subAAC, "flac": subFLAC}[f.ID]
	var coll unsafe.Pointer
	if r, _, _ := procAvailableTypes.Call(uintptr(unsafe.Pointer(&sub)), mftEnumAll, 0, uintptr(unsafe.Pointer(&coll))); hr(r) != nil {
		return fmt.Errorf("no %s encoder on this computer: %w", f.Name, hr(r))
	}
	defer release(coll)
	var n uint32
	if err := vcall(coll, cGetElementCount, uintptr(unsafe.Pointer(&n))); err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		var unk, t unsafe.Pointer
		if vcall(coll, cGetElement, uintptr(i), uintptr(unsafe.Pointer(&unk))) != nil {
			continue
		}
		err := vcall(unk, iQueryInterface, uintptr(unsafe.Pointer(&iidMediaType)), uintptr(unsafe.Pointer(&t)))
		release(unk)
		if err != nil {
			continue
		}
		if fn(t) {
			return nil
		}
		release(t)
	}
	return nil
}

func encoderType(f Format, p pcm, kbps int) (unsafe.Pointer, error) {
	var found unsafe.Pointer
	err := encoderTypes(f, func(t unsafe.Pointer) bool {
		ok := getU32(t, keyRate) == p.rate && getU32(t, keyChannels) == p.channels
		switch {
		case kbps > 0:
			ok = ok && getU32(t, keyAvgBytes) == uint32(kbps*1000/8)
		case f.ID == "flac":
			ok = ok && (getU32(t, keyBits) == 0 || getU32(t, keyBits) == p.bits)
		}
		if ok {
			found = t
		}
		return ok
	})
	if err != nil || found != nil {
		return found, err
	}
	if kbps > 0 {
		return nil, fmt.Errorf("the %s encoder has no %d kbps setting for %d Hz, %d channel(s)", f.Name, kbps, p.rate, p.channels)
	}
	return nil, fmt.Errorf("the %s encoder does not take %d Hz, %d-bit, %d channel(s)", f.Name, p.rate, p.bits, p.channels)
}

// encoderBitrates lists the kbps the format's encoder offers at a rate and
// channel count (nil when it can't tell).
func encoderBitrates(f Format, rate, channels int) []int {
	stop, err := mfStart()
	if err != nil {
		return nil
	}
	defer stop()
	var out []int
	encoderTypes(f, func(t unsafe.Pointer) bool {
		if int(getU32(t, keyRate)) == rate && int(getU32(t, keyChannels)) == channels {
			if k := int(getU32(t, keyAvgBytes)) * 8 / 1000; k > 0 && !contains(out, k) {
				out = append(out, k)
			}
		}
		return false
	})
	return out
}
