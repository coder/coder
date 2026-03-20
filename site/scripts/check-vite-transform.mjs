import { createServer } from "vite";

const server = await createServer({
	configFile: "./vite.config.mts",
	server: { middlewareMode: true },
});

try {
	const result = await server.transformRequest(
		"src/pages/AgentsPage/AgentDetail.tsx",
	);
	const code = result?.code ?? "";
	const slots = code.match(/const \$ = _c\(\d+\)/g) || [];
	console.log(`AgentDetail.tsx: ${slots.length} compiled functions`);
	slots.forEach((s) => console.log(`  ${s}`));

	// Check if AgentDetail component itself is compiled
	// Look for the component function and see if it has cache slots
	const hasAgentDetailCache =
		code.includes("const AgentDetail") && slots.length > 0;
	console.log(`\nAgentDetail component compiled: ${hasAgentDetailCache}`);

	// Count total cache slots
	const totalSlots = slots.reduce((sum, s) => {
		const m = s.match(/_c\((\d+)\)/);
		return sum + (m ? Number.parseInt(m[1]) : 0);
	}, 0);
	console.log(`Total cache slots: ${totalSlots}`);
} finally {
	await server.close();
}
