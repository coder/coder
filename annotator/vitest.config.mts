import { defineConfig } from "vitest/config";

export default defineConfig({
	resolve: {
		alias: {
			react: new URL("./src/reactShim.ts", import.meta.url).pathname,
		},
	},
	test: {
		environment: "jsdom",
		include: ["src/**/*.test.ts"],
	},
});
