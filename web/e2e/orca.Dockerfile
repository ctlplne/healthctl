# scripts/verify.py browser builds this local QA image from the locked frontend.
FROM clinical-history-qa-browser-tests
USER root
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends orca speech-dispatcher-espeak-ng pulseaudio pulseaudio-utils openbox xauth dbus-x11 libglib2.0-bin && rm -rf /var/lib/apt/lists/*
ENV DISPLAY=:99 GTK_MODULES=gail:atk-bridge NO_AT_BRIDGE=0
WORKDIR /ui
COPY orca-session.sh orca-smoke.mjs ./
USER pwuser
CMD ["dbus-run-session","--","bash","/ui/orca-session.sh"]
