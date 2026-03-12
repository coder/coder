import { tool } from "ai";
import type { FileTree } from "utils/filetree";
import {
	createFile,
	existsFile,
	getFileText,
	isFolder,
	removeFile,
	traverse,
	updateFile,
} from "utils/filetree";
import { z } from "zod";

export interface BuildResult {
	status: "succeeded" | "failed" | "canceled" | "timeout";
	error?: string;
	logs: string;
}

export interface BuildOutput {
	status: string;
	error?: string;
	logs: string;
}

export interface PublishResult {
	success: boolean;
	error?: string;
	versionName?: string;
}

export interface PublishRequestData {
	name?: string;
	message?: string;
	isActiveVersion?: boolean;
}

export interface PublishRequestOptions {
	skipDirtyCheck?: boolean;
}

interface TemplateAgentToolCallbacks {
	onFileEdited?: (path: string) => void;
	onFileDeleted?: (path: string) => void;
	onBuildRequested?: () => Promise<void>;
	waitForBuildComplete?: () => Promise<BuildResult>;
	getBuildOutput?: () => BuildOutput | undefined;
	onPublishRequested?: (
		data: PublishRequestData,
		options?: PublishRequestOptions,
	) => Promise<PublishResult>;
}

type CoderDocsRoute = {
	title: string;
	description?: string;
	path?: string;
	children?: CoderDocsRoute[];
};

const coderDocsRouteSchema: z.ZodType<CoderDocsRoute> = z.lazy(() =>
	z
		.object({
			title: z.string(),
			description: z.string().optional(),
			path: z.string().optional(),
			children: z.array(coderDocsRouteSchema).optional(),
		})
		.passthrough(),
);

const coderDocsManifestSchema = z
	.object({
		versions: z.array(z.string()).optional(),
		routes: z.array(coderDocsRouteSchema),
	})
	.passthrough();

type CoderDocsManifest = z.infer<typeof coderDocsManifestSchema>;

const CODER_DOCS_RAW_BASE_URL =
	"https://raw.githubusercontent.com/coder/coder/refs/tags";

const normalizeCoderDocsVersionTag = (version: string): string => {
	const trimmed = version.trim();
	if (trimmed.length === 0) {
		throw new Error("Coder docs version cannot be empty.");
	}

	// Build versions can include prerelease/build metadata
	// (for example v2.31.4-devel+abc123) while release docs are tagged
	// on the base semver. Strip any suffix when present.
	const baseSemverMatch = trimmed.match(/^v\d+\.\d+\.\d+/);
	return baseSemverMatch?.[0] ?? trimmed;
};

const buildCoderDocsManifestURL = (docsTag: string): string =>
	`${CODER_DOCS_RAW_BASE_URL}/${encodeURIComponent(docsTag)}/docs/manifest.json`;

const normalizeCoderDocsPath = (path: string): string => {
	const withoutFragment = path.trim().split("#")[0]?.split("?")[0] ?? "";
	const withoutPrefix = withoutFragment.startsWith("./")
		? withoutFragment.slice(2)
		: withoutFragment;
	if (withoutPrefix.length === 0) {
		throw new Error("Docs path cannot be empty.");
	}
	if (withoutPrefix.startsWith("/") || withoutPrefix.includes("..")) {
		throw new Error(
			"Docs path must be a relative markdown file from coder_docs_outline.",
		);
	}
	if (!withoutPrefix.endsWith(".md")) {
		throw new Error(
			"Docs path must point to a markdown file from coder_docs_outline.",
		);
	}
	return withoutPrefix;
};

const buildCoderDocsFileURL = (
	docsTag: string,
	normalizedPath: string,
): string =>
	`${CODER_DOCS_RAW_BASE_URL}/${encodeURIComponent(docsTag)}/docs/${normalizedPath}`;

const collectCoderDocsMarkdownPaths = (
	routes: CoderDocsRoute[],
	result = new Map<string, string>(),
): Map<string, string> => {
	for (const route of routes) {
		if (route.path?.endsWith(".md")) {
			result.set(normalizeCoderDocsPath(route.path), route.path);
		}
		if (route.children) {
			collectCoderDocsMarkdownPaths(route.children, result);
		}
	}
	return result;
};

const renderCoderDocsOutline = (
	routes: CoderDocsRoute[],
	depth = 0,
): string[] => {
	const lines: string[] = [];
	const indent = "  ".repeat(depth);
	for (const route of routes) {
		const suffix = route.path?.endsWith(".md") ? ` (${route.path})` : "";
		lines.push(`${indent}- ${route.title}${suffix}`);
		if (route.children) {
			lines.push(...renderCoderDocsOutline(route.children, depth + 1));
		}
	}
	return lines;
};

