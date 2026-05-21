import CircularProgress from "@mui/material/CircularProgress";
import { CloudUploadIcon, FolderIcon, TrashIcon } from "lucide-react";
import { type DragEvent, type FC, type ReactNode, useRef } from "react";
import { Button } from "#/components/Button/Button";
import { useClickable } from "#/hooks/useClickable";
import { cn } from "#/utils/cn";

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
			<div className="flex flex-row items-center justify-between gap-4 rounded-lg border border-border bg-surface-primary p-4">
				<div className="flex flex-row items-center gap-4">
					<FolderIcon className="size-icon-sm" />
					<span>{file.name}</span>
				</div>

				<Button
					variant="subtle"
					size="icon-lg"
					onClick={onRemove}
					title={removeLabel}
				>
					<TrashIcon className="size-icon-sm" />
				</Button>
			</div>
		);
	}

	return (
		<>
			<div
				data-testid="drop-zone"
				className={cn(
					"flex cursor-pointer items-center justify-center rounded-lg border-2",
					"border-dashed border-border p-12 hover:bg-surface-primary",
					isUploading && "pointer-events-none opacity-75",
				)}
				{...clickable}
				{...fileDrop}
			>
				<div className="flex flex-col items-center gap-2">
					<div className="flex size-16 items-center justify-center">
						{isUploading ? (
							<CircularProgress size={32} />
						) : (
							<CloudUploadIcon className="size-16" />
						)}
					</div>

					<div className="flex flex-col items-center gap-1">
						<span className="text-base leading-none">{title}</span>
						<span className="mt-1 max-w-[400px] text-center text-sm leading-6 text-content-secondary">
							{description}
						</span>
					</div>
				</div>
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
