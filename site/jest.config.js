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
      setupFilesAfterEnv: ["<rootDir>/_jest/setupTests.ts"],
      transform: {
        "^.+\\.tsx?$": "ts-jest",
      },
      testEnvironment: "jsdom",
      testRegex: "(/__tests__/.*|(\\.|/)(test|spec))\\.tsx?$",
      testPathIgnorePatterns: ["/node_modules/", "/__tests__/fakes"],
      moduleDirectories: ["node_modules", "<rootDir>"],
    },
    {
      displayName: "lint",
      runner: "jest-runner-eslint",
      testMatch: ["<rootDir>/**/*.js", "<rootDir>/**/*.ts", "<rootDir>/**/*.tsx"],
      testPathIgnorePatterns: ["/.next/", "/out/", "/_jest/", "jest.config.js", "jest-runner.*.js", "next.config.js"],
    },
  ],
  collectCoverageFrom: [
    "<rootDir>/**/*.js",
    "<rootDir>/**/*.ts",
    "<rootDir>/**/*.tsx",
    "!<rootDir>/**/*.stories.tsx",
    "!<rootDir>/.next/**/*.*",
    "!<rootDir>/api.ts",
    "!<rootDir>/dev.ts",
    "!<rootDir>/next-env.d.ts",
    "!<rootDir>/next.config.js",
    "!<rootDir>/out/**/*.*",
  ],
}
