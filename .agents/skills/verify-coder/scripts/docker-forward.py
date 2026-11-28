#!/usr/bin/env python3
"""Forwards TCP connections on <listen host>:<port> to 127.0.0.1:<port>.

launch.sh runs it on the Docker bridge gateway for the API port only, so
workspace containers reach the API through host.docker.internal while
develop, Vite included, listens on 127.0.0.1. Usage:
docker-forward.py <listen host> <port>
"""

import asyncio
import sys


async def pipe(reader, writer):
    try:
        while data := await reader.read(65536):
            writer.write(data)
            await writer.drain()
    finally:
        writer.close()


async def main(host, port):
    async def handle(client_reader, client_writer):
        try:
            reader, writer = await asyncio.open_connection("127.0.0.1", port)
        except OSError:
            client_writer.close()
            return
        await asyncio.gather(
            pipe(client_reader, writer),
            pipe(reader, client_writer),
            return_exceptions=True,
        )

    server = await asyncio.start_server(handle, host, port)
    print(f"forwarding {host}:{port} to 127.0.0.1:{port}", flush=True)
    async with server:
        await server.serve_forever()


if __name__ == "__main__":
    assert len(sys.argv) == 3, __doc__
    asyncio.run(main(sys.argv[1], int(sys.argv[2])))
