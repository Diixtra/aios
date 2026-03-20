import logging
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI

from aios_search.api import create_router
from aios_search.config import Settings
from aios_search.embedder import Embedder
from aios_search.indexer import Indexer
from aios_search.watcher import Watcher

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)
logger = logging.getLogger(__name__)


def create_app(settings: Settings | None = None) -> FastAPI:
    if settings is None:
        settings = Settings()

    embedder = Embedder(model_name=settings.embedding_model)
    indexer = Indexer(
        qdrant_url=settings.qdrant_url,
        qdrant_api_key=settings.qdrant_api_key,
        collection_name=settings.collection_name,
        vector_size=settings.vector_size,
        embedder=embedder,
        qdrant_batch_size=settings.qdrant_batch_size,
    )
    watcher = Watcher(settings=settings, indexer=indexer)

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        logger.info("Starting AIOS Search...")
        indexer.ensure_collection()
        watcher.reconcile()
        watcher.start()
        logger.info("AIOS Search ready")
        yield
        logger.info("Shutting down...")
        watcher.stop()

    app = FastAPI(title="AIOS Search", version="0.1.0", lifespan=lifespan)
    router = create_router(
        indexer=indexer,
        vault_path=settings.vault_path,
        api_key=settings.aios_api_key,
        reindex_callback=watcher.full_reindex,
    )
    app.include_router(router)
    return app


def main():
    settings = Settings()
    app = create_app(settings)
    uvicorn.run(app, host=settings.host, port=settings.port)


if __name__ == "__main__":
    main()
