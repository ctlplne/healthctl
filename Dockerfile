FROM node:22-bookworm@sha256:363e1587494626837fa7f9a23bdb453d13b0ff3c67c705c2805cfc69c2d2fad7 AS ui
WORKDIR /ui
COPY web/package*.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
COPY internal/clinical/model-guidance.txt /internal/clinical/model-guidance.txt
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

FROM readers AS application
COPY --from=server /app /app
COPY --from=ui /ui/dist /ui
COPY LICENSE docs/release.md /usr/share/licenses/clinical-history/
COPY --from=ui /ui/node_modules/@fontsource/geist/LICENSE /usr/share/licenses/clinical-history/geist.txt
COPY --from=ui /ui/node_modules/@fontsource/instrument-serif/LICENSE /usr/share/licenses/clinical-history/instrument-serif.txt
COPY --from=ui /ui/node_modules/lucide-react/LICENSE /usr/share/licenses/clinical-history/lucide.txt
COPY --from=ui /ui/node_modules/react/LICENSE /usr/share/licenses/clinical-history/react.txt
COPY --from=ui /ui/node_modules/react-dom/LICENSE /usr/share/licenses/clinical-history/react-dom.txt
COPY --from=ui /ui/node_modules/scheduler/LICENSE /usr/share/licenses/clinical-history/scheduler.txt
ENV CLINICAL_ASSETS=/ui
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app"]
