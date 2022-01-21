module.exports = {
  projects: [
    {
      coverageReporters: ["text", "lcov"],
      displayName: "test",
      preset: "ts-jest",
      roots: ["<rootDir>/site"],
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
      testMatch: ["<rootDir>/site/**/*.js", "<rootDir>/site/**/*.ts", "<rootDir>/site/**/*.tsx"],
      testPathIgnorePatterns: ["/.next/", "/out/"],
    },
  ],
  collectCoverageFrom: [
    "<rootDir>/site/**/*.js",
    "<rootDir>/site/**/*.ts",
    "<rootDir>/site/**/*.tsx",
    "!<rootDir>/site/**/*.stories.tsx",
    "!<rootDir>/site/.next/**/*.*",
    "!<rootDir>/site/api.ts",
    "!<rootDir>/site/dev.ts",
    "!<rootDir>/site/next-env.d.ts",
    "!<rootDir>/site/next.config.js",
    "!<rootDir>/site/out/**/*.*",
  ],
}
