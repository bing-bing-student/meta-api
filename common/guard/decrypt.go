package guard

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

// 解密 / 完整性校验相关常量与原语。

// keyMaterial 是 RSA-OAEP 解密 RsaCiphertext 后得到的明文 blob 的视图。
//
// blob 长度严格为 32 + 12 + 16 = 60 字节，依次是 aes_key / iv / nonce_seed。
// 与前端 envelope.rs 中拼装顺序保持一致。
type keyMaterial struct {
	AESKey    [aesKeyLen]byte
	IV        [envIVLen]byte
	NonceSeed [nonceSeedLen]byte
}

const keyMaterialLen = aesKeyLen + envIVLen + nonceSeedLen

// parseKeyMaterial 从 RSA-OAEP 解出的明文 blob 中拆解出三段。
// 入参长度不正确时返回 ok=false。
func parseKeyMaterial(blob []byte) (keyMaterial, bool) {
	var km keyMaterial
	if len(blob) != keyMaterialLen {
		return km, false
	}
	copy(km.AESKey[:], blob[:aesKeyLen])
	copy(km.IV[:], blob[aesKeyLen:aesKeyLen+envIVLen])
	copy(km.NonceSeed[:], blob[aesKeyLen+envIVLen:])
	return km, true
}

// aesGcmOpen 用 AES-256-GCM 解密。
//
// 与前端约定：AES-GCM 的 ciphertext 与 tag 是分开存放的（tag 在信封头中），
// 这里需要把它们拼回去再交给 cipher.AEAD 解密。
func aesGcmOpen(key, iv, tag, ciphertext []byte) ([]byte, error) {
	if len(key) != aesKeyLen {
		return nil, errors.New("aes key length mismatch")
	}
	if len(iv) != envIVLen {
		return nil, errors.New("aes iv length mismatch")
	}
	if len(tag) != envTagLen {
		return nil, errors.New("aes tag length mismatch")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	combined := make([]byte, 0, len(ciphertext)+len(tag))
	combined = append(combined, ciphertext...)
	combined = append(combined, tag...)
	return aead.Open(nil, iv, combined, nil)
}

// verifyHMAC 校验信封 HMAC。
//
// HMAC 输入 = 信封前缀（不含 32B 尾部 HMAC）|| build_hash(8B)。
// build_hash 必须出自一个被 buildHashRegistry 接受的版本。
//
// 返回 (true, hash)：校验通过，hash 是命中的 build_hash；
// 返回 (false, nil)：所有候选 build_hash 都不匹配。
func verifyHMAC(prefix, hmacBytes, aesKey []byte, candidates [][]byte) (bool, []byte) {
	if len(hmacBytes) != envHmacLen {
		return false, nil
	}
	for _, bh := range candidates {
		if len(bh) != fieldBuildHashLen {
			continue
		}
		mac := hmac.New(sha256.New, aesKey)
		mac.Write(prefix)
		mac.Write(bh)
		expected := mac.Sum(nil)
		if hmac.Equal(expected, hmacBytes) {
			return true, bh
		}
	}
	return false, nil
}
