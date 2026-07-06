package guard

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// 信封头部协议常量。与 portal-web/docs/anti-bot-guard-v1-spec.md §3.1 一一对应，
// 同时与前端 crypto-wasm/src/envelope.rs 中的 MAGIC / VERSION 严格对齐。
//
// 字段布局（Big-Endian）：
//
//	0      4     magic = 0x47 0x55 0x41 0x52  ("GUAR")
//	4      1     version = 0x01
//	5      1     scene
//	6      2     reserved = 0x0000
//	8      256   rsa_encrypted_key            RSA-2048-OAEP(K(32) || iv(12) || nonce_seed(16))
//	264    12    aes_iv
//	276    16    aes_tag
//	292    4     ciphertext_len (uint32 BE)
//	296    N     ciphertext                   AES-256-GCM(payload)
//	296+N  32    build_hmac                   HMAC-SHA256(prefix || build_hash)
const (
	envMagicLen      = 4
	envVersionOffset = 4
	envSceneOffset   = 5
	envRsaOffset     = 8
	envRsaLen        = 256
	envIVOffset      = 264
	envIVLen         = 12
	envTagOffset     = 276
	envTagLen        = 16
	envCtLenOffset   = 292
	envCtLenLen      = 4
	envCtOffset      = 296
	envHeaderLen     = 296
	envHmacLen       = 32
	envMinLen        = envHeaderLen + envHmacLen // 即 ciphertext_len = 0 时的最小信封长度

	// AesKeyLen / NonceSeedLen 仅用于解析 RSA 解密后的拼接 blob。
	aesKeyLen    = 32
	nonceSeedLen = 16
)

// envelopeMagic 与前端 crypto-wasm 的 MAGIC = [0x47, 0x55, 0x41, 0x52] 对齐。
var envelopeMagic = [4]byte{0x47, 0x55, 0x41, 0x52}

// envelopeVersion 与前端 VERSION = 0x01 对齐。
const envelopeVersion byte = 0x01

// TLV field id 集合（与 spec §3.2 / crypto-wasm `field` 模块对齐）。
const (
	FieldScene           uint8 = 0x01
	FieldTimestampMs     uint8 = 0x02
	FieldNonce           uint8 = 0x03
	FieldFingerprintID   uint8 = 0x04
	FieldTargetID        uint8 = 0x05
	FieldClientMeta      uint8 = 0x06
	FieldBehaviorSummary uint8 = 0x07
	FieldBehaviorRaw     uint8 = 0x08
	FieldViewport        uint8 = 0x09
	FieldSessionToken    uint8 = 0x0A
	FieldBuildHash       uint8 = 0x0B
	FieldEndMarker       uint8 = 0xFF
)

// TLV 字段长度约束（仅校验那些固定长度的字段，变长字段不强制）。
const (
	fieldNonceLen         = 16
	fieldFingerprintIDLen = 32
	fieldBuildHashLen     = 8
	fieldTimestampMsLen   = 8
)

// tlvValueMaxLen 单个 TLV value 的最大字节数（防御深度）。
//
// 真实业务字段实际最大值估算：
//   - FieldBehaviorRaw  : 600 events × 6B/event = 3600B
//   - FieldClientMeta   : JSON object，~512B 量级
//   - FieldTargetID     : 短 hex/字符串，<256B
//   - FieldFingerprintID: 32B
//   - 其它固定长度字段  : <=32B
//
// 8 KB 上限远高于上述任何字段，足够吸收未来字段扩展空间，
// 同时低于 MaxBodyBytes=16KB，对单字段构成有意义的兜底约束。
const tlvValueMaxLen = 8 * 1024

// 错误集合
var (
	errEnvBodyEmpty       = errors.New("envelope body empty")
	errEnvBodyTooLarge    = errors.New("envelope body too large")
	errEnvBodyShort       = errors.New("envelope body shorter than header")
	errEnvMagicMismatch   = errors.New("envelope magic mismatch")
	errEnvVersionMismatch = errors.New("envelope version mismatch")
	errEnvCtLenMismatch   = errors.New("envelope ciphertext_len mismatch with body length")
	errEnvCtTooLarge      = errors.New("envelope ciphertext_len exceeds limit")
	errTLVTruncated       = errors.New("tlv truncated")
	errTLVValueTooLarge   = errors.New("tlv value length exceeds limit")
)

