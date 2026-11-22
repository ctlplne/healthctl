ARG SYFT_IMAGE=anchore/syft:v1.57.2@sha256:e97e20b6e6d1699c60296b66ab337941724f9bd06ac279d893eda89601fc4b2a
FROM node:22-bookworm@sha256:363e1587494626837fa7f9a23bdb453d13b0ff3c67c705c2805cfc69c2d2fad7 AS ui
WORKDIR /ui
COPY web/package*.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
COPY internal/clinical/model-guidance.txt internal/clinical/preview.txt /internal/clinical/
COPY PRIVACY.md LICENSE /
COPY docs/use-notice.md /docs/use-notice.md
RUN npm run build

FROM golang:1.26.10-bookworm@sha256:23ce71337b726608afb1bc00986e630a05034a87a87ea1fc2cd1ae5cf33387eb AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY evaluation ./evaluation
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app ./cmd/healthctl

FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc AS readers
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates poppler-utils tesseract-ocr tesseract-ocr-eng && rm -rf /var/lib/apt/lists/*
FROM server AS evaluator-build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /evaluate ./cmd/evaluate

FROM readers AS evaluator
COPY --from=evaluator-build /evaluate /evaluate
USER 65532:65532
ENTRYPOINT ["/evaluate"]

FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc AS speech
RUN apt-get update && apt-get install -y --no-install-recommends cmake g++ make ca-certificates curl
RUN curl -fsSL --retry 3 https://codeload.github.com/ggml-org/whisper.cpp/tar.gz/refs/tags/v1.9.5 -o /tmp/source.tgz && echo "ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720  /tmp/source.tgz" | sha256sum -c - && mkdir /src && tar -xzf /tmp/source.tgz --strip-components=1 -C /src
RUN cmake -S /src -B /tmp/build -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF -DGGML_OPENMP=OFF -DWHISPER_BUILD_TESTS=OFF && cmake --build /tmp/build --target whisper-cli -j 3
RUN curl -fsSL --retry 3 https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en-q5_1.bin -o /model.bin && echo "c77c5766f1cef09b6b7d47f21b546cbddd4157886b3b5d6d4f709e91e66c7c2b  /model.bin" | sha256sum -c -
FROM server AS go-notices
COPY scripts/go-notices.sh /go-notices.sh
RUN sh /go-notices.sh /notices /app

FROM readers AS runtime
COPY --from=speech /tmp/build/bin/whisper-cli /usr/local/bin/whisper-cli
COPY --from=speech /src/LICENSE /usr/share/licenses/clinical-history/whisper-cpp.txt
COPY docs/licenses/whisper-model.txt /usr/share/licenses/clinical-history/whisper-model.txt
COPY --from=speech /model.bin /usr/share/healthctl/whisper.bin
COPY --from=server /app /app
COPY --from=ui /ui/dist /ui
COPY --from=go-notices /notices /usr/share/licenses/clinical-history/go/
COPY LICENSE THIRD_PARTY_NOTICES.md internal/evidence/starter.json docs/release.md /usr/share/licenses/clinical-history/
COPY --from=ui /ui/node_modules/@fontsource/geist/LICENSE /usr/share/licenses/clinical-history/geist.txt
COPY --from=ui /ui/node_modules/@fontsource/instrument-serif/LICENSE /usr/share/licenses/clinical-history/instrument-serif.txt
COPY --from=ui /ui/node_modules/lucide-react/LICENSE /usr/share/licenses/clinical-history/lucide.txt
COPY --from=ui /ui/node_modules/react/LICENSE /usr/share/licenses/clinical-history/react.txt
COPY --from=ui /ui/node_modules/react-dom/LICENSE /usr/share/licenses/clinical-history/react-dom.txt
COPY --from=ui /ui/node_modules/scheduler/LICENSE /usr/share/licenses/clinical-history/scheduler.txt
RUN dpkg-query -W -f='${Package}\t${Version}\t${source:Package}\t${source:Version}\n' > /usr/share/licenses/clinical-history/debian-sources.tsv
ENV CLINICAL_ASSETS=/ui
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app"]

FROM ${SYFT_IMAGE} AS scanner
FROM ui AS inventory
COPY --from=scanner /syft /usr/local/bin/syft
COPY scripts/sbom-assets.mjs /sbom-assets.mjs
RUN --network=none --mount=from=runtime,source=/,target=/scan,ro \
    SYFT_CHECK_FOR_APP_UPDATE=false syft scan dir:/scan --source-name healthctl -o spdx-json=/tmp/packages.json && \
    node /sbom-assets.mjs /tmp/packages.json /app.spdx.json /scan /ui/package-lock.json

FROM runtime AS application
COPY --from=inventory /app.spdx.json /usr/share/healthctl/app.spdx.json
