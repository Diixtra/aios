# AIOS

Kubernetes-native platform for autonomous code generation. AIOS receives GitHub issues, researches context, plans implementations, generates code with Claude Agent SDK, verifies tests pass, and delivers pull requests.

## Pipeline

```
GitHub Issue -> Webhook -> AgentTask CR -> Operator -> K8s Job (runtime) -> PR
```

The runtime executes a 6-stage pipeline:

1. **Research** -- queries memory vault and semantic search for context
2. **Understand** -- analyzes the issue using fabric-ai patterns
3. **Plan** -- develops an implementation approach
4. **Implement** -- generates code using sandboxed Claude tools (shell, read_file, write_file)
5. **Verify** -- runs tests, retries up to 3 times on failure
6. **Deliver** -- opens a pull request

## Components

| Component | Language | Description |
|-----------|----------|-------------|
| runtime | TypeScript | 6-stage agent pipeline execution engine |
| operator | Go | Kubernetes CRD controller for AgentTask orchestration |
| webhook | Go | GitHub and Paperless-ngx event handlers |
| ticktick-sync | Go | Bidirectional TickTick / GitHub issue sync |
| aios-search | Python | Semantic search service using vector embeddings |
| mcp-proxy | Python | MCP client/server bridge with sampling support |
| mcp-servers | Go/Python | MCP servers for Kubernetes, Stripe, Grafana, Cloudflare |

## Development

Requires [devbox](https://www.jetpack.io/devbox/) (manages Go 1.26, Node.js 22, Python 3.14).

```bash
devbox shell

# Runtime
cd runtime && npm ci && npx vitest run

# Webhook
cd webhook && go test ./...

# Operator
cd operator && go test ./...
```

### Pre-commit hooks

Install once after cloning:

```sh
mise install
lefthook install
```

This wires the org-wide hooks (gitleaks + biome + ruff + gofmt + Conventional Commits) defined in `lefthook.yml`. Hooks run on developer machines; CI re-runs them as a safety net.

## Deployment

Deployed on a homelab K8s cluster (Forge) via Flux GitOps. Manifests in `k8s/`.

## License

This project is licensed under the MIT License -- see [LICENSE](LICENSE) for details.
