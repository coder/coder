module.exports = {
  projects: [
    {
      coverageReporters: ["text", "lcov"],
      displayName: "test",
      preset: "ts-jest",
      roots: ["<rootDir>/site"],
      transform: {
        "^.+\\.tsx?$": "ts-jest",
      },
      testEnvironment: "jsdom",
      testRegex: "(/__tests__/.*|(\\.|/)(test|spec))\\.tsx?$",
      testPathIgnorePatterns: ["/node_modules/", "/__tests__/fakes"],
      moduleDirectories: ["node_modules", "<rootDir>"],
    },
  ],
  collectCoverageFrom: [
    "<rootDir>/site/**/*.js",
    "<rootDir>/site/**/*.ts",
    "<rootDir>/site/**/*.tsx",
    "!<rootDir>/site/**/*.stories.tsx",
    "!<rootDir>/site/.next/**/*.*",
    "!<rootDir>/site/next-env.d.ts",
    "!<rootDir>/site/next.config.js",
  ],
}
