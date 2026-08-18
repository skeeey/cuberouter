#!/usr/bin/env python3
"""
MockLLM launcher (workaround for the missing OpenAI-compatible /v1/models).

Upstream mockllm (0.0.7/0.0.8) only implements POST /v1/chat/completions and
POST /v1/messages; it has no /v1/models, which breaks cuberouter's channel
"fetch models from upstream" feature (it calls {base_url}/v1/models and parses
{"data": [{"id": ...}]}). This launcher imports mockllm's FastAPI app, mounts
an OpenAI-compatible GET /v1/models on it, and runs it — no proxy needed.

The model list comes from the optional `models:` key of the responses YAML
(e2e/mockllm/responses.yml), so it stays in sync with the e2e suite.
"""
import argparse
import os

import yaml


def load_models(responses_path: str) -> list[str]:
    with open(responses_path, encoding="utf-8") as f:
        data = yaml.safe_load(f) or {}
    models = data.get("models") or ["mock-llm"]
    return [m for m in models if isinstance(m, str) and m.strip()]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--responses",
        default=os.environ.get("MOCKLLM_RESPONSES_FILE", "responses.yml"),
        help="Path to the responses YAML (also feeds mockllm's own config)",
    )
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8000)
    args = parser.parse_args()

    os.environ["MOCKLLM_RESPONSES_FILE"] = args.responses

    # Importing the module builds the FastAPI app and registers mockllm's own
    # /v1/chat/completions + /v1/messages routes (in the lifespan).
    from fastapi.responses import JSONResponse
    from mockllm import server

    models = load_models(args.responses)

    @server.app.get("/v1/models")
    def list_models_openai() -> JSONResponse:
        return JSONResponse(
            {
                "object": "list",
                "data": [
                    {"id": m, "object": "model", "created": 0, "owned_by": "mockllm"}
                    for m in models
                ],
            }
        )

    import uvicorn

    uvicorn.run(server.app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
