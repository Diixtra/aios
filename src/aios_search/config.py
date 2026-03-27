from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Required
    vault_path: str
    qdrant_url: str
    qdrant_api_key: str
    aios_api_key: str

    # Defaults
    collection_name: str = "aios_vault"
    embedding_model: str = "all-MiniLM-L6-v2"
    embedding_url: str = "http://local-ai.local-ai.svc.cluster.local:8080"
    vector_size: int = 384
    min_score: float = 0.3
    qdrant_batch_size: int = 100
    debounce_ms: int = 500
    host: str = "0.0.0.0"
    port: int = 8000
    chunk_size_threshold: int = 1024
    chunk_word_window: int = 200
    chunk_word_overlap: int = 30
    snippet_length: int = 200

    ignored_dirs: list[str] = [
        ".obsidian",
        "80-Dashboards",
        "90-Templates",
        ".stfolder",
    ]
    ignored_files: list[str] = [
        ".stignore",
        ".DS_Store",
    ]

    model_config = {"env_prefix": ""}
