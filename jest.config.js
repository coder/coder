const maxWorkers = process.env.CI ? 16 : 2

module.exports = {
  maxWorkers,
  projects: [
    {
      coverageReporters: ["text", "lcov"],

      displayName: "test",
      preset: "ts-jest",
      roots: [
        "<rootDir>/site",
      ],
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
    "<rootDir>/site/src/**/*.js",
    "<rootDir>/site/src/**/*.ts",
    "<rootDir>/site/src/**/*.tsx",
    "!<rootDir>/site/src/**/*.stories.tsx",
  ]
}