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
	walkFilePath,
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
	/** Files already known to match; several open the picker directly. */
	choices?: readonly WorkspaceAgentFindFilesResult[];
	children: React.ReactNode;
};

/**
 * A file path mentioned in chat text. Clicking opens the file; relative
 * paths are resolved against the chat's repositories first.
 */
export const FileMention: React.FC<FileMentionProps> = ({
	path,
	line,
	choices,
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
		if (choices && choices.length > 1) {
			setMatches(choices);
			setTruncated(false);
			return;
		}
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

// Prose such as "and/or" makes candidates whose directories do not exist.
// Remembering the failure keeps every such candidate from listing again.
const missingDirectoryStaleTime = 30_000;

type FileMentionCandidateProps = {
	/** Text starting with a path-like token, possibly with spaces in it. */
	text: string;
	/** The verified path must cover the whole text, as in inline code. */
	whole: boolean;
	/** Today's rendering of the text, shown until a path is verified. */
	children: React.ReactNode;
};

/**
 * Text that may be a file path containing spaces. While it is on screen
 * and the message has finished streaming, directory listings decide how far
 * the path extends; only the verified part becomes a link.
 *
 * The walk is derived in render from the listings it needs, so it follows
 * listing refreshes and stops requesting as soon as it is not rendered.
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
	const [directories, setDirectories] = useState<readonly string[]>([]);
	const available = files !== undefined;

	useEffect(() => {
		const element = ref.current;
		if (!available || !element) {
			return;
		}
		const observer = new IntersectionObserver((entries) => {
			const latest = entries[entries.length - 1];
			if (latest) {
				setVisible(latest.isIntersecting);
			}
		});
		observer.observe(element);
		return () => {
			observer.disconnect();
			setVisible(false);
		};
	}, [available]);

	const agentId = files?.agentId ?? "";
	// Shared listings, observed without fetching, so a refresh from the file
	// tree or elsewhere changes the walk.
	const listings = useQueries({
		queries: directories.map((directory) => ({
			...workspaceAgentDirectory(agentId, directory),
			enabled: false,
		})),
	});
	const checks = useQueries({
		queries: directories.map((directory) => ({
			queryKey: [
				...workspaceAgentDirectoryKey(agentId, directory),
				"verifyPath",
			],
			queryFn: () =>
				queryClient
					.fetchQuery({
						...workspaceAgentDirectory(agentId, directory),
						staleTime: directoryStaleTime,
					})
					.then(
						(listing) => listing.contents,
						() => null,
					),
			enabled: available && visible && !streaming,
			staleTime: (query: { state: { data: unknown } }) =>
				query.state.data === null
					? missingDirectoryStaleTime
					: directoryStaleTime,
			retry: false,
		})),
	});

	const getEntries = (directory: string) => {
		const index = directories.indexOf(directory);
		if (index === -1) {
			return undefined;
		}
		return listings[index].data?.contents ?? checks[index].data;
	};
	const starts = files ? getFilePathCandidateStarts(text, files.repoRoots) : [];
	const walks = starts.map((start) =>
		walkFilePath(start.directory, text.slice(start.offset), getEntries),
	);
	const needed = [...new Set(walks.flatMap((walk) => walk.directories))];
	if (needed.join("\n") !== directories.join("\n")) {
		setDirectories(needed);
	}

	if (!files) {
		return children;
	}

	// Equally long matches in different repositories are ambiguous, so the
	// mention offers all of them.
	let end = 0;
	let line: number | undefined;
	let choices: WorkspaceAgentFindFilesResult[] = [];
	for (const [index, walk] of walks.entries()) {
		const verified = walk.verified;
		if (!verified) {
			continue;
		}
		const walkEnd = starts[index].offset + verified.length;
		if (walkEnd < end) {
			continue;
		}
		if (walkEnd > end) {
			end = walkEnd;
			line = verified.line;
			choices = [];
		}
		if (
			!choices.some((choice) => choice.absolute_path === verified.absolutePath)
		) {
			const root = starts[index].root;
			choices.push({
				absolute_path: verified.absolutePath,
				path: verified.absolutePath.slice(root.replace(/\/$/, "").length + 1),
				root,
				is_dir: false,
				score: 0,
			});
		}
	}
	const mentions = whole ? [] : findFilePathMentions(text);
	const leadEnd = mentions[0]?.start === 0 ? mentions[0].end : 0;
	const extended =
		choices.length > 0 && (whole ? end === text.length : end > leadEnd);
	const lead = extended && (
		<FileMention
			key="lead"
			path={choices.length > 1 ? text.slice(0, end) : choices[0].absolute_path}
			line={line}
			choices={choices.length > 1 ? choices : undefined}
		>
			{whole ? children : text.slice(0, end)}
		</FileMention>
	);

	if (whole) {
		return <span ref={ref}>{lead || children}</span>;
	}

	// The text renders from the same keyed segments before and after
	// verification, so links and text nodes that do not change keep their
	// DOM nodes, focus, and selection.
	const segments: React.ReactNode[] = [];
	let offset = 0;
	if (lead) {
		segments.push(lead);
		offset = end;
	}
	for (const mention of mentions) {
		if (mention.start < offset) {
			continue;
		}
		if (mention.start > offset) {
			segments.push(text.slice(offset, mention.start));
		}
		segments.push(
			<FileMention
				key={mention.start === 0 ? "lead" : `mention-${mention.start}`}
				path={mention.path}
				line={mention.line}
			>
				{text.slice(mention.start, mention.end)}
			</FileMention>,
		);
		offset = mention.end;
	}
	if (offset < text.length) {
		segments.push(text.slice(offset));
	}
	return <span ref={ref}>{segments}</span>;
};
