package cryptorand

// MustInt63 returns a non-negative random 63-bit integer as a int64.
func MustInt63() int64 {
	i, err := Int63()
	must(err)
	return i
}

// MustUint64 returns a random 64-bit integer as a uint64.
func MustUint64() uint64 {
	i, err := Uint64()
	must(err)
	return i
}

// MustUint32 returns a 32-bit value as a uint32.
func MustUint32() uint32 {
	i, err := Uint32()
	must(err)
	return i
}

// MustInt31 returns a non-negative random 31-bit integer as a int32.
func MustInt31() int32 {
	i, err := Int31()
	must(err)
	return i
}

// MustInt returns a non-negative random integer as a int.
func MustInt() int {
	i, err := Int()
	must(err)
	return i
}

// MustInt63n returns a non-negative random integer in [0,n) as a int64.
func MustInt63n(n int64) int64 {
	i, err := Int63n(n)
	must(err)
	return i
}

// MustInt31n returns a non-negative integer in [0,n) as a int32.
func MustInt31n(n int32) int32 {
	i, err := Int31n(n)
	must(err)
	return i
}

// MustIntn returns a non-negative integer in [0,n) as a int.
func MustIntn(n int) int {
	i, err := Intn(n)
	must(err)
	return i
}

// MustFloat64 returns a random number in [0.0,1.0) as a float64.
func MustFloat64() float64 {
	f, err := Float64()
	must(err)
	return f
}

// MustFloat32 returns a random number in [0.0,1.0) as a float32.
func MustFloat32() float32 {
	f, err := Float32()
	must(err)
	return f
}

// MustBool returns a random true/false value as a bool.
func MustBool() bool {
	b, err := Bool()
	must(err)
	return b
}
