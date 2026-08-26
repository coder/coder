export const reactCompilerTargetDirs = [
	"src/pages/AgentsPage",
	"src/pages/AIBridgePage",
];

const escapeRegExp = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export const reactCompilerInclude = reactCompilerTargetDirs.map(
	(dir) => new RegExp(`/${escapeRegExp(dir)}/`),
);
