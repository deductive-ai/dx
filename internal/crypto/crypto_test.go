package crypto

import (
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple string", "hello world"},
		{"api key", "test-fixture-data-456"},
		{"special characters", "p@$$w0rd!#%^&*()"},
		{"unicode", "こんにちは世界"},
		{"long string", "a]very long string that exceeds typical short values and contains multiple words and sentences to test larger payloads"},
		{"whitespace", "  spaces  and\ttabs\nnewlines  "},
		{"single char", "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt(%q) returned error: %v", tt.plaintext, err)
			}

			if encrypted == tt.plaintext {
				t.Errorf("Encrypt(%q) returned plaintext unchanged", tt.plaintext)
			}

			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() returned error: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt(Encrypt(%q)) = %q, want %q", tt.plaintext, decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncrypt_EmptyString(t *testing.T) {
	encrypted, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt(\"\") returned error: %v", err)
	}
	if encrypted != "" {
		t.Errorf("Encrypt(\"\") = %q, want empty string", encrypted)
	}
}

func TestDecrypt_EmptyString(t *testing.T) {
	decrypted, err := Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt(\"\") returned error: %v", err)
	}
	if decrypted != "" {
		t.Errorf("Decrypt(\"\") = %q, want empty string", decrypted)
	}
}

func TestDecrypt_NonEncryptedString_LegacySupport(t *testing.T) {
	legacy := "plain-api-key-without-encryption"
	decrypted, err := Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt(%q) returned error: %v", legacy, err)
	}
	if decrypted != legacy {
		t.Errorf("Decrypt(%q) = %q, want original string (legacy passthrough)", legacy, decrypted)
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"encrypted string", "enc:SGVsbG8gV29ybGQ=", true},
		{"plain string", "plain-api-key", false},
		{"empty string", "", false},
		{"just prefix", "enc:", true},
		{"partial prefix", "en", false},
		{"different prefix", "ENC:data", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEncrypted(tt.s)
			if got != tt.want {
				t.Errorf("IsEncrypted(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestEncrypt_HasPrefix(t *testing.T) {
	encrypted, err := Encrypt("test-data")
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}
	if !IsEncrypted(encrypted) {
		t.Errorf("Encrypt() result %q does not have encrypted prefix", encrypted)
	}
}

func TestEncrypt_DifferentPlaintexts_DifferentCiphertexts(t *testing.T) {
	enc1, err := Encrypt("plaintext-one")
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}
	enc2, err := Encrypt("plaintext-two")
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}
	if enc1 == enc2 {
		t.Error("different plaintexts produced identical ciphertexts")
	}
}

func TestEncrypt_SamePlaintext_DifferentCiphertexts(t *testing.T) {
	enc1, err := Encrypt("same-data")
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}
	enc2, err := Encrypt("same-data")
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}
	if enc1 == enc2 {
		t.Error("same plaintext encrypted twice produced identical ciphertexts (nonce should differ)")
	}

	// Both should decrypt to the same value
	dec1, err := Decrypt(enc1)
	if err != nil {
		t.Fatalf("Decrypt(enc1) error: %v", err)
	}
	dec2, err := Decrypt(enc2)
	if err != nil {
		t.Fatalf("Decrypt(enc2) error: %v", err)
	}
	if dec1 != dec2 {
		t.Errorf("same plaintext decrypted to different values: %q vs %q", dec1, dec2)
	}
}

func TestDecrypt_CorruptedData(t *testing.T) {
	_, err := Decrypt("enc:not-valid-base64!!!")
	if err == nil {
		t.Error("Decrypt() with corrupted data should return error")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	_, err := Decrypt("enc:AA==")
	if err == nil {
		t.Error("Decrypt() with truncated ciphertext should return error")
	}
}
