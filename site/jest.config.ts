module.exports = {
  // Use a big timeout for CI.
  testTimeout: 20_000,
  maxWorkers: 8,
  projects: [
    {
      displayName: "test",
      roots: ["<rootDir>"],
      setupFilesAfterEnv: ["./jest.setup.ts"],
      extensionsToTreatAsEsm: [".ts"],
      transform: {
        "^.+\\.(t|j)sx?$": [
          "@swc/jest",
          {
            jsc: {
              transform: {
                react: {
                  runtime: "automatic",
                  importSource: "@emotion/react",
                },
              },
              experimental: {
                plugins: [["jest_workaround", {}]],
              },
            },
          },
        ],
      },
      testEnvironment: "jsdom",
      testRegex: "(/__tests__/.*|(\\.|/)(test|spec))\\.tsx?$",
      testPathIgnorePatterns: ["/node_modules/", "/e2e/"],
      transformIgnorePatterns: [
        "<rootDir>/node_modules/@chartjs-adapter-date-fns",
      ],
      moduleDirectories: ["node_modules", "<rootDir>/src"],
      moduleNameMapper: {
        "\\.css$": "<rootDir>/src/testHelpers/styleMock.ts",
      },
    },
    {
      displayName: "lint",
      runner: "jest-runner-eslint",
      testMatch: [
        "<rootDir>/**/*.js",
        "<rootDir>/**/*.ts",
        "<rootDir>/**/*.tsx",
      ],
      testPathIgnorePatterns: [
        "/out/",
        "/_jest/",
        "jest.config.js",
        "jest-runner.*.js",
      ],
    },
  ],
  collectCoverageFrom: [
    // included files
    "<rootDir>/**/*.ts",
    "<rootDir>/**/*.tsx",
    // excluded files
    "!<rootDir>/**/*.stories.tsx",
    "!<rootDir>/_jest/**/*.*",
    "!<rootDir>/api.ts",
    "!<rootDir>/coverage/**/*.*",
    "!<rootDir>/e2e/**/*.*",
    "!<rootDir>/jest-runner.eslint.config.js",
    "!<rootDir>/jest.config.js",
    "!<rootDir>/out/**/*.*",
    "!<rootDir>/storybook-static/**/*.*",
  ],
};
