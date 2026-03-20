import numpy as np

from aios_search.embedder import Embedder


def test_embed_single_text():
    embedder = Embedder(model_name="all-MiniLM-L6-v2")
    vectors = embedder.embed(["Hello world"])

    assert len(vectors) == 1
    assert len(vectors[0]) == 384
    assert isinstance(vectors[0], np.ndarray)


def test_embed_batch():
    embedder = Embedder(model_name="all-MiniLM-L6-v2")
    texts = [f"Text number {i}" for i in range(10)]
    vectors = embedder.embed(texts, batch_size=4)

    assert len(vectors) == 10
    assert all(len(v) == 384 for v in vectors)


def test_embed_empty_list():
    embedder = Embedder(model_name="all-MiniLM-L6-v2")
    vectors = embedder.embed([])
    assert vectors == []


def test_similar_texts_have_higher_similarity():
    embedder = Embedder(model_name="all-MiniLM-L6-v2")
    vecs = embedder.embed([
        "Kubernetes cluster deployment",
        "K8s pod orchestration",
        "Chocolate cake recipe",
    ])
    sim_k8s = np.dot(vecs[0], vecs[1]) / (np.linalg.norm(vecs[0]) * np.linalg.norm(vecs[1]))
    sim_cake = np.dot(vecs[0], vecs[2]) / (np.linalg.norm(vecs[0]) * np.linalg.norm(vecs[2]))
    assert sim_k8s > sim_cake
