FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc
RUN apt-get update && apt-get install -y --no-install-recommends python3 python3-jsonschema && rm -rf /var/lib/apt/lists/*
COPY tests/fhir /validator
USER 65532:65532
ENTRYPOINT ["python3","/validator/check.py"]
