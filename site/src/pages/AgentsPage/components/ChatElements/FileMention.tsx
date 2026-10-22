import { isAxiosError } from "axios";
import { cn } from "cn";
import { LoaderIcon } from "lucide-react";
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useQueries, useQueryClient } from "react-query";
import type { WorkspaceAgentFindFilesResult } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import {
	workspaceAgentDirectory,
	workspaceAgentDirectoryKey,
	workspaceAgentFindFiles,
} from "#/api/queries/workspaceFiles";
import {
	Command,
	CommandEmpty,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { toast } from "#/components/Toaster/toast";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import {
	findFilePathMentions,
	getFilePathCandidateStarts,
	type VerifiedFilePath,
	verifyFilePath,
} from "../../utils/filePathMentions";
import { getPathBasename } from "../../utils/path";

// The agent's maximum; common names such as README.md can match many files.
const findFilesLimit = 200;

/**
 * Keeps the search results the mention names. A result whose root-relative
 * path equals the mention wins over results that only end with it, so
 * coderd/x.go does not also offer enterprise/coderd/x.go.
 */
const pickMentionedFiles = (
	results: readonly WorkspaceAgentFindFilesResult[],
	relativePath: string,
	inHome: boolean,
) => {
	const candidates = results.filter((result) => !result.is_dir);
	const exact = candidates.filter(
		(result) =>
			result.path === relativePath || result.absolute_path === relativePath,
	);
	if (exact.length > 0 || inHome) {
		return exact;
	}
	return candidates.filter((result) =>
		result.absolute_path.endsWith(`/${relativePath}`),
	);
};

type FileMentionProps = {
	/** Absolute, `~/`-prefixed, or relative path without a line suffix. */
	path: string;
	line?: number;
	children: React.ReactNode;
};

/**
 * A file path mentioned in chat text. Clicking opens the file; relative
 * paths are resolved against the chat's repositories first.
 */
export const FileMention: React.FC<FileMentionProps> = ({
	path,
	line,
	children,
}) => {
	const files = useWorkspaceFiles();
	const queryClient = useQueryClient();
	const [pending, setPending] = useState(false);
	const [matches, setMatches] = useState<
		readonly WorkspaceAgentFindFilesResult[]
	>([]);
	const [truncated, setTruncated] = useState(false);
	// Identifies the latest lookup; unmounting or a new agent or path makes
	// an in-flight lookup stale so it neither opens a file nor toasts.
	const lookupRef = useRef(0);
	const agentId = files?.agentId;
	useEffect(() => {
		const lookups = lookupRef;
		return () => {
			lookups.current++;
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
				workspaceAgentFindFiles(
					files.agentId,
					roots,
					relativePath,
					findFilesLimit,
				),
			);
			if (lookup !== lookupRef.current) {
				return;
			}
			const found = pickMentionedFiles(results, relativePath, inHome);
			setPending(false);
			if (found.length === 1) {
				files.openFile(found[0].absolute_path, { line });
			} else if (found.length > 1) {
				setMatches(found);
				setTruncated(results.length >= findFilesLimit);
			} else {
				toast.error(`No file matching ${path} in this chat's repositories`);
			}
		} catch (error) {
			if (lookup !== lookupRef.current) {
				return;
			}
			setPending(false);
			if (isAxiosError(error) && error.response?.status === 404) {
				toast.error("Update the workspace agent to open files");
			} else {
				toast.error(getErrorMessage(error, `Failed to find ${path}.`));
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
				className="w-96 max-w-[90vw] p-0"
			>
				<Command label={`Files matching ${path}`}>
					<CommandInput placeholder="Filter matching files" />
					<CommandList>
						<CommandEmpty>No matching files</CommandEmpty>
						{matches.map((match) => {
							const rootName = getPathBasename(match.root);
							return (
								<CommandItem
									key={match.absolute_path}
									value={match.absolute_path}
									aria-label={`${match.path} in ${rootName}`}
									className="items-baseline py-1.5"
									onSelect={() => {
										setMatches([]);
										files.openFile(match.absolute_path, { line });
									}}
								>
									<span className="min-w-0 truncate font-mono text-xs text-content-primary">
										{match.path}
									</span>
									<span className="ml-auto shrink-0 text-xs font-normal">
										{rootName}
									</span>
								</CommandItem>
							);
						})}
					</CommandList>
					<p className="m-0 border-0 border-t border-solid border-border px-3 py-1.5 text-xs text-content-secondary">
						{truncated
							? `Showing ${matches.length} matches from the first ${findFilesLimit} search results. Mention a longer path to narrow them down.`
							: `${matches.length} files match ${path}.`}
					</p>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

/**
 * True while the enclosing message is still streaming. Candidates wait for
 * the final text before they list directories.
 */
export const FileMentionStreamingContext = createContext(false);

// Directory listings change rarely while a chat is read; reuse them across
// candidates instead of refetching per mention.
const directoryStaleTime = 60_000;

type FileMentionCandidateProps = {
	/** Text starting with a path-like token, possibly with spaces in it. */
	text: string;
	/** The verified path must cover the whole text, as in inline code. */
	whole: boolean;
	/** Today's rendering of the text, shown until a path is verified. */
	children: React.ReactNode;
};

/**
 * Text that may be a file path containing spaces. Once it is on screen and
 * the message has finished streaming, directory listings decide how far
 * the path extends; only the verified part becomes a link.
 */
export const FileMentionCandidate: React.FC<FileMentionCandidateProps> = ({
	text,
	whole,
	children,
}) => {
	const files = useWorkspaceFiles();
	const streaming = useContext(FileMentionStreamingContext);
	const queryClient = useQueryClient();
	const ref = useRef<HTMLSpanElement>(null);
	const [visible, setVisible] = useState(false);
	const available = files !== undefined;

	useEffect(() => {
		const element = ref.current;
		if (!available || visible || !element) {
			return;
		}
		const observer = new IntersectionObserver((entries) => {
			if (entries.some((entry) => entry.isIntersecting)) {
				setVisible(true);
			}
		});
		observer.observe(element);
		return () => observer.disconnect();
	}, [available, visible]);

	const agentId = files?.agentId ?? "";
	const starts = files ? getFilePathCandidateStarts(text, files.repoRoots) : [];
	const walks = useQueries({
		queries: starts.map((start) => {
			const rest = text.slice(start.offset);
			return {
				queryKey: [
					...workspaceAgentDirectoryKey(agentId, start.directory),
					"verifyPath",
					rest,
				],
				queryFn: () =>
					verifyFilePath(start.directory, rest, async (directory) => {
						const listing = await queryClient.fetchQuery({
							...workspaceAgentDirectory(agentId, directory),
							staleTime: directoryStaleTime,
						});
						return listing.contents;
					}),
				enabled: available && visible && !streaming,
				staleTime: directoryStaleTime,
				retry: false,
			};
		}),
	});

	if (!files) {
		return children;
	}

	let best: VerifiedFilePath | undefined;
	let end = 0;
	for (const [index, walk] of walks.entries()) {
		const walkEnd = walk.data ? starts[index].offset + walk.data.length : 0;
		if (walk.data && walkEnd > end) {
			best = walk.data;
			end = walkEnd;
		}
	}
	if (!best || (whole && end !== text.length)) {
		return <span ref={ref}>{children}</span>;
	}
	if (whole) {
		return (
			<FileMention path={best.absolutePath} line={best.line}>
				{children}
			</FileMention>
		);
	}

	const tail: React.ReactNode[] = [];
	let offset = end;
	for (const mention of findFilePathMentions(text)) {
		if (mention.start < end) {
			continue;
		}
		tail.push(text.slice(offset, mention.start));
		tail.push(
			<FileMention key={mention.start} path={mention.path} line={mention.line}>
				{text.slice(mention.start, mention.end)}
			</FileMention>,
		);
		offset = mention.end;
	}
	tail.push(text.slice(offset));

	return (
		<>
			<FileMention path={best.absolutePath} line={best.line}>
				{text.slice(0, end)}
			</FileMention>
			{tail}
		</>
	);
};
