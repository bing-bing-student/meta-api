package guard

import (
	"encoding/binary"
	"errors"
	"testing"
)

// TestDecodeEnvelope_BadInput 覆盖最常见的输入异常。
func TestDecodeEnvelope_BadInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    []byte
		wantErr error
	}{
		{"empty", nil, errEnvBodyEmpty},
		{"too short", make([]byte, envHeaderLen+envHmacLen-1), errEnvBodyShort},
		{"bad magic", makeFakeEnvelope(t, [4]byte{1, 2, 3, 4}, 0x01, 0x10, 0), errEnvMagicMismatch},
		{"bad version", makeFakeEnvelope(t, envelopeMagic, 0x02, 0x10, 0), errEnvVersionMismatch},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeEnvelope(c.body)
			if err == nil {
				t.Fatalf("expected error %v, got nil", c.wantErr)
			}
			if c.wantErr != nil && !errorIs(err, c.wantErr) {
				t.Fatalf("expected %v, got %v", c.wantErr, err)
			}
		})
	}
}

// TestDecodeEnvelope_OK 构造一个 ciphertext 长度为 0 的最小信封并校验解析结果。
func TestDecodeEnvelope_OK(t *testing.T) {
	body := makeFakeEnvelope(t, envelopeMagic, envelopeVersion, byte(SceneViewLog), 0)
	ev, err := decodeEnvelope(body)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if ev.Scene != byte(SceneViewLog) {
		t.Fatalf("scene mismatch: got %x", ev.Scene)
	}
	if len(ev.RsaCiphertext) != envRsaLen {
		t.Fatalf("rsa len: got %d", len(ev.RsaCiphertext))
	}
	if len(ev.IV) != envIVLen || len(ev.Tag) != envTagLen {
		t.Fatalf("iv/tag len mismatch")
	}
	if len(ev.Ciphertext) != 0 {
		t.Fatalf("ciphertext len mismatch")
	}
	if len(ev.HMAC) != envHmacLen {
		t.Fatalf("hmac len mismatch")
	}
	if len(ev.PrefixForHmac) != envHeaderLen {
		t.Fatalf("prefix len: got %d, want %d", len(ev.PrefixForHmac), envHeaderLen)
	}
}

// TestParseTLV_RoundTrip 写入 + 读回一组字段，再验证 end marker 行为。
func TestParseTLV_RoundTrip(t *testing.T) {
	in := map[uint8][]byte{
		FieldScene:       {byte(SceneViewLog)},
		FieldTimestampMs: encUint64BE(1700000000000),
		FieldNonce:       make([]byte, fieldNonceLen),
		FieldTargetID:    []byte("article-123"),
	}
	buf := encodeTLV(in)
	out, err := parseTLV(buf)
	if err != nil {
		t.Fatalf("parseTLV: %v", err)
	}
	for k, v := range in {
		got, ok := out[k]
		if !ok {
			t.Fatalf("missing field 0x%x", k)
		}
		if string(got) != string(v) {
			t.Fatalf("field 0x%x mismatch: got %x want %x", k, got, v)
		}
	}
}

// TestParseTLV_Truncated 截断的 TLV 必须返回错误。
func TestParseTLV_Truncated(t *testing.T) {
	// id + length 写入但 value 不够长
	buf := []byte{0x01, 0x00, 0x05, 0x00, 0x00}
	if _, err := parseTLV(buf); err == nil {
		t.Fatal("expected truncated error, got nil")
	}
}

// TestParseTLV_ValueTooLarge 单字段长度超过 tlvValueMaxLen 必须返回 errTLVValueTooLarge。
//
// 防御深度：攻击者可能构造 valLen 接近 uint16 上限（65535）的 payload，
// 即使 truncated 校验也能拒绝（因为 payload 没那么长），
// 但显式上限校验让拒因更精确，避免靠"长度对不上"间接拒。
func TestParseTLV_ValueTooLarge(t *testing.T) {
	t.Parallel()

	// 构造 valLen = tlvValueMaxLen + 1，并补够 payload 长度，
	// 让 truncated 校验通过、上限校验先触发。
	const oversize = tlvValueMaxLen + 1
	buf := make([]byte, 3+oversize)
	buf[0] = FieldClientMeta
	buf[1] = byte(oversize >> 8)
	buf[2] = byte(oversize & 0xff)
	// payload 余下部分填 0，保证 i+valLen <= len(payload)

	_, err := parseTLV(buf)
	if err == nil {
		t.Fatal("expected errTLVValueTooLarge, got nil")
	}
	if !errorIs(err, errTLVValueTooLarge) {
		t.Fatalf("expected errTLVValueTooLarge, got %v", err)
	}
}

// ---- helpers ----

// makeFakeEnvelope 构造一个仅长度合法的信封（密码学上无效，仅用来测试 decode 边界）。
func makeFakeEnvelope(t *testing.T, magic [4]byte, version, scene byte, ctLen uint32) []byte {
	t.Helper()
	body := make([]byte, envHeaderLen+int(ctLen)+envHmacLen)
	copy(body[:envMagicLen], magic[:])
	body[envVersionOffset] = version
	body[envSceneOffset] = scene
	binary.BigEndian.PutUint32(body[envCtLenOffset:envCtLenOffset+envCtLenLen], ctLen)
	return body
}

func encodeTLV(in map[uint8][]byte) []byte {
	buf := make([]byte, 0, 64)
	for id, v := range in {
		buf = append(buf, id)
		buf = appendUint16BE(buf, uint16(len(v)))
		buf = append(buf, v...)
	}
	// end marker
	buf = append(buf, FieldEndMarker)
	buf = appendUint16BE(buf, 0)
	return buf
}

func appendUint16BE(b []byte, v uint16) []byte {
	return append(b, byte(v>>8), byte(v))
}

func encUint64BE(v uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, v)
	return out
}

func errorIs(err, target error) bool {
	for err != nil {
		if errors.Is(err, target) {
			return true
		}
		type wrapper interface{ Unwrap() error }
		if w, ok := err.(wrapper); ok {
			err = w.Unwrap()
			continue
		}
		break
	}
	return false
}
