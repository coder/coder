import Link from "@mui/material/Link";
import { FileUpload } from "components/FileUpload/FileUpload";
import { FC } from "react";
import { Link as RouterLink } from "react-router-dom";

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
        component={RouterLink}
        to="/starter-templates"
        // Prevent trigger the upload
        onClick={(e) => {
          e.stopPropagation();
        }}
      >
        starter templates
      </Link>{" "}
      to getting started with Coder.
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
