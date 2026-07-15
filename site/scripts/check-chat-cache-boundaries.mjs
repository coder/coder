import { readFile, readdir } from "node:fs/promises";
import { extname, join } from "node:path";

const roots = ["src"];
const agentsPageRoot = "src/pages/AgentsPage";
const keyPattern = /\bchatQueryKeys\.|\[\s*["']chats["']/;
const obsoletePattern =
	/\b(?:chatsKey|chatKey|chatMessagesKey|chatPromptsKey|chatACLKey|infiniteChatsKey|readInfiniteChatsCache|chatDebugRunsKey|chatDebugRunKey|chatDiffContentsKey|ChatStore|ChatStoreState|createChatStore|useChatStore|chatStore)\b/;
const ignoredSuffixes = [".test.ts", ".test.tsx", ".stories.ts", ".stories.tsx"];

const violations = [];

async function visit(path) {
	for (const entry of await readdir(path, { withFileTypes: true })) {
		const child = join(path, entry.name);
		if (entry.isDirectory()) {
			await visit(child);
			continue;
		}
		if (![".ts", ".tsx"].includes(extname(entry.name))) {
			continue;
		}
		if (ignoredSuffixes.some((suffix) => entry.name.endsWith(suffix))) {
			continue;
		}
		const lines = (await readFile(child, "utf8")).split("\n");
		for (const [index, line] of lines.entries()) {
			if (
				(child.startsWith(agentsPageRoot) && keyPattern.test(line)) ||
				obsoletePattern.test(line)
			) {
				violations.push(`${child}:${index + 1}: ${line.trim()}`);
			}
		}
	}
}

for (const root of roots) {
	await visit(root);
}

if (violations.length > 0) {
	console.error(
		[
			"Direct chat query-key access is restricted to src/api/queries/chats.ts.",
			"Use a query option factory or typed chat cache operation instead:",
			...violations,
		].join("\n"),
	);
	process.exitCode = 1;
}
