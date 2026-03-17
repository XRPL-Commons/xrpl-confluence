// Generates XRPL validator keypairs for use in the Kurtosis test harness.
// Run: go run ./scripts/keygen/main.go
package main

import (
	"encoding/hex"
	"fmt"
	"log"

	"github.com/LeJamon/goXRPLd/codec/addresscodec"
	"github.com/LeJamon/goXRPLd/crypto"
	"github.com/LeJamon/goXRPLd/crypto/secp256k1"
)

func main() {
	for i := 0; i < 5; i++ {
		seed, err := crypto.RandomSeed()
		if err != nil {
			log.Fatalf("Error generating seed: %v", err)
		}

		encodedSeed, err := addresscodec.EncodeSeed(seed, secp256k1.SECP256K1())
		if err != nil {
			log.Fatalf("Error encoding seed: %v", err)
		}

		_, pubKeyHex, err := secp256k1.SECP256K1().DeriveValidatorKeypair(seed)
		if err != nil {
			log.Fatalf("Error deriving keypair: %v", err)
		}

		pubKeyBytes, err := hex.DecodeString(pubKeyHex)
		if err != nil {
			log.Fatalf("Error decoding public key hex: %v", err)
		}

		validatorPubKey, err := addresscodec.EncodeNodePublicKey(pubKeyBytes)
		if err != nil {
			log.Fatalf("Error encoding node public key: %v", err)
		}

		fmt.Printf("    {\"seed\": \"%s\", \"pubkey\": \"%s\"},\n", encodedSeed, validatorPubKey)
	}
}
