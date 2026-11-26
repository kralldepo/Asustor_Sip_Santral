package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"
)

// PCM is mono audio as float32 samples in [-1, 1].
type PCM struct {
	Rate    int
	Samples []float32
}

func (p PCM) Duration() time.Duration {
	if p.Rate <= 0 {
		return 0
	}
	return time.Duration(float64(len(p.Samples)) / float64(p.Rate) * float64(time.Second))
}

var errNotWAV = errors.New("WAV dosyası değil")

// ReadWAV parses PCM (8/16/24/32 bit) and IEEE float WAV data, mixing all
// channels down to mono. Streaming writers (e.g. Piper) leave placeholder
// sizes in the header, so the data chunk is read up to EOF when needed.
func ReadWAV(r io.Reader) (PCM, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 512<<20))
	if err != nil {
		return PCM{}, err
	}
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return PCM{}, errNotWAV
	}
	var (
		format   uint16
		channels int
		rate     int
		bits     int
		haveFmt  bool
	)
	pos := 12
	for pos+8 <= len(raw) {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		body := pos + 8
		switch id {
		case "fmt ":
			if size < 16 || body+16 > len(raw) {
				return PCM{}, fmt.Errorf("WAV fmt bölümü bozuk")
			}
			f := raw[body:]
			format = binary.LittleEndian.Uint16(f[0:2])
			channels = int(binary.LittleEndian.Uint16(f[2:4]))
			rate = int(binary.LittleEndian.Uint32(f[4:8]))
			bits = int(binary.LittleEndian.Uint16(f[14:16]))
			if format == 0xFFFE && size >= 26 && body+26 <= len(raw) { // WAVE_FORMAT_EXTENSIBLE
				format = binary.LittleEndian.Uint16(f[24:26])
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return PCM{}, fmt.Errorf("WAV fmt bölümü yok")
			}
			end := body + size
			if size <= 0 || end > len(raw) || end < body {
				end = len(raw)
			}
			return decodePCM(raw[body:end], format, channels, rate, bits)
		}
		next := body + size + size%2
		if size < 0 || next <= pos || next > len(raw) {
			break
		}
		pos = next
	}
	return PCM{}, fmt.Errorf("WAV ses verisi bulunamadı")
}

func decodePCM(data []byte, format uint16, channels, rate, bits int) (PCM, error) {
	if channels < 1 || channels > 8 || rate < 4000 || rate > 384000 {
		return PCM{}, fmt.Errorf("desteklenmeyen WAV: %d kanal, %d Hz", channels, rate)
	}
	bps := bits / 8
	if format == 1 && (bits == 8 || bits == 16 || bits == 24 || bits == 32) {
	} else if format == 3 && bits == 32 {
	} else {
		return PCM{}, fmt.Errorf("desteklenmeyen WAV biçimi (tür %d, %d bit)", format, bits)
	}
	frame := bps * channels
	n := len(data) / frame
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		var sum float32
		for c := 0; c < channels; c++ {
			o := i*frame + c*bps
			var v float32
			switch {
			case format == 3:
				v = math.Float32frombits(binary.LittleEndian.Uint32(data[o:]))
			case bits == 8:
				v = (float32(data[o]) - 128) / 128
			case bits == 16:
				v = float32(int16(binary.LittleEndian.Uint16(data[o:]))) / 32768
			case bits == 24:
				x := int32(data[o]) | int32(data[o+1])<<8 | int32(int8(data[o+2]))<<16
				v = float32(x) / 8388608
			case bits == 32:
				v = float32(int32(binary.LittleEndian.Uint32(data[o:]))) / 2147483648
			}
			if v != v { // NaN
				v = 0
			}
			sum += v
		}
		out[i] = sum / float32(channels)
	}
	return PCM{Rate: rate, Samples: out}, nil
}

// Resample converts to the target rate. Downsampling first applies a
// windowed-sinc low-pass filter below the new Nyquist frequency.
func (p PCM) Resample(rate int) PCM {
	if p.Rate == rate || len(p.Samples) == 0 {
		return PCM{Rate: rate, Samples: append([]float32(nil), p.Samples...)}
	}
	src := p.Samples
	if rate < p.Rate {
		src = lowPass(src, 0.45*float64(rate)/float64(p.Rate), 95)
	}
	ratio := float64(p.Rate) / float64(rate)
	n := int(float64(len(src)) / ratio)
	out := make([]float32, n)
	for i := range out {
		x := float64(i) * ratio
		j := int(x)
		f := float32(x - float64(j))
		a := src[j]
		b := a
		if j+1 < len(src) {
			b = src[j+1]
		}
		out[i] = a + (b-a)*f
	}
	return PCM{Rate: rate, Samples: out}
}

