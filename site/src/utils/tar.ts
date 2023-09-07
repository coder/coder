// Based on https://github.com/gera2ld/tarjs
// and https://github.com/ankitrohatgi/tarballjs/blob/master/tarball.js
type TarFileType = string;
// Added the most common codes
export const TarFileTypeCodes = {
  File: "0",
  Dir: "5",
};
const encoder = new TextEncoder();
const utf8Encode = (input: string) => encoder.encode(input);
const decoder = new TextDecoder();
const utf8Decode = (input: Uint8Array) => decoder.decode(input);

export interface ITarFileInfo {
  name: string;
  type: TarFileType;
  size: number;
  mode: number;
  mtime: number;
  user: string;
  group: string;
  headerOffset: number;
}

export interface ITarWriteItem {
  name: string;
  type: TarFileType;
  data: ArrayBuffer | Promise<ArrayBuffer> | null;
  size: number;
  opts?: Partial<ITarWriteOptions>;
}

export interface ITarWriteOptions {
  uid: number;
  gid: number;
  mode: number;
  mtime: number;
  user: string;
  group: string;
}

export class TarReader {
  public fileInfo: ITarFileInfo[] = [];
  private _buffer: ArrayBuffer | null = null;

  constructor() {
    this.reset();
  }

  get buffer() {
    if (!this._buffer) {
      throw new Error("Buffer is not set");
    }

    return this._buffer;
  }

  reset() {
    this.fileInfo = [];
    this._buffer = null;
  }

  async readFile(file: ArrayBuffer | Uint8Array | Blob) {
    this.reset();
    this._buffer = await getArrayBuffer(file);
    this.readFileInfo();
    return this.fileInfo;
  }

  private readFileInfo() {
    this.fileInfo = [];
    let offset = 0;

    while (offset < this.buffer.byteLength - 512) {
      const fileName = this.readFileName(offset);
      if (!fileName) {
        break;
      }
      const fileType = this.readFileType(offset);
      const fileSize = this.readFileSize(offset);
      const fileMode = this.readFileMode(offset);
      const fileMtime = this.readFileMtime(offset);
      const fileUser = this.readFileUser(offset);
      const fileGroup = this.readFileGroup(offset);

      this.fileInfo.push({
        name: fileName,
        type: fileType,
        size: fileSize,
        headerOffset: offset,
        mode: fileMode,
        mtime: fileMtime,
        user: fileUser,
        group: fileGroup,
      });

      offset += 512 + 512 * Math.floor((fileSize + 511) / 512);
    }
  }

  private readString(offset: number, maxSize: number) {
    let size = 0;
    let view = new Uint8Array(this.buffer, offset, maxSize);
    while (size < maxSize && view[size]) {
      size += 1;
    }
    view = new Uint8Array(this.buffer, offset, size);
    return utf8Decode(view);
  }

  private readFileName(offset: number) {
    return this.readString(offset, 100);
  }

  private readFileMode(offset: number) {
    const mode = this.readString(offset + 100, 8);
    return parseInt(mode, 8);
  }

  private readFileMtime(offset: number) {
    const mtime = this.readString(offset + 136, 12);
    return parseInt(mtime, 8);
  }

  private readFileUser(offset: number) {
    return this.readString(offset + 265, 32);
  }

  private readFileGroup(offset: number) {
    return this.readString(offset + 297, 32);
  }

  private readFileType(offset: number) {
    const typeView = new Uint8Array(this.buffer, offset + 156, 1);
    const typeStr = String.fromCharCode(typeView[0]);
    return typeStr;
  }

  private readFileSize(offset: number) {
    // offset = 124, length = 12
    const view = new Uint8Array(this.buffer, offset + 124, 12);
    const sizeStr = utf8Decode(view);
    return parseInt(sizeStr, 8);
  }

  private readFileBlob(offset: number, size: number, mimetype: string) {
    const view = new Uint8Array(this.buffer, offset, size);
    return new Blob([view], { type: mimetype });
  }

  private readTextFile(offset: number, size: number) {
    const view = new Uint8Array(this.buffer, offset, size);
    return utf8Decode(view);
  }

  getTextFile(filename: string) {
    const item = this.fileInfo.find((info) => info.name === filename);
    if (item) {
      return this.readTextFile(item.headerOffset + 512, item.size);
    }
  }

  getFileBlob(filename: string, mimetype = "") {
    const item = this.fileInfo.find((info) => info.name === filename);
    if (item) {
      return this.readFileBlob(item.headerOffset + 512, item.size, mimetype);
    }
  }
}

export class TarWriter {
  private fileData: ITarWriteItem[] = [];
  private _buffer: ArrayBuffer | null = null;

  get buffer() {
    if (!this._buffer) {
      throw new Error("Buffer is not set");
    }
    return this._buffer;
  }