const fetchJSON = async (url: string): Promise<unknown> => {
	if (typeof globalThis.fetch !== "function") {
		throw new Error("Fetch API is unavailable in this browser.");
	}
	const response = await globalThis.fetch(url);
	if (!response.ok) {
		throw new Error(
			`Failed to fetch ${url}: ${response.status} ${response.statusText || "request failed"}.`,
		);
	}
	return response.json();
};

const fetchText = async (url: string): Promise<string> => {
	if (typeof globalThis.fetch !== "function") {
		throw new Error("Fetch API is unavailable in this browser.");
	}
	const response = await globalThis.fetch(url);
	if (!response.ok) {
		throw new Error(
			`Failed to fetch ${url}: ${response.status} ${response.statusText || "request failed"}.`,
		);
	}
	return response.text();
};
/**
 * Creates the set of AI tools that operate on the template editor's
 * in-memory FileTree. Tools use the provided callbacks to read and
 * mutate the tree so that React state stays in sync.
 */
export function createTemplateAgentTools(
	getFileTree: () => FileTree,
	setFileTree: (updater: (prev: FileTree) => FileTree) => void,
	hasBuiltInCurrentRunRef: { current: boolean },
	callbacks: TemplateAgentToolCallbacks = {},
	docsVersion?: string,
) {
	const { onFileEdited, onFileDeleted } = callbacks;
	const docsManifestCache = new Map<string, Promise<CoderDocsManifest>>();
	const docsMarkdownCache = new Map<string, Promise<string>>();

	const getDocsVersionInfo = () => {
		if (docsVersion === undefined || docsVersion.trim().length === 0) {
			return {
				error:
					"Coder docs are unavailable because the deployment build version is unknown.",
			} as const;
		}
		return {
			buildVersion: docsVersion,
			docsTag: normalizeCoderDocsVersionTag(docsVersion),
		} as const;
	};

	const getDocsManifest = async (
		docsTag: string,
	): Promise<CoderDocsManifest> => {
		const cached = docsManifestCache.get(docsTag);
		if (cached) {
			return cached;
		}

		const manifestPromise = fetchJSON(buildCoderDocsManifestURL(docsTag))
			.then((value) => coderDocsManifestSchema.parse(value))
			.catch((error) => {
				docsManifestCache.delete(docsTag);
				throw error;
			});
		docsManifestCache.set(docsTag, manifestPromise);
		return manifestPromise;
	};

	const getDocsMarkdown = async (
		docsTag: string,
		normalizedPath: string,
	): Promise<string> => {
		const cacheKey = `${docsTag}:${normalizedPath}`;
		const cached = docsMarkdownCache.get(cacheKey);
		if (cached) {
			return cached;
		}

		const markdownPromise = fetchText(
			buildCoderDocsFileURL(docsTag, normalizedPath),
		).catch((error) => {
			docsMarkdownCache.delete(cacheKey);
			throw error;
		});
		docsMarkdownCache.set(cacheKey, markdownPromise);
		return markdownPromise;
	};

	return {
		listFiles: tool({
			description:
				"List all files in the template. Always call this first to understand the template structure.",
			inputSchema: z.object({}),
			execute: async () => {
				const files: string[] = [];
				traverse(getFileTree(), (content, _filename, fullPath) => {
					// Only include leaf files, not directories.
					if (typeof content === "string") {
						files.push(fullPath);
					}
				});
				return { files };
			},
		}),

		readFile: tool({
			description:
				"Read the contents of a file. Use this before editing to understand the current content.",
			inputSchema: z.object({
				path: z
					.string()
					.min(1, "Path cannot be empty.")
					.describe("File path relative to template root, e.g. 'main.tf'"),
			}),
			execute: async ({ path }) => {
				const tree = getFileTree();
				if (!existsFile(path, tree)) {
					return {
						error: `File not found: ${path}. Use listFiles to see available files.`,
					};
				}
				try {
					const content = getFileText(path, tree);
					return { content };
				} catch {
					return { error: `${path} is a directory, not a file.` };
				}
			},
		}),

		editFile: tool({
			description:
				"Edit a file by replacing a specific section. To create a new file, set oldContent to an empty string. " +
				"To append to an existing file, set oldContent to empty string. " +
				"For targeted edits, provide enough context in oldContent to uniquely identify the location.",
			inputSchema: z.object({
				path: z
					.string()
					.min(1, "Path cannot be empty.")
					.describe("File path relative to template root"),
				oldContent: z
					.string()
					.describe(
						"Exact text to find and replace (empty string to create/append)",
					),
				newContent: z.string().describe("Replacement text"),
			}),
			needsApproval: true,
			execute: async ({ path, oldContent, newContent }) => {
				const result = executeEditFile(getFileTree, setFileTree, {
					path,
					oldContent,
					newContent,
				});
				if (result.success) {
					hasBuiltInCurrentRunRef.current = false;
					onFileEdited?.(path);
				}
				return result;
			},
		}),

		deleteFile: tool({
			description: "Delete a file from the template.",
			inputSchema: z.object({
				path: z
					.string()
					.min(1, "Path cannot be empty.")
					.describe("File path to delete"),
			}),
			needsApproval: true,
			execute: async ({ path }) => {
				const result = executeDeleteFile(getFileTree, setFileTree, { path });
				if (result.success) {
					hasBuiltInCurrentRunRef.current = false;
					onFileDeleted?.(path);
				}
				return result;
			},
		}),

		coder_docs_outline: tool({
			description:
				"Get an outline of the official Coder documentation for this deployment version. " +
				"Use this to discover relevant markdown file paths before calling coder_docs.",
			inputSchema: z.object({}),
			execute: async () => {
				const versionInfo = getDocsVersionInfo();
				if ("error" in versionInfo) {
					return versionInfo;
				}
				try {
					const manifest = await getDocsManifest(versionInfo.docsTag);
					return {
						buildVersion: versionInfo.buildVersion,
						docsTag: versionInfo.docsTag,
						outline: [
							`Coder docs outline for build version ${versionInfo.buildVersion} (docs tag ${versionInfo.docsTag}).`,
							"Use the exact markdown paths in parentheses with coder_docs.",
							...renderCoderDocsOutline(manifest.routes),
						].join("\n"),
					};
				} catch (err) {
					const message =
						err instanceof Error
							? err.message
							: "Failed to load the Coder docs outline.";
					return { error: message };
				}
			},
		}),

		coder_docs: tool({
			description:
				"Read a markdown file from the official Coder documentation for this deployment version. " +
				"Call coder_docs_outline first to discover valid paths.",
			inputSchema: z.object({
				path: z
					.string()
					.min(1, "Path cannot be empty.")
					.describe(
						"Markdown file path from coder_docs_outline, e.g. './admin/templates/managing-templates/index.md'",
					),
			}),
			execute: async ({ path }) => {
				const versionInfo = getDocsVersionInfo();
				if ("error" in versionInfo) {
					return versionInfo;
				}
				let normalizedPath: string;
				try {
					normalizedPath = normalizeCoderDocsPath(path);
				} catch (err) {
					const message =
						err instanceof Error ? err.message : "Invalid docs path.";
					return { error: message };
				}

				try {
					const manifest = await getDocsManifest(versionInfo.docsTag);
					const markdownPaths = collectCoderDocsMarkdownPaths(manifest.routes);
					const manifestPath = markdownPaths.get(normalizedPath);
					if (!manifestPath) {
						return {
							error: `Docs file not found in the outline: ${path}. Call coder_docs_outline first and use one of the listed markdown paths.`,
						};
					}
					return {
						buildVersion: versionInfo.buildVersion,
						docsTag: versionInfo.docsTag,
						path: manifestPath,
						sourceURL: buildCoderDocsFileURL(
							versionInfo.docsTag,
							normalizedPath,
						),
						markdown: await getDocsMarkdown(
							versionInfo.docsTag,
							normalizedPath,
						),
					};
				} catch (err) {
					const message =
						err instanceof Error
							? err.message
							: "Failed to load the Coder docs page.";
					return { error: message };
				}
			},
		}),

		buildTemplate: tool({
			description:
				"Build the current template to validate Terraform configuration. " +
				"This uploads the files and runs a provisioner job. " +
				"Returns the build status and logs when complete.",
			inputSchema: z.object({}),
			needsApproval: true,
			execute: async () => {
				if (!callbacks.onBuildRequested || !callbacks.waitForBuildComplete) {
					return { error: "Build tools are not available." };
				}
				try {
					await callbacks.onBuildRequested();
				} catch (err) {
					hasBuiltInCurrentRunRef.current = false;
					const message =
						err instanceof Error ? err.message : "Failed to trigger build";
					return { status: "failed" as const, error: message, logs: "" };
				}
				// Start waiting for build completion only after the build request
				// succeeds so failed requests do not leave pending waiters/timers.
				let timeoutID: ReturnType<typeof setTimeout> | undefined;
				try {
					const result = await Promise.race([
						callbacks.waitForBuildComplete(),
						new Promise<BuildResult>((resolve) => {
							timeoutID = setTimeout(
								() =>
									resolve({
										status: "timeout",
										error: "Build timed out after 3 minutes.",
										logs: "",
									}),
								180_000,
							);
						}),
					]);
					hasBuiltInCurrentRunRef.current = result.status === "succeeded";
					return result;
				} finally {
					if (timeoutID !== undefined) {
						clearTimeout(timeoutID);
					}
				}
			},
		}),

		getBuildLogs: tool({
			description:
				"Get the current template build status and logs. " +
				"Use this to inspect build failures, including when " +
				"the user triggered a build manually.",
			inputSchema: z.object({}),
			execute: async () => {
				if (!callbacks.getBuildOutput) {
					return { error: "Build tools are not available." };
				}
				const output = callbacks.getBuildOutput();
				if (!output) {
					return {
						status: "none",
						error: "No build has been run yet.",
						logs: "",
					};
				}
				return output;
			},
		}),

		publishTemplate: tool({
			description:
				"Publish the current template version. The build must have " +
				"succeeded before publishing. Requires user approval. " +
				"If name is omitted, the existing version name is kept.",
			inputSchema: z.object({
				name: z
					.string()
					.optional()
					.describe("Version name (defaults to current version name)"),
				message: z
					.string()
					.optional()
					.describe("Changelog message describing the changes"),
				isActiveVersion: z
					.boolean()
					.optional()
					.default(true)
					.describe("Whether to promote this as the active version"),
			}),
			needsApproval: true,
			execute: async ({ name, message, isActiveVersion }) => {
				if (!callbacks.onPublishRequested) {
					return { success: false, error: "Publish is not available." };
				}
				try {
					return await callbacks.onPublishRequested(
						{
							name,
							message,
							isActiveVersion,
						},
						hasBuiltInCurrentRunRef.current
							? { skipDirtyCheck: true }
							: undefined,
					);
				} catch (err) {
					const errorMessage =
						err instanceof Error ? err.message : "Failed to publish";
					return { success: false, error: errorMessage };
				}
			},
		}),
	};
}

