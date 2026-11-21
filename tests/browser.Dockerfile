FROM mcr.microsoft.com/playwright:v1.55.1-noble@sha256:2f29369043d81d6d69a815ceb80760f55e85f5020371ad06a4d996f18503ad1c
WORKDIR /ui
COPY web/package*.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
COPY internal/clinical/model-guidance.txt /internal/clinical/model-guidance.txt
CMD ["npm","run","test:e2e"]
