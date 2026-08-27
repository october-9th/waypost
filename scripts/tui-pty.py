#!/usr/bin/env python3
"""Chạy TUI trong pty giả, gõ phím thật, rồi in lại màn hình.

Cách duy nhất thử được TUI khi không ngồi trước terminal. Dùng:

    go build -o bin/albert .
    SCENARIO=scripts/scenarios/basic.py python3 scripts/tui-pty.py bin/albert 30 100

File scenario là Python chạy thẳng, dùng được typ(), pump(), snapshot() và fd.

Lưu ý: pump() phải trả lời OSC 10/11 và CSI 6n — termenv hỏi màu nền rồi ĐỌC
stdin chờ trả lời (timeout 5 giây mỗi query) và nuốt luôn phím gõ trong lúc
chờ. Bỏ phần đó thì triệu chứng là "app chạy nhưng không nhận phím".
"""
import os, pty, sys, time, fcntl, termios, struct, select, re

BIN = sys.argv[1]
ROWS, COLS = int(sys.argv[2]), int(sys.argv[3])
args = sys.argv[4:]

pid, fd = pty.fork()
if pid == 0:
    os.environ["TERM"] = "xterm-256color"
    os.execv(BIN, [BIN] + args)

fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
buf = b""

def respond(chunk):
    """termenv hỏi màu nền (OSC 10/11) và vị trí con trỏ (CSI 6n) rồi ĐỌC
    stdin chờ trả lời, nuốt luôn phím gõ trong lúc chờ (timeout 5s mỗi query).
    Terminal thật trả lời tức thì; harness này phải đóng vai đó."""
    for seq in (b"10", b"11"):
        if b"\x1b]" + seq + b";?" in chunk:
            os.write(fd, b"\x1b]" + seq + b";rgb:1c1c/1c1c/1c1c\x1b\\")
    if b"\x1b[6n" in chunk:
        os.write(fd, b"\x1b[1;1R")

def pump(seconds):
    global buf
    end = time.time() + seconds
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.05)
        if r:
            try:
                d = os.read(fd, 65536)
            except OSError:
                return False
            if not d:
                return False
            buf += d
            respond(d)
    return True

def typ(s, per=0.05):
    for ch in s:
        os.write(fd, ch.encode())
        pump(per)

def snapshot(tag):
    """Dựng lại màn hình từ chuỗi escape: chỉ cần CUP, EL, ED và text."""
    text = buf.decode("utf-8", "replace")
    rows, cols = ROWS, COLS
    screen = [[" "] * cols for _ in range(rows)]
    cy = cx = 0
    i = 0
    while i < len(text):
        c = text[i]
        if c == "\x1b":
            mm = re.match(r"\x1b\[([0-9;?]*)([a-zA-Z])", text[i:])
            if mm:
                params, cmd = mm.group(1), mm.group(2)
                nums = [int(x) for x in params.split(";") if x.isdigit()]
                if cmd == "H":
                    cy = (nums[0] - 1) if len(nums) > 0 else 0
                    cx = (nums[1] - 1) if len(nums) > 1 else 0
                elif cmd == "K":
                    for x in range(cx, cols):
                        screen[cy][x] = " "
                elif cmd == "J":
                    for y in range(cy, rows):
                        for x in range(cols):
                            screen[y][x] = " "
                elif cmd == "A":
                    cy = max(0, cy - (nums[0] if nums else 1))
                elif cmd == "B":
                    cy = min(rows - 1, cy + (nums[0] if nums else 1))
                i += mm.end()
                continue
            mm = re.match(r"\x1b[()][B0]|\x1b\]\d+;[^\x07\x1b]*(\x07|\x1b\\)", text[i:])
            if mm:
                i += mm.end()
                continue
            i += 1
            continue
        if c == "\r":
            cx = 0
        elif c == "\n":
            cy = min(rows - 1, cy + 1)
            cx = 0
        else:
            if 0 <= cy < rows and 0 <= cx < cols:
                screen[cy][cx] = c
            cx += 1
        i += 1
    print("===== %s =====" % tag)
    for row in screen:
        line = "".join(row).rstrip()
        print("|" + line)

pump(1.0)
exec(open(os.environ["SCENARIO"]).read())
os.close(fd)
_, status = os.waitpid(pid, 0)
print("wait status:", status)
