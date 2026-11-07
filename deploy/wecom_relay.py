#!/usr/bin/env python3
"""Relay KubeTask failure and timeout webhooks to a WeCom group robot."""

import argparse
import json
import logging
import os
from http import HTTPStatus
from http.client import HTTPException
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


EVENT_LABELS = {"task.failed": "任务失败", "task.timeout": "任务超时"}
MAX_EVENT_BYTES = 64 * 1024
MAX_REPLY_BYTES = 8 * 1024
MAX_TEXT_BYTES = 2048


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


OPENER = build_opener(NoRedirect())


def wecom_payload(event):
    label = EVENT_LABELS[event["type"]]
    execution = event["execution"]
    fields = {
        "任务": event["task"]["name"],
        "命名空间": execution["namespace"],
        "Job": execution["jobName"],
        "事件 ID": event["eventId"],
        "原因": execution.get("reason") or "-",
    }
    if not all(isinstance(value, str) and value.strip() for value in fields.values()):
        raise ValueError("invalid event fields")

    lines = [f"[KubeTask] {label}"]
    lines.extend(f"{name}: {value}" for name, value in fields.items())
    content = "\n".join(lines)
    encoded = content.encode("utf-8")
    if len(encoded) > MAX_TEXT_BYTES:
        suffix = "…"
        end = MAX_TEXT_BYTES - len(suffix.encode("utf-8"))
        content = encoded[:end].decode("utf-8", "ignore") + suffix
    return json.dumps({"msgtype": "text", "text": {"content": content}}, ensure_ascii=False).encode("utf-8")


def forward_to_wecom(url, payload):
    request = Request(url, data=payload, headers={"Content-Type": "application/json"}, method="POST")
    try:
        with OPENER.open(request, timeout=3) as response:
            body = response.read(MAX_REPLY_BYTES + 1)
    except HTTPError as error:
        error.close()
        if error.code == HTTPStatus.TOO_MANY_REQUESTS:
            return HTTPStatus.TOO_MANY_REQUESTS
        if 500 <= error.code < 600:
            return HTTPStatus.SERVICE_UNAVAILABLE
        return HTTPStatus.UNPROCESSABLE_ENTITY
    except (OSError, HTTPException):
        return HTTPStatus.SERVICE_UNAVAILABLE

    if len(body) > MAX_REPLY_BYTES:
        return HTTPStatus.BAD_GATEWAY
    try:
        code = json.loads(body)["errcode"]
    except (ValueError, KeyError, TypeError, RecursionError):
        return HTTPStatus.BAD_GATEWAY
    if type(code) is not int:
        return HTTPStatus.BAD_GATEWAY
    if code == 0:
        return HTTPStatus.NO_CONTENT
    logging.warning("WeCom rejected the alert (errcode=%d)", code)
    if code == 45009:
        return HTTPStatus.TOO_MANY_REQUESTS
    return HTTPStatus.UNPROCESSABLE_ENTITY


class RelayHandler(BaseHTTPRequestHandler):
    timeout = 5

    def do_POST(self):
        if self.path != "/hooks/kubetask":
            self.reply(HTTPStatus.NOT_FOUND)
            return
        if self.headers.get_content_type() != "application/json":
            self.reply(HTTPStatus.UNSUPPORTED_MEDIA_TYPE)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0:
                raise ValueError("missing request body")
            if length > MAX_EVENT_BYTES:
                self.reply(HTTPStatus.REQUEST_ENTITY_TOO_LARGE)
                return
            body = self.rfile.read(length)
            if len(body) != length:
                raise ValueError("incomplete request body")
            payload = wecom_payload(json.loads(body))
        except TimeoutError:
            self.reply(HTTPStatus.REQUEST_TIMEOUT)
            return
        except (ValueError, KeyError, TypeError, RecursionError):
            self.reply(HTTPStatus.BAD_REQUEST)
            return
        self.reply(forward_to_wecom(self.server.webhook_url, payload))

    def reply(self, status):
        self.send_response(status)
        if status != HTTPStatus.NO_CONTENT:
            self.send_header("Content-Length", "0")
        self.end_headers()

    def log_request(self, code="-", size="-"):
        self.log_message("status=%s", code)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1", help="listen address (default: 127.0.0.1)")
    parser.add_argument("--port", type=int, default=8090, help="listen port (default: 8090)")
    args = parser.parse_args()
    url = os.environ.get("WECOM_WEBHOOK_URL", "").strip()
    try:
        parsed = urlsplit(url)
        if (
            parsed.scheme != "https"
            or parsed.netloc != "qyapi.weixin.qq.com"
            or parsed.path != "/cgi-bin/webhook/send"
            or parsed.fragment
            or len(parse_qs(parsed.query).get("key", [])) != 1
        ):
            raise ValueError("invalid robot URL")
    except ValueError:
        parser.error("WECOM_WEBHOOK_URL must be a WeCom group robot HTTPS URL with a key")

    with ThreadingHTTPServer((args.host, args.port), RelayHandler) as server:
        server.webhook_url = url
        print(f"Listening on {args.host}:{server.server_port}/hooks/kubetask", flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
