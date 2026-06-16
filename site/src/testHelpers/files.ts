/**
 * Builds a File for tests and stories. The size argument controls the byte
 * length of the file contents, which is what attachment UIs read; pass it when
 * a case asserts on formatted file sizes.
 */
export const createMockFile = (name: string, type: string, size = 9): File =>
	new File([new Uint8Array(size)], name, { type });
