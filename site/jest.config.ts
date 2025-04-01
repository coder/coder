module.exports = {
	// Use a big timeout for CI.
	testTimeout: 20_000,
	maxWorkers: 8,
	projects: [
		{
			displayName: "test",
			roots: ["<rootDir>"],
			setupFiles: ["./jest.polyfills.js"],
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
			testEnvironment: "jest-fixed-jsdom",
			testEnvironmentOptions: {
				customExportConditions: [""],
			},
			testRegex: "(/__tests__/.*|(\\.|/)(test|spec))\\.tsx?$",
			testPathIgnorePatterns: [
				"/node_modules/",
				"/e2e/",
				// TODO: This test is timing out after upgrade a few Jest dependencies
				// and I was not able to figure out why. When running it specifically, I
				// can see many act warnings that may can help us to find the issue.
				"/usePaginatedQuery.test.ts",
			],
			transformIgnorePatterns: [
				"<rootDir>/node_modules/@chartjs-adapter-date-fns",
			],
			moduleDirectories: ["node_modules", "<rootDir>/src"],
			moduleNameMapper: {
				"\\.css$": "<rootDir>/src/testHelpers/styleMock.ts",
				"^@fontsource": "<rootDir>/src/testHelpers/styleMock.ts",
			},
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
