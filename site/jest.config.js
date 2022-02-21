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
      testPathIgnorePatterns: [
        "/.next/",
        "/out/",
        "/_jest/",
        "dev.ts",
        "jest.config.js",
        "jest-runner.*.js",
        "next.config.js",
      ],
    },
  ],
  collectCoverageFrom: [
    "<rootDir>/**/*.js",
    "<rootDir>/**/*.ts",
    "<rootDir>/**/*.tsx",
    "!<rootDir>/**/*.stories.tsx",
    "!<rootDir>/_jest/**/*.*",
    "!<rootDir>/.next/**/*.*",
    "!<rootDir>/api.ts",
    "!<rootDir>/coverage/**/*.*",
    "!<rootDir>/dev.ts",
    "!<rootDir>/jest-runner.eslint.config.js",
    "!<rootDir>/jest.config.js",
    "!<rootDir>/next-env.d.ts",
    "!<rootDir>/next.config.js",
    "!<rootDir>/out/**/*.*",
    "!<rootDir>/storybook-static/**/*.*",
  ],
  reporters: [
    "default",
    [
      "jest-junit",
      {
        suiteName: "Front-end Jest Tests",
        outputDirectory: "./test_results",
        outputName: "junit.xml",
      },
    ],
  ],
}
