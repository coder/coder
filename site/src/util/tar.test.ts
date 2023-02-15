import { TarReader, TarWriter, ITarFileInfo } from "./tar"

const mtime = 1666666666666

test("tar", async () => {
  // Write
  const writer = new TarWriter()
  writer.addFile("a.txt", "hello", { mtime })
  writer.addFile("b.txt", new Blob(["world"]), { mtime })
  writer.addFile("c.txt", "", { mtime })
  const blob = await writer.write()

  // Read
  const reader = new TarReader()
  const fileInfos = await reader.readFile(blob)
  verifyFile(fileInfos[0], reader.getTextFile(fileInfos[0].name) as string, {
    name: "a.txt",
    content: "hello",
  })
  verifyFile(fileInfos[1], reader.getTextFile(fileInfos[1].name) as string, {
    name: "b.txt",
    content: "world",
  })
  verifyFile(fileInfos[2], reader.getTextFile(fileInfos[2].name) as string, {
    name: "c.txt",
    content: "",
  })
})

function verifyFile(
  info: ITarFileInfo,
  content: string,
  expected: { name: string; content: string },
) {
  expect(info.name).toEqual(expected.name)
  expect(info.size).toEqual(expected.content.length)
  expect(content).toEqual(expected.content)
}
