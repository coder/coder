import type { Config } from "@react-router/dev/config";
import { getUrlPaths } from "./src/content.server.ts";

export default {
	appDirectory: "src",
	ssr: false,
	async prerender() {
		// Data requests omit the trailing slash, but static hosts serve these
		// pages as directories. Prerender both forms.
		const paths = new Set<string>(["/"]);
		for (const urlPath of getUrlPaths()) {
			if (urlPath === "") {
				continue;
			}
			paths.add(`/${urlPath}`);
			paths.add(`/${urlPath}/`);
		}
		return [...paths];
	},
} satisfies Config;
