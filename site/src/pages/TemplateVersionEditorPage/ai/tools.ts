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
) {
	const { onFileEdited, onFileDeleted } = callbacks;

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
				const buildPromise = Promise.race([
					callbacks.waitForBuildComplete(),
					new Promise<BuildResult>((resolve) =>
						setTimeout(
							() =>
								resolve({
									status: "timeout",
									error: "Build timed out after 3 minutes.",
									logs: "",
								}),
							180_000,
						),
					),
				]);
				const result = await buildPromise;
				hasBuiltInCurrentRunRef.current = result.status === "succeeded";
				return result;
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
