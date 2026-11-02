FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends chrony faketime \
 && rm -rf /var/lib/apt/lists/*
COPY check-chrony /usr/local/bin/check-chrony
CMD ["sleep", "infinity"]
