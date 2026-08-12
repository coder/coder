export const createMockFile = (name: string, type: string, size = 9): File =>
	new File([new Uint8Array(size)], name, { type });
