package agentrsa

import (
	"crypto/rsa"
	"math/big"
	"math/rand"
)

// GenerateDeterministicKey generates an RSA private key deterministically based on the provided seed.
// This function uses a deterministic random source to generate the primes p and q, ensuring that the
// same seed will always produce the same private key. The generated key is 2048 bits in size.
//
// Reference: https://pkg.go.dev/crypto/rsa#GenerateKey
func GenerateDeterministicKey(seed int64) *rsa.PrivateKey {
	// Since the standard lib purposefully does not generate
	// deterministic rsa keys, we need to do it ourselves.

	// Create deterministic random source
	// nolint: gosec
	deterministicRand := rand.New(rand.NewSource(seed))

	// Use fixed values for p and q based on the seed
	p := big.NewInt(0)
	q := big.NewInt(0)
	e := big.NewInt(65537) // Standard RSA public exponent

	for {
		// Generate deterministic primes using the seeded random
		// Each prime should be ~1024 bits to get a 2048-bit key
		for {
			p.SetBit(p, 1024, 1) // Ensure it's large enough
			for i := range 1024 {
				if deterministicRand.Int63()%2 == 1 {
					p.SetBit(p, i, 1)
				} else {
					p.SetBit(p, i, 0)
				}
			}
			p1 := new(big.Int).Sub(p, big.NewInt(1))
			if p.ProbablyPrime(20) && new(big.Int).GCD(nil, nil, e, p1).Cmp(big.NewInt(1)) == 0 {
				break
			}
		}

		for {
			q.SetBit(q, 1024, 1) // Ensure it's large enough
			for i := range 1024 {
				if deterministicRand.Int63()%2 == 1 {
					q.SetBit(q, i, 1)
				} else {
					q.SetBit(q, i, 0)
				}
			}
			q1 := new(big.Int).Sub(q, big.NewInt(1))
			if q.ProbablyPrime(20) && p.Cmp(q) != 0 && new(big.Int).GCD(nil, nil, e, q1).Cmp(big.NewInt(1)) == 0 {
				break
			}
		}

		// Calculate phi = (p-1) * (q-1)
		p1 := new(big.Int).Sub(p, big.NewInt(1))
		q1 := new(big.Int).Sub(q, big.NewInt(1))
		phi := new(big.Int).Mul(p1, q1)

		// Calculate private exponent d
		d := new(big.Int).ModInverse(e, phi)
		if d != nil {
			// Calculate n = p * q
			n := new(big.Int).Mul(p, q)

			// Create the private key
			privateKey := &rsa.PrivateKey{
				PublicKey: rsa.PublicKey{
					N: n,
					E: int(e.Int64()),
				},
				D:      d,
				Primes: []*big.Int{p, q},
			}

			// Compute precomputed values
			privateKey.Precompute()

			return privateKey
		}
	}
}
