import { cn } from "cn";
import {
	ChevronDownIcon,
	FolderGit2Icon,
	FolderIcon,
	FolderInputIcon,
	HardDriveIcon,
	HouseIcon,
} from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { getPathBasename } from "../../utils/path";

/** Shows an absolute path under home as `~` or `~/rest`. */
export const abbreviateHomePath = (
	path: string,
	homePath: string | undefined,
): string => {
	if (
		homePath === undefined ||
		homePath === "/" ||
		(path !== homePath && !path.startsWith(`${homePath}/`))
	) {
		return path;
	}
	return `~${path.slice(homePath.length)}`;
};

type RootOption = {
	/** The root in list-directory path form, as stored per chat. */
	root: string;
	label: string;
	description: string;
	title: string;
	icon: React.ReactNode;
};

const iconClassName = "size-3.5 shrink-0";

/**
 * Picker entries in display order: the Filesystem places, the chat's
 * repositories, then the current root when it is none of those (one opened
 * by path).
 */
const getRootOptions = ({
	root,
	rootPath,
	homePath,
	defaultDirectory,
	repoRoots,
}: {
	root: string | undefined;
	rootPath: string | undefined;
	homePath: string | undefined;
	/** Undefined when the default directory is home. */
	defaultDirectory: string | undefined;
	repoRoots: readonly string[];
}) => {
	const filesystem: RootOption[] = [
		{
			root: "/",
			label: "Root",
			description: "/",
			title: "/",
			icon: <HardDriveIcon className={iconClassName} />,
		},
		{
			// One entry serves both forms when home is the default directory, so
			// a stored "" still shows as selected.
			root: defaultDirectory === undefined && root === "" ? "" : "~",
			label: "Home",
			description: "~",
			title: homePath ?? "~",
			icon: <HouseIcon className={iconClassName} />,
		},
	];
	// A default directory that is also a repository is listed once, under
	// Repositories, unless it is the current root in its "" form.
	if (
		defaultDirectory !== undefined &&
		(!repoRoots.includes(defaultDirectory) || root === "")
	) {
		filesystem.push({
			root: "",
			label: getPathBasename(defaultDirectory),
			description: abbreviateHomePath(defaultDirectory, homePath),
			title: defaultDirectory,
			icon: <FolderIcon className={iconClassName} />,
		});
	}
	const repositories = repoRoots.map(
		(path): RootOption => ({
			root: path,
			label: getPathBasename(path),
			description: abbreviateHomePath(path, homePath),
			title: path,
			icon: <FolderGit2Icon className={iconClassName} />,
		}),
	);
	const isListed = [...filesystem, ...repositories].some(
		(option) => option.root === root,
	);
	const resolved = rootPath ?? root ?? "";
	const opened: RootOption | undefined =
		isListed || root === undefined
			? undefined
			: {
					root,
					label: getPathBasename(resolved) || resolved,
					description: abbreviateHomePath(resolved, homePath),
					title: resolved,
					icon: <FolderIcon className={iconClassName} />,
				};
	return { filesystem, repositories, opened };
};

// Radix radio values must be non-empty to round-trip reliably, and "" is the
// default directory path form, so every root value carries a prefix.
const rootValuePrefix = "path:";

const rowClassName =
	"flex items-center gap-2 rounded-sm py-1.5 pl-2 pr-7 text-xs";

const RootMenuItem: React.FC<{ option: RootOption }> = ({ option }) => (
	<DropdownMenuRadioItem
		value={`${rootValuePrefix}${option.root}`}
		title={option.title}
		className={rowClassName}
	>
		<span className="inline-flex size-3.5 shrink-0 items-center justify-center">
			{option.icon}
		</span>
		<span className="whitespace-nowrap font-medium">{option.label}</span>
		<span className="min-w-0 flex-1 truncate font-normal text-content-secondary">
			{option.description}
		</span>
	</DropdownMenuRadioItem>
);

type FilesRootPickerProps = {
	/** Undefined while the panel waits for the chat's repositories. */
	root: string | undefined;
	/** The root's resolved absolute path, once its listing loaded. */
	rootPath: string | undefined;
	homePath: string | undefined;
	/** Undefined when the default directory is home. */
	defaultDirectory: string | undefined;
	repoRoots: readonly string[];
	triggerRef: React.Ref<HTMLButtonElement>;
	onSelectRoot: (root: string) => void;
	onOpenPath: () => void;
	onCloseAutoFocus: (event: Event) => void;
};

/** The Files panel root menu, styled like the Git panel's view switcher. */
export const FilesRootPicker: React.FC<FilesRootPickerProps> = ({
	root,
	rootPath,
	homePath,
	defaultDirectory,
	repoRoots,
	triggerRef,
	onSelectRoot,
	onOpenPath,
	onCloseAutoFocus,
}) => {
	const options = getRootOptions({
		root,
		rootPath,
		homePath,
		defaultDirectory,
		repoRoots,
	});
	const current: Pick<RootOption, "label" | "title" | "icon"> =
		root === undefined
			? {
					label: "Loading",
					title: "Waiting for the chat's repositories",
					icon: <FolderIcon className={iconClassName} />,
				}
			: ([
					...options.filesystem,
					...options.repositories,
					...(options.opened ? [options.opened] : []),
				].find((option) => option.root === root) ?? options.filesystem[0]);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					ref={triggerRef}
					type="button"
					aria-label={`Root directory: ${current.label}`}
					title={current.title}
					className={cn(
						"inline-flex h-6 min-w-0 max-w-full cursor-pointer items-center gap-1.5 rounded-md border border-solid border-border",
						"bg-surface-primary px-2 text-xs text-content-primary transition-colors hover:bg-surface-secondary",
					)}
				>
					<span className="inline-flex size-3.5 shrink-0 items-center justify-center text-content-secondary">
						{current.icon}
					</span>
					<span className="truncate font-medium">{current.label}</span>
					<ChevronDownIcon className="size-3 shrink-0 opacity-70" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				className="w-(--radix-dropdown-menu-trigger-width) min-w-[240px] p-1"
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<DropdownMenuRadioGroup
					value={root === undefined ? "" : `${rootValuePrefix}${root}`}
					onValueChange={(value) =>
						onSelectRoot(value.slice(rootValuePrefix.length))
					}
				>
					<DropdownMenuLabel className="py-1">Filesystem</DropdownMenuLabel>
					{options.filesystem.map((option) => (
						<RootMenuItem key={option.root} option={option} />
					))}
					{options.repositories.length > 0 && (
						<DropdownMenuLabel className="py-1">Repositories</DropdownMenuLabel>
					)}
					{options.repositories.map((option) => (
						<RootMenuItem key={option.root} option={option} />
					))}
					{options.opened && (
						<>
							<DropdownMenuSeparator />
							<RootMenuItem option={options.opened} />
						</>
					)}
				</DropdownMenuRadioGroup>
				<DropdownMenuSeparator />
				<DropdownMenuItem className={rowClassName} onSelect={onOpenPath}>
					<span className="inline-flex size-3.5 shrink-0 items-center justify-center">
						<FolderInputIcon className={iconClassName} />
					</span>
					<span className="font-medium">Open path...</span>
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
