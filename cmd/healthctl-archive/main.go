// healthctl-archive streams a password-encrypted desktop backup. It reads the
// password from descriptor 3 so it never appears in arguments or in the dump.
package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/scrypt"
)

const magic = "healthctl-desktop-backup-1\n"
const chunkSize = 64 * 1024

func cryptStream(src io.Reader, dst io.Writer, password []byte, seal bool) error {
	var salt [32]byte
	if seal {
		if _, err := rand.Read(salt[:]); err != nil {
			return err
		}
		if _, err := io.WriteString(dst, magic); err != nil {
			return err
		}
		if _, err := dst.Write(salt[:]); err != nil {
			return err
		}
	} else {
		header := make([]byte, len(magic))
		if _, err := io.ReadFull(src, header); err != nil || string(header) != magic {
			return errors.New("unrecognized backup format")
		}
		if _, err := io.ReadFull(src, salt[:]); err != nil {
			return err
		}
	}
	key, err := scrypt.Key(password, salt[:], 1<<16, 8, 1, 32)
	if err != nil {
		return err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	// The random 256-bit salt derives a new file key. Frame numbers prevent
	// reordering. An authenticated empty final frame detects truncation.
	var nonce [12]byte
	var size [4]byte
	buf := make([]byte, chunkSize+aead.Overhead())
	for i := uint64(0); ; i++ {
		binary.BigEndian.PutUint64(nonce[4:], i)
		if seal {
			n, err := io.ReadFull(src, buf[:chunkSize])
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return err
			}
			ciphertext := aead.Seal(buf[:0], nonce[:], buf[:n], []byte(magic))
			binary.BigEndian.PutUint32(size[:], uint32(len(ciphertext)))
			if _, err = dst.Write(size[:]); err != nil {
				return err
			}
			if _, err = dst.Write(ciphertext); err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
		} else {
			if _, err = io.ReadFull(src, size[:]); err != nil {
				return err
			}
			n := int(binary.BigEndian.Uint32(size[:]))
			if n < aead.Overhead() || n > len(buf) {
				return errors.New("invalid backup frame")
			}
			if _, err = io.ReadFull(src, buf[:n]); err != nil {
				return err
			}
			plain, err := aead.Open(buf[:0], nonce[:], buf[:n], []byte(magic))
			if err != nil {
				return errors.New("incorrect passphrase or damaged backup")
			}
			if len(plain) == 0 {
				var extra [1]byte
				_, err := io.ReadFull(src, extra[:])
				if err != io.EOF {
					return errors.New("unexpected trailing data")
				}
				return nil
			}
			if _, err = dst.Write(plain); err != nil {
				return err
			}
		}
	}
}
func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "seal" && os.Args[1] != "open") {
		return errors.New("choose seal or open")
	}
	input := os.NewFile(3, "passphrase")
	if input == nil {
		return errors.New("missing passphrase input")
	}
	defer input.Close()
	pass, err := bufio.NewReader(io.LimitReader(input, 1025)).ReadBytes('\n')
	if err != nil && err != io.EOF {
		return err
	}
	pass = bytes.TrimSuffix(pass, []byte("\n"))
	defer clear(pass)
	if len(pass) < 12 || len(pass) > 1024 {
		return errors.New("choose a passphrase of 12–1024 bytes")
	}
	return cryptStream(os.Stdin, os.Stdout, pass, os.Args[1] == "seal")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
