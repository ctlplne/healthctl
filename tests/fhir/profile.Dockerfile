FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl openjdk-17-jre-headless && rm -rf /var/lib/apt/lists/*
RUN curl --fail --show-error --silent --location --retry 3 https://github.com/hapifhir/org.hl7.fhir.core/releases/download/7.0.6/validator_cli.jar -o /opt/validator.jar && echo '2686590539f0b3ba7683149b25d78249f6886c04e2c326e884c5049d8947df89  /opt/validator.jar' | sha256sum -c -
# Cache only public specification dependencies at build time. Patient material
# is never present during this step. Actual validation runs without a network.
RUN echo '{"resourceType":"Patient","id":"synthetic","active":true}' > /tmp/seed.json && java -Xmx1100m -jar /opt/validator.jar /tmp/seed.json -version 4.0.1 -tx n/a
ENTRYPOINT ["java", "-Xmx1400m", "-jar", "/opt/validator.jar"]
