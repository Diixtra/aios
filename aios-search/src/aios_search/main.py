import asyncio
import logging
from contextlib import asynccontextmanager
from dataclasses import dataclass
from datetime import datetime, timezone

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


@dataclass
class ReconcileState:
    # Kubernetes probes hit /health before the background startup finishes, so the
    # stage is what they see while the backing services come up.
    stage: str = "pending"
    error: str | None = None
    started_at: str | None = None
    completed_at: str | None = None


async def _run_startup(indexer: Indexer, watcher: Watcher, state: ReconcileState) -> None:
    loop = asyncio.get_running_loop()
    state.started_at = datetime.now(timezone.utc).isoformat()
    try:
        state.stage = "ensuring_collection"
        await loop.run_in_executor(None, indexer.ensure_collection)

        state.stage = "reconciling"
        await loop.run_in_executor(None, watcher.reconcile)

        state.stage = "watching"
        watcher.start()
        state.completed_at = datetime.now(timezone.utc).isoformat()
    except asyncio.CancelledError:
        raise
    except Exception as exc:
        state.stage = "error"
        state.error = f"{type(exc).__name__}: {exc}"
        logger.exception("Startup background task failed; /health remains reachable")


def create_app(
    settings: Settings | None = None,
    indexer_override: Indexer | None = None,
    watcher_override: Watcher | None = None,
) -> FastAPI:
    if settings is None:
        settings = Settings()

    if indexer_override is None:
        embedder = Embedder(
            base_url=settings.embedding_url,
            model_name=settings.embedding_model,
        )
        indexer = Indexer(
            database_url=settings.database_url,
            embedder=embedder,
            batch_size=settings.upsert_batch_size,
        )
    else:
        indexer = indexer_override

    watcher = watcher_override if watcher_override is not None else Watcher(
        settings=settings, indexer=indexer
    )

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        logger.info("Starting AIOS Search...")
        state = ReconcileState()
        app.state.reconcile = state
        task = asyncio.create_task(_run_startup(indexer, watcher, state))
        app.state.reconcile_task = task
        logger.info("AIOS Search HTTP ready; reconcile continues in background")
        try:
            yield
        finally:
            logger.info("Shutting down...")
            if not task.done():
                task.cancel()
                try:
                    await task
                except asyncio.CancelledError:
                    pass
                except Exception:
                    logger.exception("Startup task errored during shutdown")
            watcher.stop()
            close = getattr(indexer, "close", None)
            if callable(close):
                try:
                    close()
                except Exception:
                    logger.exception("Indexer close failed during shutdown")

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
