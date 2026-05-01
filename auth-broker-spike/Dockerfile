FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq tini \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g @mariozechner/pi-coding-agent@0.70.6
WORKDIR /work
ENV PI_CODING_AGENT_DIR=/pi-state
ENV TERM=dumb
ENTRYPOINT ["/usr/bin/tini","--"]
CMD ["bash"]
