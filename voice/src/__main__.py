"""Entry point for running the voice gateway with uvicorn."""

import uvicorn

from src.config import VoiceConfig


def main() -> None:
    config = VoiceConfig.from_env()
    uvicorn.run(
        "src.gateway:app",
        host="0.0.0.0",
        port=config.port,
        log_level="info",
    )


if __name__ == "__main__":
    main()
