package conf

import (
	"crypto/sha256"
	"fmt"

	"github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"
)

type blockCrypt struct {
	keySize int // required key size; if 0, the entire key is used
	build   func(key []byte) (kcp.BlockCrypt, error)
}

var blockCrypts = map[string]blockCrypt{
	"aes":         {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESBlockCrypt(key) }},
	"aes-128":     {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESBlockCrypt(key) }},
	"aes-128-gcm": {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESGCMCrypt(key) }},
	"aes-192":     {24, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESBlockCrypt(key) }},
	"salsa20":     {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewSalsa20BlockCrypt(key) }},
	"blowfish":    {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewBlowfishBlockCrypt(key) }},
	"twofish":     {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewTwofishBlockCrypt(key) }},
	"cast5":       {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewCast5BlockCrypt(key) }},
	"3des":        {24, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewTripleDESBlockCrypt(key) }},
	"tea":         {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewTEABlockCrypt(key) }},
	"xtea":        {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewXTEABlockCrypt(key) }},
	"xor":         {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewSimpleXORBlockCrypt(key) }},
	"sm4":         {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewSM4BlockCrypt(key) }},
	"none":        {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewNoneBlockCrypt(key) }},
	"null":        {0, func(key []byte) (kcp.BlockCrypt, error) { return nil, nil }},
}

func newBlock(block, key string) (kcp.BlockCrypt, error) {
	dkey := pbkdf2.Key([]byte(key), []byte("paqet"), 100_000, 32, sha256.New)

	if b, ok := blockCrypts[block]; ok {
		bkey := dkey
		if b.keySize > 0 && len(bkey) >= b.keySize {
			bkey = bkey[:b.keySize]
		}
		block, err := b.build(bkey)
		if err != nil {
			return nil, err
		}
		return block, nil
	}

	return nil, fmt.Errorf("unsupported block type: %s", block)
}
