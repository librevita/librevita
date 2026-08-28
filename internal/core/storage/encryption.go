package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"librevita.org/internal/core/crypto"
)

const (
	encryptedFileVersion     byte = crypto.DefaultEncryptionVersion
	encryptedFileChunkSize        = 64 << 10
	encryptedFileHeaderSize       = 4 + 1 + crypto.SizeNonce
	encryptedFrameHeaderSize      = 4
)

var encryptedFileMagic = [4]byte{'L', 'V', 'F', 'E'}

var ErrInvalidEncryptedObject = errors.New("storage: invalid encrypted object")

// EncryptedSize returns the encoded size of a payload with the chunked
// XChaCha20-Poly1305 file envelope. A negative input means the size is
// unknown and returns -1.
func EncryptedSize(plaintextSize int64) int64 {
	if plaintextSize < 0 {
		return -1
	}
	if plaintextSize == 0 {
		return encryptedFileHeaderSize
	}
	chunks := (plaintextSize + encryptedFileChunkSize - 1) / encryptedFileChunkSize
	return encryptedFileHeaderSize + plaintextSize +
		chunks*int64(encryptedFrameHeaderSize+crypto.SizeAuthTag)
}

// EncryptedReader encodes a plaintext reader as independently authenticated
// chunks. It does not buffer the complete file.
type EncryptedReader struct {
	source    io.Reader
	key       []byte
	aad       []byte
	baseNonce []byte
	header    []byte
	headerPos int
	frame     []byte
	framePos  int
	counter   uint64
	done      bool
	err       error
}

// NewEncryptedReader creates a streaming encrypted object reader.
func NewEncryptedReader(source io.Reader, key, aad []byte) (*EncryptedReader, error) {
	if source == nil {
		return nil, errors.New("storage: encrypted reader source is nil")
	}
	if len(key) != crypto.SizeDEK {
		return nil, crypto.ErrInvalidDEK
	}
	baseNonce, err := crypto.RandomBytes(crypto.SizeNonce)
	if err != nil {
		return nil, fmt.Errorf("storage: encrypted nonce: %w", err)
	}
	header := make([]byte, encryptedFileHeaderSize)
	copy(header, encryptedFileMagic[:])
	header[4] = encryptedFileVersion
	copy(header[5:], baseNonce)
	return &EncryptedReader{
		source:    source,
		key:       append([]byte(nil), key...),
		aad:       append([]byte(nil), aad...),
		baseNonce: baseNonce,
		header:    header,
	}, nil
}

func (r *EncryptedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.headerPos < len(r.header) {
		n := copy(p, r.header[r.headerPos:])
		r.headerPos += n
		return n, nil
	}
	if r.framePos == len(r.frame) {
		if r.done {
			r.finish()
			return 0, io.EOF
		}
		if err := r.fillFrame(); err != nil {
			r.err = err
			r.finish()
			return 0, err
		}
		if r.done && len(r.frame) == 0 {
			r.finish()
			return 0, io.EOF
		}
	}
	n := copy(p, r.frame[r.framePos:])
	r.framePos += n
	if r.framePos == len(r.frame) {
		crypto.ZeroBytes(r.frame)
		r.frame = nil
		r.framePos = 0
	}
	return n, nil
}