  addFile(
    name: string,
    file: string | ArrayBuffer | Uint8Array | Blob,
    opts?: Partial<ITarWriteOptions>,
  ) {
    const data = getArrayBuffer(file);
    const size = (data as ArrayBuffer).byteLength ?? (file as Blob).size;
    const item: ITarWriteItem = {
      name,
      type: TarFileTypeCodes.File,
      data,
      size,
      opts,
    };
    this.fileData.push(item);
  }

  addFolder(name: string, opts?: Partial<ITarWriteOptions>) {
    this.fileData.push({
      name,
      type: TarFileTypeCodes.Dir,
      data: null,
      size: 0,
      opts,
    });
  }

  private createBuffer() {
    const dataSize = this.fileData.reduce(
      (prev, item) => prev + 512 + 512 * Math.floor((item.size + 511) / 512),
      0,
    );
    const bufSize = 10240 * Math.floor((dataSize + 10240 - 1) / 10240);
    this._buffer = new ArrayBuffer(bufSize);
  }

  async write() {
    this.createBuffer();
    const view = new Uint8Array(this.buffer);
    let offset = 0;
    for (const item of this.fileData) {
      // write header
      this.writeFileName(item.name, offset);
      this.writeFileType(item.type, offset);
      this.writeFileSize(item.size, offset);
      this.fillHeader(
        offset,
        item.opts as Partial<ITarWriteOptions>,
        item.type,
      );
      this.writeChecksum(offset);

      // write data
      const data = new Uint8Array((await item.data) as ArrayBuffer);
      view.set(data, offset + 512);
      offset += 512 + 512 * Math.floor((item.size + 511) / 512);
    }
    // Required so it works in the browser and node.
    if (typeof Blob !== "undefined") {
      return new Blob([this.buffer], { type: "application/x-tar" });
    } else {
      return this.buffer;
    }
  }

  private writeString(str: string, offset: number, size: number) {
    const strView = utf8Encode(str);
    const view = new Uint8Array(this.buffer, offset, size);
    for (let i = 0; i < size; i += 1) {
      view[i] = i < strView.length ? strView[i] : 0;
    }
  }

  private writeFileName(name: string, offset: number) {
    // offset: 0
    this.writeString(name, offset, 100);
  }

  private writeFileType(type: TarFileType, offset: number) {
    // offset: 156
    const typeView = new Uint8Array(this.buffer, offset + 156, 1);
    typeView[0] = type.charCodeAt(0);
  }

  private writeFileSize(size: number, offset: number) {
    // offset: 124
    const sizeStr = size.toString(8).padStart(11, "0");
    this.writeString(sizeStr, offset + 124, 12);
  }

  private writeFileMode(mode: number, offset: number) {
    // offset: 100
    this.writeString(mode.toString(8).padStart(7, "0"), offset + 100, 8);
  }

  private writeFileUid(uid: number, offset: number) {
    // offset: 108
    this.writeString(uid.toString(8).padStart(7, "0"), offset + 108, 8);
  }

  private writeFileGid(gid: number, offset: number) {
    // offset: 116
    this.writeString(gid.toString(8).padStart(7, "0"), offset + 116, 8);
  }

  private writeFileMtime(mtime: number, offset: number) {
    // offset: 136
    this.writeString(mtime.toString(8).padStart(11, "0"), offset + 136, 12);
  }

  private writeFileUser(user: string, offset: number) {
    // offset: 265
    this.writeString(user, offset + 265, 32);
  }

  private writeFileGroup(group: string, offset: number) {
    // offset: 297
    this.writeString(group, offset + 297, 32);
  }

  private writeChecksum(offset: number) {
    // offset: 148
    this.writeString("        ", offset + 148, 8); // first fill with spaces

    // add up header bytes
    const header = new Uint8Array(this.buffer, offset, 512);
    let chksum = 0;
    for (let i = 0; i < 512; i += 1) {
      chksum += header[i];
    }
    this.writeString(chksum.toString(8), offset + 148, 8);
  }

  private fillHeader(
    offset: number,
    opts: Partial<ITarWriteOptions>,
    fileType: TarFileType,
  ) {
    const { uid, gid, mode, mtime, user, group } = {
      uid: 1000,
      gid: 1000,
      mode: fileType === TarFileTypeCodes.File ? 0o664 : 0o775,
      mtime: ~~(Date.now() / 1000),
      user: "tarballjs",
      group: "tarballjs",
      ...opts,
    };

    this.writeFileMode(mode, offset);
    this.writeFileUid(uid, offset);
    this.writeFileGid(gid, offset);
    this.writeFileMtime(mtime, offset);

    this.writeString("ustar", offset + 257, 6); // magic string
    this.writeString("00", offset + 263, 2); // magic version

    this.writeFileUser(user, offset);
    this.writeFileGroup(group, offset);
  }
}

function getArrayBuffer(file: string | ArrayBuffer | Uint8Array | Blob) {
  if (typeof file === "string") {
    return utf8Encode(file).buffer;
  }
  if (file instanceof ArrayBuffer) {
    return file;
  }
  if (ArrayBuffer.isView(file)) {
    return new Uint8Array(file).buffer;
  }
  return file.arrayBuffer();
}
