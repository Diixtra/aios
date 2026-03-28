FROM python:3.14-slim AS base

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY pyproject.toml .
COPY src/ src/

RUN uv pip install --system "."

EXPOSE 8000

CMD ["python", "-m", "aios_search.main"]
