import { fireEvent, screen } from "@testing-library/react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { FileUpload } from "./FileUpload";

test("accepts files with the correct extension", () => {
	const onUpload = vi.fn();

	renderComponent(
		<FileUpload
			isUploading={false}
			onUpload={onUpload}
			removeLabel="Remove file"
			title="Upload file"
			extensions={["tar", "zip"]}
		/>,
	);

	const dropZone = screen.getByTestId("drop-zone");

	const tarFile = new File([""], "file.tar");
	fireEvent.drop(dropZone, {
		dataTransfer: { files: [tarFile] },
	});
	expect(onUpload).toHaveBeenCalledWith(tarFile);
	onUpload.mockClear();

	const zipFile = new File([""], "file.zip");
	fireEvent.drop(dropZone, {
		dataTransfer: { files: [zipFile] },
	});
	expect(onUpload).toHaveBeenCalledWith(zipFile);
	onUpload.mockClear();

	const uppercaseTarFile = new File([""], "file.TAR");
	fireEvent.drop(dropZone, {
		dataTransfer: { files: [uppercaseTarFile] },
	});
	expect(onUpload).toHaveBeenCalledWith(uppercaseTarFile);
	onUpload.mockClear();

	const unsupportedFile = new File([""], "file.mp4");
	fireEvent.drop(dropZone, {
		dataTransfer: { files: [unsupportedFile] },
	});
	expect(onUpload).not.toHaveBeenCalled();
});

test("reports files with an unsupported extension", () => {
	const onUpload = vi.fn();
	const onUnsupportedFile = vi.fn();

	renderComponent(
		<FileUpload
			isUploading={false}
			onUpload={onUpload}
			onUnsupportedFile={onUnsupportedFile}
			removeLabel="Remove file"
			title="Upload file"
			extensions={["env", "json"]}
		/>,
	);

	const unsupportedFile = new File([""], "bad.txt");
	fireEvent.drop(screen.getByTestId("drop-zone"), {
		dataTransfer: { files: [unsupportedFile] },
	});

	expect(onUnsupportedFile).toHaveBeenCalledWith(unsupportedFile);
	expect(onUpload).not.toHaveBeenCalled();
});
