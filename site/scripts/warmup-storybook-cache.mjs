// Warm vite's transform cache for storybook story files.
// Only needed on cold cache (first run after pnpm install).
import { createServer } from "vite";
import { readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, "..");

const server = await createServer({
	configFile: join(root, "vite.config.mts"),
	root,
});
await server.listen();

const stories = readdirSync(join(root, "src"), { recursive: true })
	.filter((f) => String(f).endsWith(".stories.tsx"))
	.map((f) => `/src/${f}`);

await Promise.all(
	stories.map((f) =>
		server.environments.client.warmupRequest(f).catch(() => {}),
	),
);

await server.close();
