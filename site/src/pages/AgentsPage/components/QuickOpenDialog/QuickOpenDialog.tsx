import { useEffect, useRef, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { useParams } from "react-router";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import { workspaceAgentFindFiles } from "#/api/queries/workspaceFiles";
import {
	Command,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { useDebouncedValue } from "#/hooks/debounce";
import { isMac } from "#/utils/platform";
import {
	useWorkspaceFiles,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";
import { useFilesPanelRoot } from "../RightPanel/FilesPanel";

/** Opens a workspace file by fuzzy name search on Cmd+P (macOS) or Ctrl+P. */
export const QuickOpenDialog: React.FC = () => {
	const workspaceFiles = useWorkspaceFiles();
	if (!workspaceFiles) {
		return null;
	}
	return <QuickOpen workspaceFiles={workspaceFiles} />;
};

const QuickOpen: React.FC<{ workspaceFiles: WorkspaceFilesContextValue }> = ({
	workspaceFiles,
}) => {
	const { agentId, repoRoots, openFile } = workspaceFiles;
	const [open, setOpen] = useState(false);
	const [query, setQuery] = useState("");
	const trimmedQuery = query.trim();
	const debouncedQuery = useDebouncedValue(trimmedQuery, 150);
	// The chat route's id keys the Files panel root, so a root picked there
	// is searched too. "/" would scan the whole filesystem.
	const { agentId: chatId } = useParams();
	const { root: panelRoot } = useFilesPanelRoot(chatId, repoRoots);
	const searchRoots =
		panelRoot === "/" || repoRoots.includes(panelRoot)
			? repoRoots
			: [...repoRoots, panelRoot];
	const roots = searchRoots.length > 0 ? searchRoots : [""];
	const findQuery = useQuery({
		...workspaceAgentFindFiles(agentId, roots, debouncedQuery),
		enabled: open && debouncedQuery !== "",
		placeholderData: keepPreviousData,
	});
	// The dialog has no Radix trigger, so focus returns to the element that
	// had it when the shortcut fired. Opening a file skips this because the
	// file tab takes focus.
	const openerRef = useRef<HTMLElement | null>(null);
	const openedFileRef = useRef(false);

	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			const isModifierPressed = isMac() ? event.metaKey : event.ctrlKey;
			if (
				!isModifierPressed ||
				event.altKey ||
				event.shiftKey ||
				event.key.toLowerCase() !== "p"
			) {
				return;
			}
			event.preventDefault();
			if (
				openerRef.current === null &&
				document.activeElement instanceof HTMLElement
			) {
				openerRef.current = document.activeElement;
			}
			setOpen(true);
		};
		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, []);

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		if (!nextOpen) {
			setQuery("");
		}
	};

	const handleCloseAutoFocus = (event: Event) => {
		event.preventDefault();
		if (!openedFileRef.current) {
			openerRef.current?.focus();
		}
		openerRef.current = null;
		openedFileRef.current = false;
	};

	// Results for an older query stay visible while the new one loads, but
	// cannot be opened, so Enter never opens a file the input no longer matches.
	const isStale =
		trimmedQuery !== debouncedQuery || findQuery.isPlaceholderData;
	const files =
		trimmedQuery === "" || findQuery.isError
			? []
			: (findQuery.data?.results.filter((result) => !result.is_dir) ?? []);

	let status: string | undefined;
	if (trimmedQuery === "") {
		status = "Type to search files.";
	} else if (findQuery.isError && !isStale) {
		status =
			getErrorStatus(findQuery.error) === 404
				? "Update the workspace agent to search files."
				: getErrorMessage(findQuery.error, "Failed to search files.");
	} else if (files.length === 0) {
		status =
			isStale || findQuery.isPending ? "Searching..." : "No matching files.";
	}

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent
				className="max-w-xl gap-0 overflow-hidden p-0"
				onCloseAutoFocus={handleCloseAutoFocus}
			>
				<DialogTitle className="sr-only">Open file</DialogTitle>
				<Command shouldFilter={false} label="Search files">
					<CommandInput
						placeholder="Search files by name"
						value={query}
						onValueChange={setQuery}
					/>
					<CommandList>
						{status !== undefined && (
							<div className="py-6 text-center text-sm text-content-secondary">
								{status}
							</div>
						)}
						{isStale && files.length > 0 && (
							<div
								role="status"
								className="px-3 py-1 text-xs text-content-secondary"
							>
								Refreshing...
							</div>
						)}
						{files.map((result) => (
							<CommandItem
								key={result.absolute_path}
								value={result.absolute_path}
								disabled={isStale}
								onSelect={() => {
									openedFileRef.current = true;
									openFile(result.absolute_path);
									handleOpenChange(false);
								}}
								className="flex-col items-start gap-0"
							>
								<span className="font-semibold text-content-primary">
									{getPathBasename(result.path)}
								</span>
								<span className="truncate text-xs font-normal text-content-secondary">
									{roots.length > 1
										? `${getPathBasename(result.root)}/${result.path}`
										: result.path}
								</span>
							</CommandItem>
						))}
					</CommandList>
				</Command>
			</DialogContent>
		</Dialog>
	);
};
