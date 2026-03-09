import { Button } from "components/Button/Button";
import {
	CheckIcon,
	FilePenLineIcon,
	Trash2Icon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import { type FC, useEffect, useMemo, useRef } from "react";
import { cn } from "utils/cn";
import {
	existsFile,
	type FileTree,
	getFileText,
	isFolder,
} from "utils/filetree";
import type { DisplayToolCall } from "./useTemplateAgent";

interface EditApprovalCardProps {
	toolCall: DisplayToolCall;
	isPending: boolean;
	onApprove: () => void;
	onReject: () => void;
	onNavigateToFile?: (path: string) => void;
	getFileTree: () => FileTree;
}

type DiffLine = {
	type: "added" | "removed" | "unchanged";
	text: string;
	oldLineNo: number | undefined;
	newLineNo: number | undefined;
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const splitLines = (content: string) => content.split("\n");

/**
 * Find the 1-based line number where oldContent starts in the full file.
 * Returns 1 if the snippet cannot be located.
 */
const findStartLine = (fullContent: string, oldContent: string): number => {
	if (oldContent.length === 0 || fullContent.length === 0) {
		return 1;
	}
	const index = fullContent.indexOf(oldContent);
	if (index === -1) {
		return 1;
	}

	// Count newlines before the match to get the 1-based line number.
	let lineNumber = 1;
	for (let i = 0; i < index; i++) {
		if (fullContent[i] === "\n") {
			lineNumber++;
		}
	}
	return lineNumber;
};

/**
 * Compute a line-level diff between oldContent and newContent
 * using a simple LCS-based algorithm. Returns lines annotated
 * as "unchanged", "removed", or "added".
 */
const buildDiffLines = (
	oldContent: string,
	newContent: string,
	startLine: number,
): DiffLine[] => {
	if (oldContent.length === 0) {
		return splitLines(newContent).map((text, i) => ({
			type: "added",
			text,
			oldLineNo: undefined,
			newLineNo: startLine + i,
		}));
	}
	if (newContent.length === 0) {
		return splitLines(oldContent).map((text, i) => ({
			type: "removed",
			text,
			oldLineNo: startLine + i,
			newLineNo: undefined,
		}));
	}

	const oldLines = splitLines(oldContent);
	const newLines = splitLines(newContent);

	// Build the LCS table.
	const m = oldLines.length;
	const n = newLines.length;
	const dp: number[][] = Array.from({ length: m + 1 }, () =>
		new Array<number>(n + 1).fill(0),
	);
	for (let i = 1; i <= m; i++) {
		for (let j = 1; j <= n; j++) {
			dp[i][j] =
				oldLines[i - 1] === newLines[j - 1]
					? dp[i - 1][j - 1] + 1
					: Math.max(dp[i - 1][j], dp[i][j - 1]);
		}
	}

	// Backtrack to produce the diff.
	const result: DiffLine[] = [];
	let i = m;
	let j = n;
	while (i > 0 || j > 0) {
		if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
			result.push({
				type: "unchanged",
				text: oldLines[i - 1],
				oldLineNo: undefined,
				newLineNo: undefined,
			});
			i--;
			j--;
		} else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
			result.push({
				type: "added",
				text: newLines[j - 1],
				oldLineNo: undefined,
				newLineNo: undefined,
			});
			j--;
		} else {
			result.push({
				type: "removed",
				text: oldLines[i - 1],
				oldLineNo: undefined,
				newLineNo: undefined,
			});
			i--;
		}
	}

	result.reverse();

	let oldLineNo = startLine;
	let newLineNo = startLine;
	for (const line of result) {
		if (line.type === "unchanged") {
			line.oldLineNo = oldLineNo;
			line.newLineNo = newLineNo;
			oldLineNo++;
			newLineNo++;
		} else if (line.type === "removed") {
			line.oldLineNo = oldLineNo;
			oldLineNo++;
		} else {
			line.newLineNo = newLineNo;
			newLineNo++;
		}
	}

	return result;
};

