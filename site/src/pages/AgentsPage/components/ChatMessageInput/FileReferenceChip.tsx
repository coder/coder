import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { XIcon } from "lucide-react";
import { FileIcon } from "#/components/FileIcon/FileIcon";
import { getFileReferenceDisplay } from "./fileReferenceDisplay";

export const fileReferenceChipVariants = cva(
	"inline-flex min-h-5 max-w-[300px] select-none items-center gap-1 rounded-md border border-border bg-surface-primary py-0 pl-0.5 pr-1.5 align-middle font-sans text-[13px] font-normal leading-none text-inherit shadow-xs transition-colors",
	{
		variants: {
			interactive: {
				true: "cursor-pointer hover:border-border-secondary hover:bg-surface-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				false: "cursor-default",
			},
			selected: {
				true: "border-content-link bg-content-link/10 text-content-primary ring-1 ring-content-link/40",
				false: "",
			},
			editable: {
				true: "text-content-primary hover:bg-surface-quaternary",
				false: "",
			},
		},
		compoundVariants: [
			{
				editable: true,
				selected: false,
				className: "border-border-secondary bg-surface-tertiary",
			},
		],
		defaultVariants: {
			interactive: false,
			selected: false,
			editable: false,
		},
	},
);

const fileReferenceTriggerVariants = cva(
	"inline-flex min-w-0 items-center gap-1 border-0 bg-transparent p-0 font-sans text-[13px] font-normal leading-none text-inherit",
	{
		variants: {
			interactive: {
				true: "cursor-pointer focus-visible:outline-hidden",
				false: "cursor-default",
			},
		},
		defaultVariants: {
			interactive: false,
		},
	},
);

const fileReferenceIconStyle: React.CSSProperties = {
	fontSize: 16,
	height: "1rem",
	minWidth: "1rem",
};

type FileReferenceChipContentProps = {
	fileName: string;
	lineRange: string;
};

const FileReferenceChipContent: React.FC<FileReferenceChipContentProps> = ({
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

/**
 * Removes an editable chip by click or by Tab then Enter or Space.
 * The pseudo-element widens the tap target to 24px without moving the icon.
 */
export const ChipRemoveButton: React.FC<{
	label: string;
	onRemove: () => void;
}> = ({ label, onRemove }) => (
	<button
		type="button"
		className="relative ml-0.5 inline-flex size-3.5 shrink-0 cursor-pointer items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary transition-colors after:absolute after:-inset-1.25 after:content-[''] hover:bg-surface-quaternary hover:text-content-primary"
		onClick={(e) => {
			e.preventDefault();
			e.stopPropagation();
			onRemove();
		}}
		// Enter would send the message from the editor instead of pressing this.
		onKeyDownCapture={(e) => {
			if (e.key === "Enter" || e.key === " ") {
				e.stopPropagation();
			}
		}}
		aria-label={label}
	>
		<XIcon className="size-2.5" />
	</button>
);

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
				fileReferenceChipVariants({
					interactive: true,
					selected,
					editable: true,
				}),
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
			<ChipRemoveButton label={`Remove ${title}`} onRemove={onRemove} />
		</span>
	);
}
