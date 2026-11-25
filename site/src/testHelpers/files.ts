export const createMockFile = (name: string, type: string, size = 9): File =>
	new File([new Uint8Array(size)], name, { type });

/** A 1x1 coral image, so an inline image part loads without a server. */
export const MockPngBase64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";
