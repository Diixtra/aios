import { describe, it, expect, vi, beforeEach } from "vitest";
import { MemoryClient } from "./memory.js";

describe("MemoryClient", () => {
  let client: MemoryClient;
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockFetch = vi.fn();
    client = new MemoryClient(
      "http://memory:8080",
      "http://search:8080",
      mockFetch as any,
    );
  });

  describe("searchMemory", () => {
    it("searches memory and returns results", async () => {
      const results = [
        { id: "1", content: "result 1", score: 0.9, metadata: {} },
        { id: "2", content: "result 2", score: 0.8, metadata: {} },
      ];

      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({ results }),
      });

      const res = await client.searchMemory("test query", 5);

      expect(res).toEqual(results);
      expect(mockFetch).toHaveBeenCalledWith("http://memory:8080/search", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: "test query", top_k: 5 }),
      });
    });

    it("throws on HTTP error", async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      await expect(client.searchMemory("query")).rejects.toThrow(
        "Memory search failed: 500",
      );
    });

    it("uses default topK of 5", async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({ results: [] }),
      });

      await client.searchMemory("query");

      expect(mockFetch).toHaveBeenCalledWith(
        "http://memory:8080/search",
        expect.objectContaining({
          body: JSON.stringify({ query: "query", top_k: 5 }),
        }),
      );
    });
  });

  describe("storeMemory", () => {
    it("stores a memory entry", async () => {
      mockFetch.mockResolvedValue({ ok: true });

      await client.storeMemory("key1", "content text", { source: "test" });

      expect(mockFetch).toHaveBeenCalledWith("http://memory:8080/store", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          key: "key1",
          content: "content text",
          metadata: { source: "test" },
        }),
      });
    });

    it("stores without metadata", async () => {
      mockFetch.mockResolvedValue({ ok: true });

      await client.storeMemory("key1", "content text");

      expect(mockFetch).toHaveBeenCalledWith(
        "http://memory:8080/store",
        expect.objectContaining({
          body: JSON.stringify({ key: "key1", content: "content text" }),
        }),
      );
    });

    it("throws on HTTP error", async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 503,
        statusText: "Service Unavailable",
      });

      await expect(
        client.storeMemory("key1", "content"),
      ).rejects.toThrow("Memory store failed: 503");
    });
  });

  describe("semanticSearch", () => {
    it("searches via aios-search and returns results", async () => {
      const results = [
        { file_path: "docs/arch.md", title: "Architecture", score: 0.95, snippet: "vault doc" },
      ];

      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({ results }),
      });

      const res = await client.semanticSearch("architecture patterns", 3);

      expect(res).toEqual(results);
      expect(mockFetch).toHaveBeenCalledWith("http://search:8080/search", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: "architecture patterns", limit: 3 }),
      });
    });

    it("throws on HTTP error", async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      await expect(client.semanticSearch("query")).rejects.toThrow(
        "Semantic search failed: 404",
      );
    });
  });
});
