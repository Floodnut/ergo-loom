"""
Ergo Loom — Claude Agent SDK bridge.

Wraps claude_agent_sdk and exposes a streaming HTTP endpoint so the Go
backend can drive Claude via the subscription OAuth token instead of the
Claude Code CLI subprocess.

Usage:
    pip install -r requirements.txt
    CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-... python main.py [--port 3764]

Environment:
    CLAUDE_CODE_OAUTH_TOKEN   OAuth token from `claude setup-token`
    ERGO_SDK_BRIDGE_PORT      Port to listen on (default 3764)
"""

import asyncio
import json
import os
import sys
import argparse
from typing import AsyncIterator

from aiohttp import web
import claude_code_sdk as sdk
from claude_code_sdk import (
    query,
    ClaudeCodeOptions,
    AssistantMessage,
    TextBlock,
    ToolUseBlock,
    ToolResultBlock,
)


PORT = int(os.environ.get("ERGO_SDK_BRIDGE_PORT", "3764"))


async def stream_chat(request: web.Request) -> web.StreamResponse:
    try:
        body = await request.json()
    except Exception:
        return web.Response(status=400, text="invalid json body")

    prompt: str = body.get("prompt", "").strip()
    session_id: str = body.get("sessionId", "").strip()
    model_ref: str = body.get("modelRef", "").strip()
    thinking_effort: str = body.get("thinkingEffort", "medium").strip()

    if not prompt:
        return web.Response(status=400, text="prompt is required")

    response = web.StreamResponse(
        status=200,
        headers={
            "Content-Type": "text/event-stream; charset=utf-8",
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
        },
    )
    await response.prepare(request)

    emitted_session_id = session_id
    full_text: list[str] = []

    try:
        options = ClaudeCodeOptions(
            allowed_tools=[],
            model=_map_model(model_ref) or None,
        )
        if session_id:
            options.resume = session_id

        async for message in query(prompt=prompt, options=options):
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock) and block.text:
                        full_text.append(block.text)
                        await _send_event(response, "delta", {"text": block.text})
                    elif isinstance(block, ToolUseBlock):
                        await _send_event(response, "tool_start", {
                            "toolName": block.name,
                            "toolId": block.id,
                            "command": _tool_input_summary(block.input),
                        })
            # sdk surfaces session_id on the result message
            raw = getattr(message, "__dict__", {})
            if sid := raw.get("session_id") or raw.get("sessionId"):
                emitted_session_id = sid

    except Exception as exc:
        await _send_event(response, "error", {"message": str(exc)})
        await response.write_eof()
        return response

    await _send_event(response, "done", {
        "text": "".join(full_text),
        "sessionId": emitted_session_id,
    })
    await response.write_eof()
    return response


async def health(request: web.Request) -> web.Response:
    return web.Response(text="ok")


async def _send_event(response: web.StreamResponse, kind: str, payload: dict) -> None:
    line = json.dumps({"type": kind, "payload": payload}, ensure_ascii=False)
    await response.write(f"data: {line}\n\n".encode())


def _map_model(model_ref: str) -> str:
    mapping = {
        "claude-sonnet-4.6": "claude-sonnet-4-5",
        "claude-sonnet-4-6": "claude-sonnet-4-5",
        "claude-haiku-4.5": "claude-haiku-4-5",
        "claude-haiku-4-5": "claude-haiku-4-5",
        "claude-opus-4.8": "claude-opus-4-5",
        "claude-opus-4-8": "claude-opus-4-5",
        "sonnet": "claude-sonnet-4-5",
        "haiku": "claude-haiku-4-5",
        "opus": "claude-opus-4-5",
    }
    return mapping.get(model_ref.strip(), "")


def _tool_input_summary(input_value) -> str:
    if isinstance(input_value, dict):
        if cmd := input_value.get("command"):
            return str(cmd)
        try:
            return json.dumps(input_value)
        except Exception:
            return str(input_value)
    return str(input_value) if input_value else ""


def main() -> None:
    parser = argparse.ArgumentParser(description="Ergo Loom Claude SDK bridge")
    parser.add_argument("--port", type=int, default=PORT)
    args = parser.parse_args()

    if not os.environ.get("CLAUDE_CODE_OAUTH_TOKEN") and not os.environ.get("ANTHROPIC_API_KEY"):
        print(
            "Warning: neither CLAUDE_CODE_OAUTH_TOKEN nor ANTHROPIC_API_KEY is set.\n"
            "Run `claude setup-token` and export CLAUDE_CODE_OAUTH_TOKEN.",
            file=sys.stderr,
        )

    app = web.Application()
    app.router.add_post("/v1/claude/chat", stream_chat)
    app.router.add_get("/healthz", health)

    print(f"Ergo Loom Claude SDK bridge: http://127.0.0.1:{args.port}", flush=True)
    web.run_app(app, host="127.0.0.1", port=args.port, print=None)


if __name__ == "__main__":
    main()