// Close zeroes key material and closes the source when it is closable.
func (r *EncryptedReader) Close() error {
	r.finish()
	if closer, ok := r.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (r *EncryptedReader) fillFrame() error {
	buf := make([]byte, encryptedFileChunkSize)
	defer crypto.ZeroBytes(buf)
	n, err := io.ReadFull(r.source, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("storage: read plaintext: %w", err)
	}
	if n == 0 {
		r.done = true
		return nil
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		r.done = true
	}

	plain := buf[:n]
	length := make([]byte, encryptedFrameHeaderSize)
	if n > encryptedFileChunkSize {
		return ErrInvalidEncryptedObject
	}
	binary.BigEndian.PutUint32(length, uint32(n)) // #nosec G115 -- n is bounded by encryptedFileChunkSize
	nonce := chunkNonce(r.baseNonce, r.counter)
	aead, err := crypto.NewAEADCipher(r.key)
	if err != nil {
		return fmt.Errorf("storage: encrypted chunk init: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plain, frameAAD(r.aad, r.counter, length))
	r.frame = make([]byte, encryptedFrameHeaderSize+len(ciphertext))
	copy(r.frame, length)
	copy(r.frame[encryptedFrameHeaderSize:], ciphertext)
	r.counter++
	return nil
}

func (r *EncryptedReader) finish() {
	r.done = true
	crypto.ZeroBytes(r.key)
	crypto.ZeroBytes(r.aad)
	crypto.ZeroBytes(r.baseNonce)
	crypto.ZeroBytes(r.header)
	crypto.ZeroBytes(r.frame)
	r.key = nil
	r.aad = nil
	r.baseNonce = nil
	r.header = nil
	r.frame = nil
}

// DecryptedReader decodes and authenticates a chunked encrypted object.
type DecryptedReader struct {
	source     io.Reader
	key        []byte
	aad        []byte
	baseNonce  []byte
	pending    []byte
	pendingPos int
	counter    uint64
	done       bool
	err        error
}

// NewDecryptedReader validates the envelope header and creates a streaming
// plaintext reader.
func NewDecryptedReader(source io.Reader, key, aad []byte) (*DecryptedReader, error) {
	if source == nil {
		return nil, errors.New("storage: decrypted reader source is nil")
	}
	if len(key) != crypto.SizeDEK {
		return nil, crypto.ErrInvalidDEK
	}
	header := make([]byte, encryptedFileHeaderSize)
	if _, err := io.ReadFull(source, header); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrInvalidEncryptedObject, err)
	}
	if string(header[:4]) != string(encryptedFileMagic[:]) || header[4] != encryptedFileVersion {
		return nil, ErrInvalidEncryptedObject
	}
	return &DecryptedReader{
		source:    source,
		key:       append([]byte(nil), key...),
		aad:       append([]byte(nil), aad...),
		baseNonce: append([]byte(nil), header[5:]...),
	}, nil
}

func (r *DecryptedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	for r.pendingPos == len(r.pending) && !r.done {
		if err := r.fillFrame(); err != nil {
			r.err = err
			r.finish()
			return 0, err
		}
	}
	if r.pendingPos == len(r.pending) {
		r.finish()
		return 0, io.EOF
	}
	n := copy(p, r.pending[r.pendingPos:])
	r.pendingPos += n
	if r.pendingPos == len(r.pending) {
		crypto.ZeroBytes(r.pending)
		r.pending = nil
		r.pendingPos = 0
	}
	return n, nil
}

// Close zeroes key material and closes the encrypted source.
func (r *DecryptedReader) Close() error {
	r.finish()
	if closer, ok := r.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (r *DecryptedReader) fillFrame() error {
	var lengthBytes [encryptedFrameHeaderSize]byte
	n, err := io.ReadFull(r.source, lengthBytes[:])
	if errors.Is(err, io.EOF) && n == 0 {
		r.done = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: frame header: %v", ErrInvalidEncryptedObject, err)
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length == 0 || length > encryptedFileChunkSize {
		return ErrInvalidEncryptedObject
	}
	ciphertext := make([]byte, int(length)+crypto.SizeAuthTag)
	if _, err := io.ReadFull(r.source, ciphertext); err != nil {
		return fmt.Errorf("%w: frame payload: %v", ErrInvalidEncryptedObject, err)
	}
	nonce := chunkNonce(r.baseNonce, r.counter)
	aead, err := crypto.NewAEADCipher(r.key)
	if err != nil {
		return fmt.Errorf("storage: decrypted chunk init: %w", err)
	}
	plain, err := aead.Open(nil, nonce, ciphertext, frameAAD(r.aad, r.counter, lengthBytes[:]))
	if err != nil || len(plain) != int(length) {
		if plain != nil {
			crypto.ZeroBytes(plain)
		}
		if err == nil {
			err = ErrInvalidEncryptedObject
		}
		return fmt.Errorf("%w: frame authentication: %v", ErrInvalidEncryptedObject, err)
	}
	r.pending = plain
	r.counter++
	return nil
}

func (r *DecryptedReader) finish() {
	r.done = true
	crypto.ZeroBytes(r.key)
	crypto.ZeroBytes(r.aad)
	crypto.ZeroBytes(r.baseNonce)
	crypto.ZeroBytes(r.pending)
	r.key = nil
	r.aad = nil
	r.baseNonce = nil
	r.pending = nil
}

func chunkNonce(base []byte, counter uint64) []byte {
	nonce := append([]byte(nil), base...)
	binary.BigEndian.PutUint64(nonce[len(nonce)-8:], counter)
	return nonce
}

func frameAAD(aad []byte, counter uint64, length []byte) []byte {
	out := make([]byte, len(aad)+8+len(length))
	copy(out, aad)
	binary.BigEndian.PutUint64(out[len(aad):], counter)
	copy(out[len(aad)+8:], length)
	return out
}
