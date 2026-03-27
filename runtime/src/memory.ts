/**
 * Search result from memory or semantic search.
 */
export interface SearchResult {
  id: string;
  content: string;
  score: number;
  metadata?: Record<string, unknown>;
}

/**
 * Client for interacting with the memory MCP server and aios-search service.
 */
export class MemoryClient {
  private readonly memoryUrl: string;
  private readonly searchUrl: string;
  private readonly fetchFn: typeof globalThis.fetch;

  constructor(
    memoryUrl: string,
    searchUrl: string,
    fetchFn: typeof globalThis.fetch = globalThis.fetch,
  ) {
    this.memoryUrl = memoryUrl;
    this.searchUrl = searchUrl;
    this.fetchFn = fetchFn;
  }

  /**
   * Search memory (Qdrant) for relevant context.
   * @param query - Search query text
   * @param topK - Maximum number of results to return
   */
  async searchMemory(query: string, topK: number = 5): Promise<SearchResult[]> {
    const response = await this.fetchFn(`${this.memoryUrl}/search`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, top_k: topK }),
    });

    if (!response.ok) {
      throw new Error(
        `Memory search failed: ${response.status} ${response.statusText}`,
      );
    }

    const data = (await response.json()) as { results: SearchResult[] };
    return data.results;
  }

  /**
   * Store a memory entry in Qdrant via the memory MCP server.
   * @param key - Unique key for the memory entry
   * @param content - Text content to store
   * @param metadata - Optional metadata to attach
   */
  async storeMemory(
    key: string,
    content: string,
    metadata?: Record<string, unknown>,
  ): Promise<void> {
    const response = await this.fetchFn(`${this.memoryUrl}/store`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, content, metadata }),
    });

    if (!response.ok) {
      throw new Error(
        `Memory store failed: ${response.status} ${response.statusText}`,
      );
    }
  }

  /**
   * Semantic search across the vault/codebase via aios-search.
   * @param query - Search query text
   * @param topK - Maximum number of results to return
   */
  async semanticSearch(
    query: string,
    topK: number = 5,
  ): Promise<SearchResult[]> {
    const response = await this.fetchFn(`${this.searchUrl}/search`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, limit: topK }),
    });

    if (!response.ok) {
      throw new Error(
        `Semantic search failed: ${response.status} ${response.statusText}`,
      );
    }

    const data = (await response.json()) as { results: SearchResult[] };
    return data.results;
  }
}
