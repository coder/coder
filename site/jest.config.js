// REMARK: Jest is supposed to never exceed 50% maxWorkers by default. However,
//         there seems to be an issue with this in our Ubuntu-based workspaces.
//         If we don't limit it, then 100% CPU and high MEM usage is hit
//         unexpectedly, leading to OOM kills.
//
// SEE thread: https://github.com/coder/coder/pull/483#discussion_r829636583
const maxWorkers = 2

module.exports = {
  maxWorkers,
  projects: [
    {
      globals: {
        "ts-jest": {
          tsconfig: "./tsconfig.test.json",
        },
      },
      coverageReporters: ["text", "lcov"],
      displayName: "test",
      preset: "ts-jest",

      roots: ["<rootDir>"],
      setupFilesAfterEnv: ["./jest.setup.ts"],
      transform: {
        "^.+\\.tsx?$": "ts-jest",
      },
      testEnvironment: "jsdom",
      testRegex: "(/__tests__/.*|(\\.|/)(test|spec))\\.tsx?$",
      testPathIgnorePatterns: ["/node_modules/", "/__tests__/fakes", "/e2e/"],
      moduleDirectories: ["node_modules", "<rootDir>/src"],
      moduleNameMapper: {
        "\\.css$": "<rootDir>/src/testHelpers/styleMock.ts",
      },
    },
    {
      displayName: "lint",
      runner: "jest-runner-eslint",
      testMatch: ["<rootDir>/**/*.js", "<rootDir>/**/*.ts", "<rootDir>/**/*.tsx"],
      testPathIgnorePatterns: ["/out/", "/_jest/", "jest.config.js", "jest-runner.*.js"],
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
  reporters: [
    "default",
    [
      "jest-junit",
      {
        suiteName: "Front-end Jest Tests",
        outputDirectory: "./test-results",
        outputName: "junit.xml",
      },
    ],
  ],
}