// lowPass filters with a Blackman-windowed sinc; cutoff is in cycles/sample.
func lowPass(x []float32, cutoff float64, taps int) []float32 {
	if taps%2 == 0 {
		taps++
	}
	h := make([]float64, taps)
	m := float64(taps - 1)
	var sum float64
	for i := range h {
		k := float64(i) - m/2
		var s float64
		if k == 0 {
			s = 2 * cutoff
		} else {
			s = math.Sin(2*math.Pi*cutoff*k) / (math.Pi * k)
		}
		w := 0.42 - 0.5*math.Cos(2*math.Pi*float64(i)/m) + 0.08*math.Cos(4*math.Pi*float64(i)/m)
		h[i] = s * w
		sum += h[i]
	}
	for i := range h {
		h[i] /= sum
	}
	half := taps / 2
	out := make([]float32, len(x))
	for i := range x {
		var acc float64
		for t := 0; t < taps; t++ {
			j := i + t - half
			if j >= 0 && j < len(x) {
				acc += float64(x[j]) * h[t]
			}
		}
		out[i] = float32(acc)
	}
	return out
}

// Normalize scales the peak to peakDB (dBFS), boosting at most maxGainDB.
func (p PCM) Normalize(peakDB, maxGainDB float64) PCM {
	var peak float32
	for _, v := range p.Samples {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak < 1e-5 {
		return p
	}
	target := math.Pow(10, peakDB/20)
	gain := target / float64(peak)
	if max := math.Pow(10, maxGainDB/20); gain > max {
		gain = max
	}
	out := make([]float32, len(p.Samples))
	for i, v := range p.Samples {
		out[i] = float32(float64(v) * gain)
	}
	return PCM{Rate: p.Rate, Samples: out}
}

// TrimSilence removes quiet lead-in/lead-out, keeping `keep` around the sound.
func (p PCM) TrimSilence(thresholdDB float64, keep time.Duration) PCM {
	thr := float32(math.Pow(10, thresholdDB/20))
	first, last := -1, -1
	for i, v := range p.Samples {
		if v > thr || v < -thr {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return PCM{Rate: p.Rate}
	}
	k := int(keep.Seconds() * float64(p.Rate))
	start, end := first-k, last+k+1
	if start < 0 {
		start = 0
	}
	if end > len(p.Samples) {
		end = len(p.Samples)
	}
	return PCM{Rate: p.Rate, Samples: append([]float32(nil), p.Samples[start:end]...)}
}

// Fade applies short fade-in/out ramps to avoid clicks.
func (p PCM) Fade(d time.Duration) PCM {
	n := int(d.Seconds() * float64(p.Rate))
	out := append([]float32(nil), p.Samples...)
	if n*2 > len(out) {
		n = len(out) / 2
	}
	for i := 0; i < n; i++ {
		g := float32(i) / float32(n)
		out[i] *= g
		out[len(out)-1-i] *= g
	}
	return PCM{Rate: p.Rate, Samples: out}
}

// EncodeWAV16 writes 16-bit mono PCM WAV.
func EncodeWAV16(p PCM) []byte {
	var buf bytes.Buffer
	dataLen := len(p.Samples) * 2
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint32(p.Rate))
	binary.Write(&buf, binary.LittleEndian, uint32(p.Rate*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataLen))
	b := make([]byte, 2)
	for _, v := range p.Samples {
		x := float64(v) * 32767
		if x > 32767 {
			x = 32767
		} else if x < -32768 {
			x = -32768
		}
		binary.LittleEndian.PutUint16(b, uint16(int16(math.Round(x))))
		buf.Write(b)
	}
	return buf.Bytes()
}

// Tone generates a sine tone (used for the voicemail beep).
func Tone(freq float64, d time.Duration, rate int, amp float64) PCM {
	n := int(d.Seconds() * float64(rate))
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(amp * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
	}
	return PCM{Rate: rate, Samples: s}.Fade(8 * time.Millisecond)
}

// Silence returns d of silence.
func Silence(d time.Duration, rate int) PCM {
	return PCM{Rate: rate, Samples: make([]float32, int(d.Seconds()*float64(rate)))}
}

// PrepareAnnouncement turns any decoded audio into a telephone prompt:
// trimmed, 8 kHz mono, peak-normalized, with soft edges.
func PrepareAnnouncement(p PCM) (PCM, error) {
	if len(p.Samples) == 0 {
		return PCM{}, verr("Ses dosyası boş")
	}
	if p.Duration() > 6*time.Minute {
		return PCM{}, verr("Anons en fazla 5 dakika olabilir")
	}
	t := p.TrimSilence(-48, 150*time.Millisecond)
	if t.Duration() < 300*time.Millisecond {
		return PCM{}, verr("Kayıtta duyulabilir ses yok")
	}
	if t.Duration() > 5*time.Minute+time.Second {
		return PCM{}, verr("Anons en fazla 5 dakika olabilir")
	}
	out := t.Resample(8000).Normalize(-1.5, 24).Fade(6 * time.Millisecond)
	return out, nil
}

// wavFileDuration reads the duration of a PCM WAV file from its header.
func wavFileDuration(path string) time.Duration {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0
	}
	hdr := make([]byte, 44)
	if _, err := io.ReadFull(f, hdr); err != nil || string(hdr[0:4]) != "RIFF" {
		return 0
	}
	rate := binary.LittleEndian.Uint32(hdr[24:28])
	byteRate := binary.LittleEndian.Uint32(hdr[28:32])
	if rate == 0 || byteRate == 0 {
		return 0
	}
	return time.Duration(float64(st.Size()-44) / float64(byteRate) * float64(time.Second))
}
