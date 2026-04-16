import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { FileUpload } from "#/components/FileUpload/FileUpload";
import { Link } from "#/components/Link/Link";

export interface TemplateUploadProps {
	isUploading: boolean;
	onUpload: (file: File) => void;
	onRemove: () => void;
	file?: File;
}

export const TemplateUpload: FC<TemplateUploadProps> = ({
	isUploading,
	onUpload,
	onRemove,
	file,
}) => {
	const description = (
		<>
			The template has to be a .tar or .zip file. You can also use our{" "}
			<Link
				asChild
				showExternalIcon={false}
				// Prevent trigger the upload
				onClick={(e) => {
					e.stopPropagation();
				}}
			>
				<RouterLink to="/starter-templates">starter templates</RouterLink>
			</Link>{" "}
			to get started with Coder.
		</>
	);

	return (
		<FileUpload
			isUploading={isUploading}
			onUpload={onUpload}
			onRemove={onRemove}
			file={file}
			removeLabel="Remove file"
			title="Upload template"
			description={description}
			extensions={["tar", "zip"]}
		/>
	);
};
