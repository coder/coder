export const createMockFile = (name: string, type: string, size = 9): File =>
	new File([new Uint8Array(size)], name, { type });

// jsdom's File does not implement Blob.text().
export const readMockFileText = (file: File): Promise<string> =>
	new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(String(reader.result));
		reader.onerror = () => reject(reader.error);
		reader.readAsText(file);
	});
