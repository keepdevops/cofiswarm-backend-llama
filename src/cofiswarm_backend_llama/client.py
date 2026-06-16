"""HTTP client for llama-server OpenAI-compatible /v1/chat/completions."""
from __future__ import annotations

import json
import urllib.request
from dataclasses import dataclass
from typing import Any


@dataclass
class LlamaClient:
    host: str = "127.0.0.1"
    port: int = 8080
    timeout: int = 120

    @property
    def base_url(self) -> str:
        return f"http://{self.host}:{self.port}"

    def chat(self, system_prompt: str, prompt: str, max_tokens: int = 512) -> str:
        messages: list[dict[str, str]] = []
        if system_prompt:
            messages.append({"role": "system", "content": system_prompt})
        messages.append({"role": "user", "content": prompt})
        body: dict[str, Any] = {
            "messages": messages,
            "max_tokens": max_tokens,
            "cache_prompt": True,
            "stop": ["", "<|im_start|>", "<|eot_id|>"],
        }
        req = urllib.request.Request(
            f"{self.base_url}/v1/chat/completions",
            data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=self.timeout) as resp:
            data = json.loads(resp.read())
        return data["choices"][0]["message"]["content"]

    def health(self) -> bool:
        try:
            with urllib.request.urlopen(f"{self.base_url}/health", timeout=5) as resp:
                return resp.status == 200
        except Exception:
            return False
