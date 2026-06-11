import { cva, type VariantProps } from "class-variance-authority";
import { XIcon } from "lucide-react";
import type { CSSProperties, FC } from "react";
import { FileIcon } from "#/components/FileIcon/FileIcon";
import { cn } from "#/utils/cn";
import { getFileReferenceDisplay } from "./fileReferenceDisplay";

const fileReferenceChipVariants = cva(
	"inline-flex min-h-5 max-w-[300px] select-none items-center gap-1 rounded-md border border-border-default bg-surface-primary py-0 pl-0.5 pr-1.5 align-middle font-sans text-[13px] font-normal leading-none text-inherit shadow-sm transition-colors",
	{
		variants: {
			interactive: {
				true: "cursor-pointer hover:border-border-secondary hover:bg-surface-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
				false: "cursor-default",
			},
			selected: {
				true: "border-content-link bg-content-link/10 text-content-primary ring-1 ring-content-link/40",
				false: "",
			},
		},
		defaultVariants: {
			interactive: false,
			selected: false,
		},
	},
);

const fileReferenceTriggerVariants = cva(
	"inline-flex min-w-0 items-center gap-1 border-0 bg-transparent p-0 font-sans text-[13px] font-normal leading-none text-inherit",
	{
		variants: {
			interactive: {
				true: "cursor-pointer focus-visible:outline-none",
				false: "cursor-default",
			},
		},
		defaultVariants: {
			interactive: false,
		},
	},
);

const fileReferenceIconStyle: CSSProperties = {
	fontSize: 16,
	height: "1rem",
	minWidth: "1rem",
};

type FileReferenceChipContentProps = {
	fileName: string;
	lineRange: string;
};

const FileReferenceChipContent: FC<FileReferenceChipContentProps> = ({
	fileName,
	lineRange,
}) => {
	return (
		<>
			<FileIcon
				fileName={fileName}
				className="shrink-0"
				style={fileReferenceIconStyle}
			/>
			<span
				data-slot="file-reference-chip-label"
				className="inline-flex min-w-0 items-center gap-0.5"
			>
				<span dir="rtl" className="min-w-0 truncate">
					{fileName}
				</span>
				<span className="shrink-0">·</span>
				<span className="shrink-0 tabular-nums">{lineRange}</span>
			</span>
		</>
	);
};

type FileReferenceChipBaseProps = {
	fileName: string;
	startLine: number;
	endLine: number;
	className?: string;
};

type FileReferenceChipSelectedProps = Pick<
	VariantProps<typeof fileReferenceChipVariants>,
	"selected"
>;

type FileReferenceChipProps = FileReferenceChipBaseProps &
	FileReferenceChipSelectedProps;

export function FileReferenceChip({
	fileName,
	startLine,
	endLine,
	selected,
	className,
}: FileReferenceChipProps) {
	const { shortFile, lineRange, title } = getFileReferenceDisplay({
		fileName,
		startLine,
		endLine,
	});

	return (
		<span
			data-slot="file-reference-chip"
			className={cn(fileReferenceChipVariants({ selected }), className)}
			title={title}
		>
			<span
				data-slot="file-reference-chip-trigger"
				className={fileReferenceTriggerVariants()}
			>
				<FileReferenceChipContent fileName={shortFile} lineRange={lineRange} />
			</span>
		</span>
	);
}

export function EditableFileReferenceChip({
	fileName,
	startLine,
	endLine,
	selected,
	onRemove,
	onOpen,
	className,
}: FileReferenceChipBaseProps &
	FileReferenceChipSelectedProps & {
		onRemove: () => void;
		onOpen: () => void;
	}) {
	const { shortFile, lineRange, title } = getFileReferenceDisplay({
		fileName,
		startLine,
		endLine,
	});

	return (
		<span
			data-slot="file-reference-chip"
			className={cn(
				fileReferenceChipVariants({ interactive: true, selected }),
				"border-border-secondary bg-surface-tertiary text-content-primary hover:bg-surface-quaternary",
				className,
			)}
			contentEditable={false}
			title={title}
		>
			<button
				data-slot="file-reference-chip-trigger"
				type="button"
				className={fileReferenceTriggerVariants({ interactive: true })}
				onClick={onOpen}
				aria-label={`Open ${title}`}
			>
				<FileReferenceChipContent fileName={shortFile} lineRange={lineRange} />
			</button>
			<button
				data-slot="file-reference-chip-remove"
				type="button"
				className="ml-0.5 inline-flex size-3.5 shrink-0 cursor-pointer items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary transition-colors hover:bg-surface-quaternary hover:text-content-primary"
				onClick={(e) => {
					e.preventDefault();
					e.stopPropagation();
					onRemove();
				}}
				aria-label="Remove reference"
				tabIndex={-1}
			>
				<XIcon className="size-2.5" />
			</button>
		</span>
	);
}
