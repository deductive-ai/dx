// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"runtime"
)

// encryptedPrefix is prepended to encrypted strings to identify them
const encryptedPrefix = "enc:"

// getMachineKey derives a unique key based on machine-specific info
// This ensures encrypted data can only be decrypted on the same machine
func getMachineKey() []byte {
	// Combine machine-specific information
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()
	
	// Create a seed from machine info
	seed := fmt.Sprintf("dai-cli:%s:%s:%s:%s", 
		hostname, 
		homeDir,
		runtime.GOOS,
		runtime.GOARCH,
	)
	
	// Hash to get a 32-byte key for AES-256
	hash := sha256.Sum256([]byte(seed))
	return hash[:]
}

// Encrypt encrypts a plaintext string using AES-GCM
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key := getMachineKey()
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	
	return encryptedPrefix + encoded, nil
}

// Decrypt decrypts an encrypted string using AES-GCM
func Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	// Check if it's actually encrypted
	if len(encrypted) < len(encryptedPrefix) || encrypted[:len(encryptedPrefix)] != encryptedPrefix {
		// Not encrypted (legacy data), return as-is
		return encrypted, nil
	}

	encoded := encrypted[len(encryptedPrefix):]
	
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode: %w", err)
	}

	key := getMachineKey()
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted checks if a string appears to be encrypted
func IsEncrypted(s string) bool {
	return len(s) >= len(encryptedPrefix) && s[:len(encryptedPrefix)] == encryptedPrefix
}
