import { isAxiosError } from "axios";
import { cn } from "cn";
import { LoaderIcon } from "lucide-react";
import { type FC, type ReactNode, useEffect, useRef, useState } from "react";
import { useQueryClient } from "react-query";
import type { WorkspaceAgentFindFilesResult } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import { workspaceAgentFindFiles } from "#/api/queries/workspaceFiles";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { toast } from "#/components/Toaster/toast";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";

type FileMentionProps = {
	/** Absolute, `~/`-prefixed, or relative path without a line suffix. */
	path: string;
	line?: number;
	children: ReactNode;
};

/**
 * A file path mentioned in chat text. Clicking opens the file; relative
 * paths are resolved against the chat's repositories first.
 */
export const FileMention: FC<FileMentionProps> = ({ path, line, children }) => {
	const files = useWorkspaceFiles();
	const queryClient = useQueryClient();
	const [pending, setPending] = useState(false);
	const [matches, setMatches] = useState<
		readonly WorkspaceAgentFindFilesResult[]
	>([]);
	// Identifies the latest lookup; unmounting or a new agent or path makes
	// an in-flight lookup stale so it neither opens a file nor toasts.
	const lookupRef = useRef(0);
	const agentId = files?.agentId;
	useEffect(() => {
		return () => {
			lookupRef.current++;
			setPending(false);
			setMatches([]);
		};
	}, [agentId, path]);

	if (!files) {
		return children;
	}

	const resolve = async () => {
		if (path.startsWith("/")) {
			files.openFile(path, { line });
			return;
		}
		const inHome = path.startsWith("~/");
		const relativePath = inHome ? path.slice(2) : path.replace(/^\.\//, "");
		let roots: readonly string[] = [""];
		if (inHome) {
			roots = ["~"];
		} else if (files.repoRoots.length > 0) {
			roots = files.repoRoots;
		}
		setPending(true);
		const lookup = ++lookupRef.current;
		try {
			const { results } = await queryClient.fetchQuery(
				workspaceAgentFindFiles(files.agentId, roots, relativePath),
			);
			if (lookup !== lookupRef.current) {
				return;
			}
			const found = results.filter(
				(result) =>
					!result.is_dir &&
					(inHome
						? result.path === relativePath
						: result.absolute_path === relativePath ||
							result.absolute_path.endsWith(`/${relativePath}`)),
			);
			if (found.length === 1) {
				files.openFile(found[0].absolute_path, { line });
			} else if (found.length > 1) {
				setMatches(found);
			} else {
				toast.error(`No file matching ${path} in this chat's repositories`);
			}
		} catch (error) {
			if (lookup !== lookupRef.current) {
				return;
			}
			if (isAxiosError(error) && error.response?.status === 404) {
				toast.error("Update the workspace agent to open files");
			} else {
				toast.error(getErrorMessage(error, `Failed to find ${path}.`));
			}
		} finally {
			if (lookup === lookupRef.current) {
				setPending(false);
			}
		}
	};

	return (
		<Popover
			open={matches.length > 1}
			onOpenChange={(open) => {
				if (!open) {
					setMatches([]);
				}
			}}
		>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-busy={pending}
					className={cn(
						"inline cursor-pointer border-0 bg-transparent p-0 text-left font-[inherit] text-inherit",
						"underline decoration-dotted decoration-content-secondary underline-offset-2 hover:decoration-content-link",
						pending && "cursor-wait",
					)}
					onClick={(event) => {
						// The trigger's own toggle would open an empty picker.
						event.preventDefault();
						if (event.detail <= 1 && !pending && matches.length === 0) {
							void resolve();
						}
					}}
				>
					{children}
					{pending && (
						<LoaderIcon
							aria-hidden
							className="ml-0.5 inline size-3 animate-spin align-middle motion-reduce:animate-none"
						/>
					)}
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="start"
				aria-label={`Files matching ${path}`}
				className="w-auto max-w-md p-1"
			>
				<ul className="m-0 list-none p-0">
					{matches.map((match) => (
						<li key={match.absolute_path}>
							<button
								type="button"
								aria-label={`${match.path} in ${getPathBasename(match.root)}`}
								className="flex w-full cursor-pointer items-baseline gap-2 rounded border-0 bg-transparent px-2 py-1 text-left text-content-primary hover:bg-surface-secondary focus-visible:bg-surface-secondary"
								onClick={() => {
									setMatches([]);
									files.openFile(match.absolute_path, { line });
								}}
							>
								<span className="min-w-0 truncate font-mono text-xs">
									{match.path}
								</span>
								<span className="shrink-0 text-xs text-content-secondary">
									{getPathBasename(match.root)}
								</span>
							</button>
						</li>
					))}
				</ul>
			</PopoverContent>
		</Popover>
	);
};
