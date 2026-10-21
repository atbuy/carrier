"""Types lines into an interactive bash in a pseudo-terminal, for recording
shell sessions with an old carrier release.

Usage: shell.py <home> <carrier-binary> <line>...

The shell starts with a fixed, minimal environment and no rc files.
"""
import os
import pty
import select
import shutil
import sys
import time

home, binary, lines = sys.argv[1], sys.argv[2], sys.argv[3:]
bindir = os.path.join(home, ".bin")
os.makedirs(bindir, exist_ok=True)
target = os.path.join(bindir, "carrier")
if os.path.lexists(target):
    os.remove(target)
shutil.copy(binary, target)

env = {
    "HOME": home,
    "SHELL": "/usr/bin/bash",
    "PATH": bindir + ":/usr/bin:/bin",
    "LANG": "C.UTF-8",
    "TERM": "xterm",
    "PS1": "$ ",
    "CARRIER_COLOR": "never",
}
os.chdir(os.path.join(home, "api"))
pid, fd = pty.fork()
if pid == 0:
    os.execve("/usr/bin/bash", ["bash", "--norc", "-i"], env)


def drain(seconds=1.5):
    end = time.time() + seconds
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], 0.1)
        if not ready:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError:
            return
        # Answer the terminal queries carrier shell sends at startup.
        if b"]11;?" in data:
            os.write(fd, b"\x1b]11;rgb:0000/0000/0000\x1b\\")
        if b"[6n" in data:
            os.write(fd, b"\x1b[1;1R")


for line in lines:
    drain()
    os.write(fd, (line + "\n").encode())
drain(3)
try:
    os.kill(pid, 9)
    os.waitpid(pid, 0)
except OSError:
    pass