/**
 * Execute the editFile tool logic. Separated from the tool definition
 * so it can be called after user approval.
 */
function executeEditFile(
	getFileTree: () => FileTree,
	setFileTree: (updater: (prev: FileTree) => FileTree) => void,
	args: { path: string; oldContent: string; newContent: string },
): { success: boolean; action?: string; error?: string; path: string } {
	const { path, oldContent, newContent } = args;
	if (path.length === 0) {
		return { success: false, error: "File path cannot be empty.", path };
	}

	const tree = getFileTree();
	const exists = existsFile(path, tree);

	// Create new file. createFile can throw if the path is invalid
	// (e.g. an intermediate segment is an existing file), so we
	// catch and return a structured error instead of breaking the
	// agent loop.
	if (!exists && oldContent === "") {
		try {
			setFileTree((prev) => createFile(path, prev, newContent));
		} catch (err) {
			const message =
				err instanceof Error ? err.message : "Failed to create file";
			return { success: false, error: message, path };
		}
		return { success: true, action: "created", path };
	}

	// Cannot replace content in a file that doesn't exist.
	if (!exists) {
		return {
			success: false,
			error: `File not found: ${path}. Use listFiles first.`,
			path,
		};
	}

	let current: string;
	try {
		current = getFileText(path, tree);
	} catch {
		return {
			success: false,
			error: `${path} is a directory, not a file.`,
			path,
		};
	}

	// Append or write.
	if (oldContent === "") {
		const updated = current.length > 0 ? current + newContent : newContent;
		const action = current.length > 0 ? "appended" : "written";
		setFileTree((prev) => updateFile(path, updated, prev));
		return { success: true, action, path };
	}

	// Search-and-replace: must match exactly once.
	const occurrences = current.split(oldContent).length - 1;
	if (occurrences === 0) {
		return {
			success: false,
			error: `oldContent not found in ${path}. Read the file first to get exact content.`,
			path,
		};
	}
	if (occurrences > 1) {
		return {
			success: false,
			error: `oldContent matches ${occurrences} locations in ${path}. Include more surrounding context to make the match unique.`,
			path,
		};
	}

	setFileTree((prev) =>
		updateFile(path, current.replace(oldContent, newContent), prev),
	);
	return { success: true, action: "edited", path };
}

/**
 * Execute the deleteFile tool logic. Separated from the tool definition
 * so it can be called after user approval.
 */
function executeDeleteFile(
	getFileTree: () => FileTree,
	setFileTree: (updater: (prev: FileTree) => FileTree) => void,
	args: { path: string },
): { success: boolean; error?: string; path: string } {
	const { path } = args;
	if (path.length === 0) {
		return { success: false, error: "File path cannot be empty.", path };
	}

	const tree = getFileTree();
	if (!existsFile(path, tree)) {
		return { success: false, error: `File not found: ${path}`, path };
	}
	if (isFolder(path, tree)) {
		return {
			success: false,
			error: `${path} is a directory, not a file. Delete individual files instead.`,
			path,
		};
	}
	setFileTree((prev) => removeFile(path, prev));
	return { success: true, path };
}