export const EditApprovalCard: FC<EditApprovalCardProps> = ({
	toolCall,
	isPending,
	onApprove,
	onReject,
	onNavigateToFile,
	getFileTree,
}) => {
	const approveRef = useRef<HTMLButtonElement>(null);
	// Move focus to the Approve button when this card first
	// requires user input so keyboard and screen-reader users
	// are immediately aware of the pending action.
	useEffect(() => {
		if (isPending) {
			approveRef.current?.focus();
		}
	}, [isPending]);

	const path = typeof toolCall.args.path === "string" ? toolCall.args.path : "";
	const hasValidPath = path.length > 0;
	const pathLabel = hasValidPath ? path : "(invalid path)";

	const oldContent =
		typeof toolCall.args.oldContent === "string"
			? toolCall.args.oldContent
			: "";
	const newContent =
		typeof toolCall.args.newContent === "string"
			? toolCall.args.newContent
			: "";

	const startLine = useMemo(() => {
		if (!hasValidPath || toolCall.toolName !== "editFile") {
			return 1;
		}
		try {
			const tree = getFileTree();
			if (!existsFile(path, tree) || isFolder(path, tree)) {
				return 1;
			}
			const fullContent = getFileText(path, tree);
			return findStartLine(fullContent, oldContent);
		} catch {
			return 1;
		}
	}, [getFileTree, hasValidPath, oldContent, path, toolCall.toolName]);

	const diffLines = useMemo(() => {
		if (toolCall.toolName !== "editFile") {
			return [];
		}
		return buildDiffLines(oldContent, newContent, startLine);
	}, [newContent, oldContent, startLine, toolCall.toolName]);

	const result = isRecord(toolCall.result) ? toolCall.result : null;
	const resultError = typeof result?.error === "string" ? result.error : null;
	const resultSuccess = result?.success === true;

	const actionLabel =
		toolCall.toolName === "editFile"
			? `Edit file: ${pathLabel}`
			: `Delete file: ${pathLabel}`;

	return (
		<div
			className="space-y-2 rounded-md border border-solid border-border-default p-2.5"
			role="region"
			aria-label={actionLabel}
		>
			<div className="flex items-center gap-2">
				{toolCall.toolName === "editFile" ? (
					<FilePenLineIcon className="size-3.5 text-content-secondary" />
				) : (
					<Trash2Icon className="size-3.5 text-content-destructive" />
				)}
				<button
					type="button"
					onClick={() => {
						if (hasValidPath) {
							onNavigateToFile?.(path);
						}
					}}
					disabled={!onNavigateToFile || !hasValidPath}
					className={cn(
						"border-none bg-transparent p-0 text-left text-xs font-medium",
						"text-content-link hover:underline",
						onNavigateToFile && hasValidPath
							? "cursor-pointer"
							: "cursor-default",
					)}
				>
					{pathLabel}
				</button>
			</div>

			{!hasValidPath && (
				<div className="rounded-md bg-surface-destructive/10 p-2 text-xs text-content-destructive">
					This tool call is missing a valid file path.
				</div>
			)}

			{toolCall.toolName === "editFile" ? (
				<div className="max-h-64 overflow-y-auto rounded-md border border-solid border-border-default">
					{diffLines.length > 0 ? (
						diffLines.map((line, index) => (
							<div
								key={`${line.type}-${index}`}
								className={cn(
									"flex font-mono text-[11px] leading-5",
									line.type === "added" &&
										"bg-surface-green/20 text-content-success",
									line.type === "removed" &&
										"bg-surface-red/20 text-content-destructive",
									line.type === "unchanged" && "text-content-secondary",
								)}
							>
								<span className="inline-block w-8 shrink-0 select-none pr-1 text-right text-content-disabled">
									{line.oldLineNo ?? ""}
								</span>
								<span className="inline-block w-8 shrink-0 select-none pr-1 text-right text-content-disabled">
									{line.newLineNo ?? ""}
								</span>
								<span className="inline-block w-4 shrink-0 select-none text-center">
									{line.type === "added"
										? "+"
										: line.type === "removed"
											? "−"
											: " "}
								</span>
								<span className="flex-1 whitespace-pre-wrap break-all pr-2">
									{line.text}
								</span>
							</div>
						))
					) : (
						<p className="m-0 p-2 text-xs text-content-secondary">
							No content changes.
						</p>
					)}
				</div>
			) : (
				<div className="rounded-md bg-surface-destructive/10 p-2 text-xs text-content-destructive">
					Delete file: {pathLabel}
				</div>
			)}

			{isPending && (
				<div
					className="flex items-center gap-1.5"
					role="group"
					aria-label="Approval actions"
				>
					<Button
						ref={approveRef}
						variant="outline"
						size="sm"
						onClick={onApprove}
					>
						<CheckIcon />
						Approve
					</Button>
					<Button variant="subtle" size="sm" onClick={onReject}>
						<XIcon />
						Reject
					</Button>
				</div>
			)}

			{!isPending && toolCall.state === "pending" && (
				<p className="m-0 text-xs text-content-secondary">
					Waiting for approval…
				</p>
			)}

			{toolCall.state === "result" && resultSuccess && (
				<p className="m-0 text-xs text-content-success">
					{toolCall.toolName === "deleteFile"
						? "File deleted."
						: "Edit applied."}
				</p>
			)}

			{toolCall.state === "result" && resultError && (
				<div className="flex items-start gap-2 rounded-md bg-surface-destructive/10 p-2 text-xs text-content-destructive">
					<TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0" />
					<span>{resultError}</span>
				</div>
			)}
		</div>
	);
};
