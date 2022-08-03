declare module "can-ndjson-stream" {
  function ndjsonStream<TValueType>(
    body: ReadableStream<Uint8Array> | null,
  ): Promise<ReadableStream<TValueType>>
  export default ndjsonStream
}
