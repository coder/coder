"""Test-only PTY driver. Prompt glyph detection synchronizes keyboard input.

All behavior assertions use structured bridge events, not terminal snapshots.
"""
import json
import os
import pty
import select
import signal
import struct
import sys
import termios
import fcntl

pid, master = pty.fork()
if pid == 0:
    os.environ['TERM'] = 'xterm-256color'
    os.execvp(sys.argv[1], sys.argv[1:])

fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 120, 0, 0))
print(json.dumps({'pid': pid}), flush=True)
buffer = b''
screen = b''
ready_sent = False


def stop(signum, frame):
    raise SystemExit(0)


signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)
try:
    while True:
        ready, _, _ = select.select([master, sys.stdin.buffer], [], [])
        if master in ready:
            try:
                data = os.read(master, 65536)
                if not data:
                    break
                screen = (screen + data)[-65536:]
                if not ready_sent and '❯'.encode() in screen:
                    ready_sent = True
                    print(json.dumps({'ready': True}), flush=True)
            except OSError:
                break
        if sys.stdin.buffer in ready:
            chunk = os.read(sys.stdin.fileno(), 65536)
            if not chunk:
                break
            buffer += chunk
            while b'\n' in buffer:
                line, buffer = buffer.split(b'\n', 1)
                message = json.loads(line)
                os.write(master, message['input'].encode())
finally:
    os.close(master)
    try:
        os.kill(pid, signal.SIGHUP)
    except ProcessLookupError:
        pass
    # Bound cleanup even if a native terminal stops responding to hangup.
    signal.signal(signal.SIGALRM, lambda *_: os.kill(pid, signal.SIGKILL))
    signal.alarm(5)
    os.waitpid(pid, 0)
    signal.alarm(0)
