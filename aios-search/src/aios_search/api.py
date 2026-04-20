import logging
import threading
from pathlib import Path

import frontmatter
from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from aios_search.auth import require_api_key
from aios_search.indexer import Indexer

logger = logging.getLogger(__name__)


class SearchRequest(BaseModel):
    query: str
    limit: int = 5
    filters: dict | None = None


class SimilarRequest(BaseModel):
    file_path: str
    limit: int = 5
    filters: dict | None = None


def create_router(
    indexer: Indexer,
    vault_path: str,
    api_key: str,
    reindex_callback=None,
) -> APIRouter:
    router = APIRouter()
    auth = require_api_key(api_key)

    @router.post("/search")
    def search(req: SearchRequest, _=Depends(auth)):
        try:
            results = indexer.search(
                query=req.query,
                limit=req.limit,
                filters=req.filters,
            )
        except Exception:
            logger.exception("Search failed — vector database unavailable")
            return JSONResponse(
                status_code=503,
                content={"error": "Vector database unavailable"},
            )
        return {"results": results}

    @router.post("/similar")
    def similar(req: SimilarRequest, _=Depends(auth)):
        try:
            results = indexer.find_similar(
                file_path=req.file_path,
                limit=req.limit,
                filters=req.filters,
            )
        except Exception:
            logger.exception("Similar search failed — vector database unavailable")
            return JSONResponse(
                status_code=503,
                content={"error": "Vector database unavailable"},
            )
        if results is None:
            raise HTTPException(status_code=404, detail="File not indexed")
        return {"results": results}

    @router.get("/note/{path:path}")
    def get_note(path: str, _=Depends(auth)):
        file_path = Path(vault_path) / path
        if not file_path.is_file():
            raise HTTPException(status_code=404, detail="Note not found")

        try:
            post = frontmatter.load(str(file_path))
            metadata = dict(post.metadata)
            content = post.content
            title = metadata.get("title", file_path.stem)
        except Exception:
            content = file_path.read_text(encoding="utf-8")
            metadata = {}
            title = file_path.stem

        return {
            "file_path": path,
            "title": title,
            "metadata": metadata,
            "content": content,
        }

    @router.get("/health")
    def health():
        try:
            stats = indexer.get_stats()
            return {**stats, "healthy": True}
        except Exception as e:
            return {"healthy": False, "error": str(e)}

    @router.post("/reindex", status_code=202)
    def reindex(_=Depends(auth)):
        if reindex_callback:
            threading.Thread(target=reindex_callback, daemon=True).start()
        return {"status": "reindex_started"}

    return router
