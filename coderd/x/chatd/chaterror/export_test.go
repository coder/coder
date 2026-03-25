package chaterror

// ExtractStatusCodeForTest lets external-package tests pin signal extraction
// behavior without exposing the helper in production builds.
func ExtractStatusCodeForTest(lower string) int {
	return extractStatusCode(lower)
}

// DetectProviderForTest lets external-package tests cover provider-detection
// ordering without opening the production API surface.
func DetectProviderForTest(lower string) string {
	return detectProvider(lower)
}
