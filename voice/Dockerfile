FROM python:3.14-slim
WORKDIR /app
COPY pyproject.toml ./
RUN pip install --no-cache-dir .
COPY src/ ./src/
COPY static/ ./static/
USER 1000:1000
EXPOSE 8080
CMD ["python", "-m", "src"]
