#!/usr/bin/env bash
set -eu
export XDG_RUNTIME_DIR="/tmp/orca-qa-runtime"
mkdir -p -m 700 "$XDG_RUNTIME_DIR"
Xvfb :99 -screen 0 1280x900x24 -nolisten tcp >/tmp/orca-x.log 2>&1 &
for i in {1..40}; do [ -e /tmp/.X11-unix/X99 ] && break; sleep .1; done
openbox >/tmp/orca-window-manager.log 2>&1 &
pulseaudio --start --exit-idle-time=-1 --log-target=file:/tmp/orca-audio.log
pactl load-module module-null-sink sink_name=qa_output >/dev/null
pactl set-default-sink qa_output
gsettings set org.gnome.desktop.interface toolkit-accessibility true
orca --replace --enable=speech --disable=braille --debug-file=/tmp/orca-events.log >/tmp/orca-start.log 2>&1 &
node /ui/orca-smoke.mjs
