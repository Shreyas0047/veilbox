#!/bin/bash
# Auto-start niri session if on tty1
if [ "$(tty)" = "/dev/tty1" ]; then
    exec niri-session
fi
# Source bashrc for prompt and aliases on non-tty1 sessions (SSH, other ttys)
[ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"
