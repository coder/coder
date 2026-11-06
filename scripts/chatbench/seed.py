#!/usr/bin/env python3
"""Seed the chat rendering benchmark deployment.

Creates the first user, an openai-compat provider pointing at fakellm, a
default chat model, and N chats each with TURNS completed user/assistant
turns. Writes $CHATBENCH_ROOT/state.json (default ~/.cache/chatbench) for the
browser harness.

Usage: seed.py [--chats 2] [--turns 120]
       seed.py add [--turns 120]   append one more chat to the existing state
       seed.py extend <chat_id> [--turns 120]   add more completed turns to one chat
       seed.py stream <seconds>    start a long streaming turn in every seeded chat
       seed.py status              print the status of every seeded chat
       seed.py login               refresh the session token in state.json

Session tokens expire after a day; stream, status and add refresh the token
in state.json when the saved one is rejected.
"""
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

ROOT = os.environ.get(
    "CHATBENCH_ROOT",
    os.path.join(os.environ.get("XDG_CACHE_HOME", os.path.expanduser("~/.cache")), "chatbench"),
)
API = "http://127.0.0.1:18080"
LLM = "http://127.0.0.1:18081"
STATE = os.path.join(ROOT, "state.json")

USER = {"email": "bench@example.com", "username": "bench", "password": "BenchPassword123!", "name": "Bench", "trial": False}


