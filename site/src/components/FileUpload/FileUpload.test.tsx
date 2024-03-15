import { fireEvent, render, screen } from "@testing-library/react";
import { ThemeProvider } from "contexts/ThemeProvider";
import { FileUpload } from "./FileUpload";

test("accepts files with the correct extension", async () => {
  const onUpload = jest.fn();

  render(
    <ThemeProvider>
      <FileUpload
        isUploading={false}
        onUpload={onUpload}
        removeLabel="Remove file"
        title="Upload file"
        extensions={["tar", "zip"]}
      />
    </ThemeProvider>,
  );

  const dropZone = screen.getByTestId("drop-zone");

  const tarFile = new File([""], "file.tar");
  fireEvent.drop(dropZone, {
    dataTransfer: { files: [tarFile] },
  });
  expect(onUpload).toBeCalledWith(tarFile);
  onUpload.mockClear();

  const zipFile = new File([""], "file.zip");
  fireEvent.drop(dropZone, {
    dataTransfer: { files: [zipFile] },
  });
  expect(onUpload).toBeCalledWith(zipFile);
  onUpload.mockClear();

  const unsupportedFile = new File([""], "file.mp4");
  fireEvent.drop(dropZone, {
    dataTransfer: { files: [unsupportedFile] },
  });
  expect(onUpload).not.toHaveBeenCalled();
});
