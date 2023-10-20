export const localStorageMock = () => {
  const store = {} as Record<string, string>;

  return {
    getItem: (key: string): string => {
      return store[key];
    },
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    clear: () => {
      Object.keys(store).forEach((key) => {
        delete store[key];
      });
    },
    removeItem: (key: string) => {
      delete store[key];
    },
  };
};

Object.defineProperty(window, "localStorage", { value: localStorageMock() });
