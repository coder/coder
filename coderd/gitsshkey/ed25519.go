// This file contains an adapted version of the original implementation available
// under the following URL: https://github.com/mikesmitty/edkey/blob/3356ea4e686a1d47ae5d2d4c3cbc1832ce2df626/edkey.go
// The following changes have been made:
// * Replaced usage of math/rand with crypto/rand
// This should be removed soon as support for marshaling ED25519 private keys
// is added to the Golang standard library.
// See: https://github.com/golang/go/issues/37132
// --- BEGIN ORIGINAL LICENSE ---
// MIT License
// Copyright (c) 2017 Michael Smith
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
// --- END ORIGINAL LICENSE ---
package gitsshkey
import (
	"fmt"
	"errors"
	"crypto/rand"
	"encoding/binary"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)
func MarshalED25519PrivateKey(key ed25519.PrivateKey) ([]byte, error) {
	// Add our key header (followed by a null byte)
	magic := append([]byte("openssh-key-v1"), 0)
	var msg struct {
		CipherName   string
		KdfName      string
		KdfOpts      string
		NumKeys      uint32
		PubKey       []byte
		PrivKeyBlock []byte
	}
	// Fill out the private key fields
	pk1 := struct {
		Check1  uint32
		Check2  uint32
		Keytype string
		Pub     []byte
		Priv    []byte
		Comment string
		Pad     []byte `ssh:"rest"`
	}{}
	// Random check bytes
	var check uint32
	if err := binary.Read(rand.Reader, binary.BigEndian, &check); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}
	pk1.Check1 = check
	pk1.Check2 = check
	// Set our key type
	pk1.Keytype = ssh.KeyAlgoED25519
	// Add the pubkey to the optionally-encrypted block
	pk, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("ed25519.PublicKey type assertion failed on an ed25519 public key")
	}
	pubKey := []byte(pk)
	pk1.Pub = pubKey
	// Add our private key
	pk1.Priv = []byte(key)
	// Might be useful to put something in here at some point
	pk1.Comment = ""
	// Add some padding to match the encryption block size within PrivKeyBlock (without Pad field)
	// 8 doesn't match the documentation, but that's what ssh-keygen uses for unencrypted keys. *shrug*
	bs := 8
	blockLen := len(ssh.Marshal(pk1))
	padLen := (bs - (blockLen % bs)) % bs
	pk1.Pad = make([]byte, padLen)
	// Padding is a sequence of bytes like: 1, 2, 3...
	for i := 0; i < padLen; i++ {
		pk1.Pad[i] = byte(i + 1)
	}
	// Generate the pubkey prefix "\0\0\0\nssh-ed25519\0\0\0 "
	prefix := []byte{0x0, 0x0, 0x0, 0x0b}
	prefix = append(prefix, []byte(ssh.KeyAlgoED25519)...)
	prefix = append(prefix, []byte{0x0, 0x0, 0x0, 0x20}...)
	prefix = append(prefix, pubKey...)
	// Only going to support unencrypted keys for now
	msg.CipherName = "none"
	msg.KdfName = "none"
	msg.KdfOpts = ""
	msg.NumKeys = 1
	msg.PubKey = prefix
	msg.PrivKeyBlock = ssh.Marshal(pk1)
	magic = append(magic, ssh.Marshal(msg)...)
	return magic, nil
}