class Client:
    def __init__(self, token=None):
        self.token = token

    def req(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Coder-Session-Token"] = self.token
        r = urllib.request.Request(API + path, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(r, timeout=60) as resp:
                raw = resp.read()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as e:
            body = e.read().decode(errors="replace")
            raise SystemExit(f"{method} {path} -> {e.code}: {body[:500]}")

    def authenticated(self):
        """True when the saved token is still accepted."""
        r = urllib.request.Request(API + "/api/v2/users/me", headers={"Coder-Session-Token": self.token or ""})
        try:
            with urllib.request.urlopen(r, timeout=60):
                return True
        except urllib.error.HTTPError as e:
            if e.code == 401:
                return False
            raise


def wait_status(c, chat_id, want=("waiting", "completed"), timeout=120):
    """Poll chat status. Bounded: fakellm answers seed turns immediately, so
    the chat converges within a few seconds or something is broken."""
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        chat = c.req("GET", f"/api/v2/chats/{chat_id}")
        last = chat["status"]
        if last in want:
            return chat
        if last == "error":
            raise SystemExit(f"chat {chat_id} turn failed: {chat.get('error') or chat}")
        time.sleep(0.2)
    raise SystemExit(f"chat {chat_id} stuck in status {last!r} after {timeout}s")


def text_part(text):
    return [{"type": "text", "text": text}]


def user_prose(i):
    return (
        f"BENCH:seed {i} Please review the provisioner scheduling path once more and "
        f"summarize the retry behavior for job {i}. Focus on the audit log and the "
        f"database indexes involved, and call out anything that looks wrong."
    )


def login():
    c = Client()
    try:
        login = c.req("POST", "/api/v2/users/login", {"email": USER["email"], "password": USER["password"]})
    except SystemExit:
        c.req("POST", "/api/v2/users/first", USER)
        login = c.req("POST", "/api/v2/users/login", {"email": USER["email"], "password": USER["password"]})
    c.token = login["session_token"]
    return c


def load_state():
    with open(STATE) as f:
        return json.load(f)


def save_state(st):
    os.makedirs(ROOT, exist_ok=True)
    with open(STATE, "w") as f:
        json.dump(st, f, indent=1)


def state_client(st):
    """Client for the saved state; re-logs in and rewrites the token when the
    saved one has expired so the browser harness keeps working."""
    c = Client(st["token"])
    if not c.authenticated():
        c = login()
        st["token"] = c.token
        save_state(st)
        print("session token refreshed in", STATE)
    return c


def setup(c=None):
    c = c or login()
    me = c.req("GET", "/api/v2/users/me")
    org_id = me["organization_ids"][0]
    provider = next((p for p in c.req("GET", "/api/v2/ai/providers") if p["name"] == "fakellm"), None)
    if provider is None:
        provider = c.req("POST", "/api/v2/ai/providers", {
            "type": "openai-compat", "name": "fakellm", "display_name": "Fake LLM",
            "enabled": True, "base_url": LLM, "api_keys": ["sk-fake"],
        })
    models = c.req("GET", f"/api/v2/organizations/{org_id}/chats/models")["models"]
    model = next((m for m in models if m["model"] == "bench-model"), None)
    if model is None:
        model = c.req("POST", f"/api/v2/organizations/{org_id}/chats/models", {
            "ai_provider_id": provider["id"], "model": "bench-model", "display_name": "Bench Model",
            "enabled": True, "is_default": True, "context_limit": 400000,
        })
    return c, org_id, model["id"]


def seed_chat(c, org_id, model_id, turns, label):
    chat = c.req("POST", "/api/v2/chats", {
        "organization_id": org_id, "title": f"Bench chat {label}",
        "content": text_part(user_prose(1)), "model_config_id": model_id,
    })
    chat_id = chat["id"]
    wait_status(c, chat_id)
    extend_chat(c, chat_id, 2, turns, label)
    return chat_id


def extend_chat(c, chat_id, first, last, label):
    """Complete seed turns first..last (inclusive) in an existing chat."""
    for i in range(first, last + 1):
        c.req("POST", f"/api/v2/chats/{chat_id}/messages", {"content": text_part(user_prose(i))})
        wait_status(c, chat_id)
        if i % 20 == 0:
            print(f"  chat {label}: {i}/{last} turns", flush=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", nargs="?", default="seed")
    ap.add_argument("arg", nargs="?")
    ap.add_argument("--chats", type=int, default=2)
    ap.add_argument("--turns", type=int, default=120)
    a = ap.parse_args()

    if a.cmd == "login":
        st = load_state()
        st["token"] = login().token
        save_state(st)
        print("session token refreshed in", STATE)
        return

    if a.cmd == "add":
        st = load_state()
        c = state_client(st)
        c, org_id, model_id = setup(c)
        label = chr(ord("A") + len(st["chats"]))
        print(f"seeding chat {label} with {a.turns} turns", flush=True)
        t0 = time.time()
        st["chats"].append(seed_chat(c, org_id, model_id, a.turns, label))
        save_state(st)
        print(f"added chat {label} ({st['chats'][-1]}) with {a.turns} turns in {time.time() - t0:.0f}s")
        return

    if a.cmd == "extend":
        st = load_state()
        c = state_client(st)
        if a.arg not in st["chats"]:
            raise SystemExit(f"chat {a.arg!r} is not in {STATE}")
        t0 = time.time()
        extend_chat(c, a.arg, 1, a.turns, a.arg[:8])
        print(f"extended chat {a.arg} by {a.turns} turns in {time.time() - t0:.0f}s")
        return

    if a.cmd == "stream":
        st = load_state()
        c = state_client(st)
        secs = int(a.arg or 240)
        for chat_id in st["chats"]:
            c.req("POST", f"/api/v2/chats/{chat_id}/messages", {
                "content": text_part(f"BENCH:stream {secs} keep going with the deep dive"),
            })
            print(f"streaming {secs}s started in {chat_id}")
        return

    if a.cmd == "status":
        st = load_state()
        c = state_client(st)
        for chat_id in st["chats"]:
            ch = c.req("GET", f"/api/v2/chats/{chat_id}")
            msgs = c.req("GET", f"/api/v2/chats/{chat_id}/messages?limit=1")
            print(chat_id, ch["status"], "has_more" if msgs.get("has_more") else "", ch.get("title"))
        return

    t0 = time.time()
    c, org_id, model_id = setup()
    chats = []
    for n in range(a.chats):
        label = chr(ord("A") + n)
        print(f"seeding chat {label} with {a.turns} turns", flush=True)
        chats.append(seed_chat(c, org_id, model_id, a.turns, label))
    state = {"token": c.token, "org_id": org_id, "model_id": model_id, "chats": chats,
             "web": "http://127.0.0.1:18080", "turns": a.turns}
    save_state(state)
    print(f"seeded {len(chats)} chats x {a.turns} turns in {time.time() - t0:.0f}s -> {STATE}")
    print("chat ids:", *chats)


if __name__ == "__main__":
    main()
