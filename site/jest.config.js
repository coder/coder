module.exports = {
  projects: [
    {
      globals: {
        "ts-jest": {
          tsconfig: "tsconfig.test.json",
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
      moduleDirectories: ["node_modules", "<rootDir>"],
    },
    {
      displayName: "lint",
      runner: "jest-runner-eslint",
      testMatch: ["<rootDir>/**/*.js", "<rootDir>/**/*.ts", "<rootDir>/**/*.tsx"],
      testPathIgnorePatterns: ["/out/", "/_jest/", "jest.config.js", "jest-runner.*.js"],
    },
  ],
  collectCoverageFrom: [
    "<rootDir>/**/*.js",
    "<rootDir>/**/*.ts",
    "<rootDir>/**/*.tsx",
    "!<rootDir>/**/*.stories.tsx",
    "!<rootDir>/_jest/**/*.*",
    "!<rootDir>/api.ts",
    "!<rootDir>/coverage/**/*.*",
    "!<rootDir>/e2e/**/*.*",
    "!<rootDir>/jest-runner.eslint.config.js",
    "!<rootDir>/jest.config.js",
    "!<rootDir>/webpack.*.ts",
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
