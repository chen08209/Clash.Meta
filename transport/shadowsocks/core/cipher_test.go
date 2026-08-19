package core

import "testing"

func TestPickCipherRC4(t *testing.T) {
	cipher, err := PickCipher("rc4", nil, "password")
	if err != nil {
		t.Fatalf("PickCipher() error = %v", err)
	}

	stream, ok := cipher.(*StreamCipher)
	if !ok {
		t.Fatalf("PickCipher() type = %T, want *StreamCipher", cipher)
	}

	if got := stream.IVSize(); got != 0 {
		t.Fatalf("IVSize() = %d, want 0", got)
	}

	if got := len(stream.Key); got != 16 {
		t.Fatalf("key length = %d, want 16", got)
	}
}
