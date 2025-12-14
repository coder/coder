import CircularProgress from "@mui/material/CircularProgress";
import IconButton from "@mui/material/IconButton";
import { Stack } from "components/Stack/Stack";
import { useClickable } from "hooks/useClickable";
import { CloudUploadIcon, FolderIcon, TrashIcon } from "lucide-react";
import { type DragEvent, type FC, type ReactNode, useRef } from "react";
import { cn } from "utils/cn";

interface FileUploadProps {
	isUploading: boolean;
	onUpload: (file: File) => void;
	onRemove?: () => void;
	file?: File;
	removeLabel: string;
	title: string;
	description?: ReactNode;
	extensions?: string[];
}

export const FileUpload: FC<FileUploadProps> = ({
	isUploading,
	onUpload,
	onRemove,
	file,
	removeLabel,
	title,
	description,
	extensions,
}) => {
	const fileDrop = useFileDrop(onUpload, extensions);
	const inputRef = useRef<HTMLInputElement>(null);
	const clickable = useClickable<HTMLDivElement>(() =>
		inputRef.current?.click(),
	);

	if (!isUploading && file) {
		return (
			<Stack
				className="rounded-lg p-4 border border-solid bg-surface-secondary"
				direction="row"
				justifyContent="space-between"
				alignItems="center"
			>
				<Stack direction="row" alignItems="center">
					<FolderIcon className="size-icon-sm" />
					<span>{file.name}</span>
				</Stack>

				<IconButton title={removeLabel} size="small" onClick={onRemove}>
					<TrashIcon className="size-icon-sm" />
				</IconButton>
			</Stack>
		);
	}

	return (
		<>
			<div
				data-testid="drop-zone"
				className={cn(
					"flex items-center justify-center rounded-lg p-12 cursor-pointer",
					"border-2 border-dashed hover:bg-surface-secondary",
					isUploading && "pointer-events-none opacity-75",
				)}
				{...clickable}
				{...fileDrop}
			>
				<Stack alignItems="center" spacing={1}>
					<div
						// Used to maintain the size of icon and spinner
						className="size-16 flex items-center justify-center"
					>
						{isUploading ? (
							<CircularProgress size={32} />
						) : (
							<CloudUploadIcon className="size-16" />
						)}
					</div>

					<Stack alignItems="center" spacing={0.5}>
						<span className="text-base leading-none">{title}</span>
						<span className="text-center text-content-secondary max-w-[400px] text-sm leading-normal mt-1">
							{description}
						</span>
					</Stack>
				</Stack>
			</div>

			<input
				type="file"
				data-testid="file-upload"
				ref={inputRef}
				className="hidden"
				accept={extensions?.map((ext) => `.${ext}`).join(",")}
				onChange={(event) => {
					const file = event.currentTarget.files?.[0];
					if (file) {
						onUpload(file);
					}
				}}
			/>
		</>
	);
};

const useFileDrop = (
	callback: (file: File) => void,
	extensions?: string[],
): {
	onDragOver: (e: DragEvent<HTMLDivElement>) => void;
	onDrop: (e: DragEvent<HTMLDivElement>) => void;
} => {
	const onDragOver = (e: DragEvent<HTMLDivElement>) => {
		e.preventDefault();
	};

	const onDrop = (e: DragEvent<HTMLDivElement>) => {
		e.preventDefault();
		const file = e.dataTransfer.files[0] as File | undefined;

		if (!file) {
			return;
		}

		if (!extensions) {
			callback(file);
			return;
		}

		const extension = file.name.split(".").pop();

		if (!extension) {
			throw new Error(`File has no extension to compare with ${extensions}`);
		}

		if (extensions.includes(extension)) {
			callback(file);
		}
	};

	return {
		onDragOver,
		onDrop,
	};
};
