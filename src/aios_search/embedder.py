import httpx
import numpy as np


class Embedder:
    def __init__(self, base_url: str, model_name: str = "all-MiniLM-L6-v2"):
        self._base_url = base_url
        self._model_name = model_name

    def embed(self, texts: list[str]) -> list[np.ndarray]:
        if not texts:
            return []

        with httpx.Client(base_url=self._base_url, timeout=60.0) as client:
            resp = client.post(
                "/v1/embeddings",
                json={"model": self._model_name, "input": texts},
            )
            resp.raise_for_status()

        data = resp.json()["data"]
        # Sort by index to guarantee order matches input
        data.sort(key=lambda d: d["index"])
        return [np.array(d["embedding"], dtype=np.float32) for d in data]
