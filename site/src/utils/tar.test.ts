import { TarReader, TarWriter, ITarFileInfo, TarFileTypeCodes } from "./tar";

const mtime = 1666666666666;

test("tar", async () => {
  // Write
  const writer = new TarWriter();
  writer.addFile("a.txt", "hello", { mtime });
  writer.addFile("b.txt", new Blob(["world"]), { mtime });
  writer.addFile("c.txt", "", { mtime });
  writer.addFolder("etc", { mtime });
  writer.addFile("etc/d.txt", "Some text content", {
    mtime,
    user: "coder",
    group: "codergroup",
    mode: parseInt("777", 8),
  });
  const blob = (await writer.write()) as Blob;

  // Read
  const reader = new TarReader();
  const fileInfos = await reader.readFile(blob);
  verifyFile(fileInfos[0], reader.getTextFile(fileInfos[0].name) as string, {
    name: "a.txt",
    content: "hello",
  });
  verifyFile(fileInfos[1], reader.getTextFile(fileInfos[1].name) as string, {
    name: "b.txt",
    content: "world",
  });
  verifyFile(fileInfos[2], reader.getTextFile(fileInfos[2].name) as string, {
    name: "c.txt",
    content: "",
  });
  verifyFolder(fileInfos[3], {
    name: "etc",
  });
  verifyFile(fileInfos[4], reader.getTextFile(fileInfos[4].name) as string, {
    name: "etc/d.txt",
    content: "Some text content",
  });
  expect(fileInfos[4].group).toEqual("codergroup");
  expect(fileInfos[4].user).toEqual("coder");
  expect(fileInfos[4].mode).toEqual(parseInt("777", 8));
});

function verifyFile(
  info: ITarFileInfo,
  content: string,
  expected: { name: string; content: string },
) {
  expect(info.name).toEqual(expected.name);
  expect(info.size).toEqual(expected.content.length);
  expect(content).toEqual(expected.content);
}

function verifyFolder(info: ITarFileInfo, expected: { name: string }) {
  expect(info.name).toEqual(expected.name);
  expect(info.type).toEqual(TarFileTypeCodes.Dir);
}
