const localStorageMock = (): Storage => {
	const store = new Map<string, string>();

	return {
		getItem: (key) => {
			return store.get(key) ?? null;
		},
		setItem: (key: string, value: string) => {
			store.set(key, value);
		},
		clear: () => {
			store.clear();
		},
		removeItem: (key: string) => {
			store.delete(key);
		},

		get length() {
			return store.size;
		},

		key: (index) => {
			const values = store.values();
			let value: IteratorResult<string, undefined> = values.next();
			for (let i = 1; i < index && !value.done; i++) {
				value = values.next();
			}

			return value.value ?? null;
		},
	};
};

Object.defineProperty(globalThis, "localStorage", {
	value: localStorageMock(),
	writable: false,
});