// envelopeView 信封解码后的内存视图。
//
// 注意：所有 byte slice 字段都是 *直接引用* 原始 RawBody 上的子切片，
// 不做拷贝（拷贝由调用方按需进行）。RawBody 必须在使用 envelopeView 期间保持存活。
type envelopeView struct {
	Magic         [4]byte
	Version       byte
	Scene         byte
	RsaCiphertext []byte // 长度固定 256
	IV            []byte // 长度固定 12
	Tag           []byte // 长度固定 16
	CiphertextLen uint32
	Ciphertext    []byte
	HMAC          []byte // 长度固定 32

	// PrefixForHmac 是 HMAC 输入的前缀（信封中除最后 32B HMAC 之外的全部字节）。
	PrefixForHmac []byte
}

// decodeEnvelope 仅做"格式与长度"层面的校验，不做密码学校验
func decodeEnvelope(body []byte) (*envelopeView, error) {
	if len(body) == 0 {
		return nil, errEnvBodyEmpty
	}
	if len(body) > MaxBodyBytes {
		return nil, errEnvBodyTooLarge
	}
	if len(body) < envMinLen {
		return nil, errEnvBodyShort
	}

	var ev envelopeView
	copy(ev.Magic[:], body[:envMagicLen])
	if ev.Magic != envelopeMagic {
		return nil, errEnvMagicMismatch
	}
	ev.Version = body[envVersionOffset]
	if ev.Version != envelopeVersion {
		return nil, errEnvVersionMismatch
	}
	ev.Scene = body[envSceneOffset]
	// reserved 2B 暂不校验（前端固定 0x0000，未来扩展再启用）

	ev.RsaCiphertext = body[envRsaOffset : envRsaOffset+envRsaLen]
	ev.IV = body[envIVOffset : envIVOffset+envIVLen]
	ev.Tag = body[envTagOffset : envTagOffset+envTagLen]

	ctLen := binary.BigEndian.Uint32(body[envCtLenOffset : envCtLenOffset+envCtLenLen])
	ev.CiphertextLen = ctLen
	if int64(envCtOffset)+int64(ctLen)+envHmacLen != int64(len(body)) {
		return nil, fmt.Errorf("%w: declared=%d actual=%d", errEnvCtLenMismatch, ctLen, len(body))
	}
	if ctLen > MaxBodyBytes {
		return nil, errEnvCtTooLarge
	}
	ev.Ciphertext = body[envCtOffset : envCtOffset+int(ctLen)]
	hmacOffset := envCtOffset + int(ctLen)
	ev.HMAC = body[hmacOffset : hmacOffset+envHmacLen]
	ev.PrefixForHmac = body[:hmacOffset]
	return &ev, nil
}

// parseTLV 把 plaintext payload 解析成 fieldID -> value map。
//
// 字段顺序由前端按 nonce_seed 派生的随机排列决定，所以这里只按顺序逐条读取，
// 不依赖固定顺序。重复 fieldID 取最后一次（前端不会重复，重复属于异常协议）。
//
// payload 末尾必须以 FieldEndMarker(0xFF) + 0x00 0x00（length=0）结尾。
func parseTLV(payload []byte) (map[uint8][]byte, error) {
	out := make(map[uint8][]byte, 12)
	for i := 0; i < len(payload); {
		// id(1) + len(2)
		if i+3 > len(payload) {
			return nil, errTLVTruncated
		}
		id := payload[i]
		valLen := binary.BigEndian.Uint16(payload[i+1 : i+3])
		i += 3

		if id == FieldEndMarker && valLen == 0 {
			// 容忍 end marker 后还有少量填充字节，但不再继续解析。
			return out, nil
		}

		// 单字段上限校验（防御深度）：合法字段最大也只有几 KB；
		// 攻击者构造畸形 valLen 接近 uint16 上限时提前拒，避免后续逻辑处理超大切片。
		if int(valLen) > tlvValueMaxLen {
			return nil, errTLVValueTooLarge
		}

		if i+int(valLen) > len(payload) {
			return nil, errTLVTruncated
		}
		out[id] = payload[i : i+int(valLen)]
		i += int(valLen)
	}
	// 没有显式 end marker 也认为合法（前端会发，但容错处理）
	return out, nil
}

// reasonForDecodeError 把 envelope 解码错误映射到统一拒因码。
func reasonForDecodeError(err error) string {
	switch {
	case errors.Is(err, errEnvBodyEmpty):
		return ReasonBodyEmpty
	case errors.Is(err, errEnvBodyTooLarge):
		return ReasonBodyTooLarge
	case errors.Is(err, errEnvMagicMismatch):
		return ReasonMagicMismatch
	case errors.Is(err, errEnvVersionMismatch):
		return ReasonVersionMismatch
	default:
		return ReasonEnvDecodeFail
	}
}
