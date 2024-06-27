const { File } = require("node:buffer");

Object.defineProperties(globalThis, {
  File: { value: File },
  matchMedia: {
    value: (query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    }),
  },
});
