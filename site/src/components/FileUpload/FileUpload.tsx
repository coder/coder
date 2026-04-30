import { css, type Interpolation, type Theme } from "@emotion/react";
import CircularProgress from "@mui/material/CircularProgress";
import { CloudUploadIcon, FolderIcon, TrashIcon } from "lucide-react";
import { type DragEvent, type FC, type ReactNode, useRef } from "react";
import { Button } from "#/components/Button/Button";
import { useClickable } from "#/hooks/useClickable";

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
			<div
				css={styles.file}
				className="flex flex-row justify-between items-center gap-4"
			>
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
				css={[styles.root, isUploading && styles.disabled]}
				{...clickable}
				{...fileDrop}
			>
				<div className="flex flex-col items-center gap-2">
					<div css={styles.iconWrapper}>
						{isUploading ? (
							<CircularProgress size={32} />
						) : (
							<CloudUploadIcon className="size-16" />
						)}
					</div>

					<div className="flex flex-col items-center gap-1">
						<span css={styles.title}>{title}</span>
						<span css={styles.description}>{description}</span>
					</div>
				</div>
			</div>

			<input
				type="file"
				data-testid="file-upload"
				ref={inputRef}
				css={styles.input}
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

const styles = {
	root: (theme) => css`
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 8px;
		border: 2px dashed ${theme.palette.divider};
		padding: 48px;
		cursor: pointer;

		&:hover {
			background-color: ${theme.palette.background.paper};
		}
	`,

	disabled: {
		pointerEvents: "none",
		opacity: 0.75,
	},

	// Used to maintain the size of icon and spinner
	iconWrapper: {
		width: 64,
		height: 64,
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
	},

	title: {
		fontSize: 16,
		lineHeight: "1",
	},

	description: (theme) => ({
		color: theme.palette.text.secondary,
		textAlign: "center",
		maxWidth: 400,
		fontSize: 14,
		lineHeight: "1.5",
		marginTop: 4,
	}),

	input: {
		display: "none",
	},

	file: (theme) => ({
		borderRadius: 8,
		border: `1px solid ${theme.palette.divider}`,
		padding: 16,
		background: theme.palette.background.paper,
	}),
} satisfies Record<string, Interpolation<Theme>>;
